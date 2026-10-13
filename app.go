package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jrcasanya/difftwin/internal/comparison"
	"github.com/jrcasanya/difftwin/internal/credentials"
	"github.com/jrcasanya/difftwin/internal/domain"
	"github.com/jrcasanya/difftwin/internal/storage"
)

type App struct {
	store       *storage.Store
	client      *http.Client
	credentials credentials.Store
}

type ProjectAuthSettings struct {
	Auth               map[string]domain.DestinationAuth `json:"auth"`
	Credentials        map[string]domain.AuthCredentials `json:"credentials"`
	CredentialsMissing bool                              `json:"credentials_missing"`
}

type PostmanImportResult struct {
	Collection Collection `json:"collection"`
	Warnings   []string   `json:"warnings"`
}

func NewApp(projectStore *storage.Store) *App {
	return &App{
		store: projectStore, client: &http.Client{Timeout: 30 * time.Second},
		credentials: credentials.KeyringStore{},
	}
}

func (a *App) GetProjects() []Project {
	return a.store.Snapshot()
}

func (a *App) CreateDemoProject() (Project, error) {
	const demoID = "diffbench-api-comparison-demo"
	for _, project := range a.store.Snapshot() {
		if project.ID == demoID {
			return project, nil
		}
	}

	project := Project{
		ID:        demoID,
		Name:      "Demo: comparación de APIs",
		Primary:   "https://httpbin.org/anything/stable/",
		Candidate: "https://httpbin.org/anything/candidate/",
		Secondary: "https://httpbin.org/anything/stable/",
		Collections: []Collection{{
			ID:   "demo-api-differences",
			Name: "Ejemplos",
			Requests: []Request{{
				ID:     "echo-api-versions",
				Name:   "Comparar versiones",
				Method: "GET",
				Path:   "echo?sample=difftwin",
			}},
		}},
	}
	if err := domain.PrepareProject(&project); err != nil {
		return Project{}, err
	}
	if err := a.store.ImportProject(project); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (a *App) SaveProject(project Project) (Project, error) {
	if err := domain.PrepareProject(&project); err != nil {
		return Project{}, err
	}
	existing := domain.FindProject(a.store.Snapshot(), project.ID)
	if existing != nil {
		if hasConfiguredAuth(project.Auth) && !maps.Equal(project.Auth, existing.Auth) {
			return Project{}, errors.New("use SaveProjectWithAuth to change destination authentication")
		}
		project.Auth = existing.Auth
	} else if hasConfiguredAuth(project.Auth) {
		return Project{}, errors.New("use SaveProjectWithAuth to configure destination authentication")
	}
	return a.savePreparedProject(project, false)
}

func (a *App) SaveProjectWithAuth(project Project, credentialsByTarget map[string]domain.AuthCredentials) (Project, error) {
	if err := domain.PrepareProject(&project); err != nil {
		return Project{}, err
	}
	isNew := project.ID == ""
	if project.ID == "" {
		project.ID = domain.NewID()
	}
	if project.Secondary == "" {
		delete(project.Auth, "secondary")
		delete(credentialsByTarget, "secondary")
	}
	for target := range credentialsByTarget {
		if target != "primary" && target != "candidate" && target != "secondary" {
			return Project{}, errors.New("credential target must be primary, candidate, or secondary")
		}
	}
	for target, auth := range project.Auth {
		if err := domain.ValidateAuthCredentials(auth, credentialsByTarget[target]); err != nil {
			return Project{}, fmt.Errorf("%s authentication: %w", target, err)
		}
	}
	existing := domain.FindProject(a.store.Snapshot(), project.ID)
	needsCredentialStore := hasConfiguredAuth(project.Auth) || (existing != nil && hasConfiguredAuth(existing.Auth))
	var previousCredentials string
	var previousErr error
	if needsCredentialStore {
		previousCredentials, previousErr = a.credentials.Get(project.ID)
		if previousErr != nil && !credentials.IsNotFound(previousErr) {
			return Project{}, previousErr
		}
		storedCredentials := make(map[string]domain.AuthCredentials)
		for target, auth := range project.Auth {
			if auth.Type == "" || auth.Type == "none" {
				continue
			}
			value := credentialsByTarget[target]
			switch auth.Type {
			case "bearer":
				value.Username, value.Password, value.APIKey = "", "", ""
			case "basic":
				value.Token, value.APIKey = "", ""
			case "api_key":
				value.Token, value.Username, value.Password = "", "", ""
			}
			storedCredentials[target] = value
		}
		rawCredentials, err := json.Marshal(storedCredentials)
		if err != nil {
			return Project{}, fmt.Errorf("encode destination credentials: %w", err)
		}
		if hasConfiguredAuth(project.Auth) {
			err = a.credentials.Set(project.ID, string(rawCredentials))
		} else {
			err = a.credentials.Delete(project.ID)
		}
		if err != nil {
			return Project{}, err
		}
	}
	saved, err := a.savePreparedProject(project, isNew)
	if err != nil {
		if needsCredentialStore {
			var restoreErr error
			if previousErr == nil {
				restoreErr = a.credentials.Set(project.ID, previousCredentials)
			} else {
				restoreErr = a.credentials.Delete(project.ID)
			}
			if restoreErr != nil {
				return Project{}, fmt.Errorf("%w (also failed to restore previous credentials: %v)", err, restoreErr)
			}
		}
		return Project{}, err
	}
	return saved, nil
}

func (a *App) savePreparedProject(project Project, createNew bool) (Project, error) {
	if project.ID == "" {
		project.ID = domain.NewID()
		createNew = true
	}
	if createNew {
		err := a.store.Update(func(projects *[]Project) error {
			for _, existing := range *projects {
				if existing.ID == project.ID {
					return fmt.Errorf("a project with ID %q already exists", project.ID)
				}
			}
			stored := domain.CloneProjects([]Project{project})[0]
			*projects = append(*projects, stored)
			return nil
		})
		return project, err
	}
	err := a.store.Update(func(projects *[]Project) error {
		for i := range *projects {
			if (*projects)[i].ID == project.ID {
				project.Collections = (*projects)[i].Collections
				(*projects)[i] = domain.CloneProjects([]Project{project})[0]
				return nil
			}
		}
		return os.ErrNotExist
	})
	return project, err
}

func (a *App) GetProjectAuth(projectID string) (ProjectAuthSettings, error) {
	project := domain.FindProject(a.store.Snapshot(), projectID)
	if project == nil {
		return ProjectAuthSettings{}, os.ErrNotExist
	}
	settings := ProjectAuthSettings{
		Auth: project.Auth, Credentials: map[string]domain.AuthCredentials{},
	}
	if !hasConfiguredAuth(project.Auth) {
		return settings, nil
	}
	raw, err := a.credentials.Get(projectID)
	if err != nil {
		if credentials.IsNotFound(err) {
			settings.CredentialsMissing = true
			return settings, nil
		}
		return ProjectAuthSettings{}, err
	}
	if err := json.Unmarshal([]byte(raw), &settings.Credentials); err != nil {
		return ProjectAuthSettings{}, fmt.Errorf("decode credentials from system keyring: %w", err)
	}
	return settings, nil
}

func hasConfiguredAuth(auth map[string]domain.DestinationAuth) bool {
	for _, config := range auth {
		if config.Type != "" && config.Type != "none" {
			return true
		}
	}
	return false
}

func (a *App) DeleteProject(projectID string) error {
	project := domain.FindProject(a.store.Snapshot(), projectID)
	if project == nil {
		return os.ErrNotExist
	}
	var previousCredentials string
	hadCredentials := false
	if hasConfiguredAuth(project.Auth) {
		raw, err := a.credentials.Get(projectID)
		if err != nil && !credentials.IsNotFound(err) {
			return err
		}
		if err == nil {
			previousCredentials, hadCredentials = raw, true
		}
		if err := a.credentials.Delete(projectID); err != nil {
			return err
		}
	}
	err := a.store.Update(func(projects *[]Project) error {
		for i := range *projects {
			if (*projects)[i].ID == projectID {
				*projects = append((*projects)[:i], (*projects)[i+1:]...)
				return nil
			}
		}
		return os.ErrNotExist
	})
	if err != nil && hadCredentials {
		if restoreErr := a.credentials.Set(projectID, previousCredentials); restoreErr != nil {
			return fmt.Errorf("%w (also failed to restore deleted credentials: %v)", err, restoreErr)
		}
	}
	return err
}

func (a *App) SaveCollection(projectID string, collection Collection) (Collection, error) {
	collection.Name = strings.TrimSpace(collection.Name)
	if collection.Name == "" {
		return Collection{}, fmt.Errorf("collection name is required")
	}
	if collection.ID == "" {
		collection.ID = domain.NewID()
	}
	err := a.store.Update(func(projects *[]Project) error {
		project := domain.FindProject(*projects, projectID)
		if project == nil {
			return os.ErrNotExist
		}
		for i := range project.Collections {
			if project.Collections[i].ID == collection.ID {
				project.Collections[i] = collection
				return nil
			}
		}
		project.Collections = append(project.Collections, collection)
		return nil
	})
	return collection, err
}

func (a *App) DeleteCollection(projectID, collectionID string) error {
	return a.store.Update(func(projects *[]Project) error {
		project := domain.FindProject(*projects, projectID)
		if project == nil {
			return os.ErrNotExist
		}
		for i := range project.Collections {
			if project.Collections[i].ID == collectionID {
				project.Collections = append(project.Collections[:i], project.Collections[i+1:]...)
				return nil
			}
		}
		return os.ErrNotExist
	})
}

func (a *App) ImportProjectConfig(raw string) (Project, error) {
	if len(raw) > maxBodySize {
		return Project{}, fmt.Errorf("el archivo de proyecto supera el límite de %d MiB", maxBodySize>>20)
	}
	project, err := decodeProjectConfig([]byte(raw))
	if err != nil {
		return Project{}, err
	}
	project.Auth = map[string]domain.DestinationAuth{}
	if project.ID == "" {
		project.ID = domain.NewID()
	}
	if err := a.store.ImportProject(project); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (a *App) ImportPostmanCollection(projectID, raw string) (PostmanImportResult, error) {
	if len(raw) > maxBodySize {
		return PostmanImportResult{}, fmt.Errorf("la colección supera el límite de %d MiB", maxBodySize>>20)
	}
	collection, variables, warnings, err := decodePostmanCollection([]byte(raw))
	if err != nil {
		return PostmanImportResult{}, err
	}
	err = a.store.Update(func(projects *[]Project) error {
		project := domain.FindProject(*projects, projectID)
		if project == nil {
			return os.ErrNotExist
		}
		project.Collections = append(project.Collections, collection)
		if project.Variables == nil {
			project.Variables = make(map[string]string)
		}
		for key, value := range variables {
			if _, exists := project.Variables[key]; exists {
				warnings = append(warnings, fmt.Sprintf("variable %q omitida porque ya existe en el proyecto", key))
				continue
			}
			project.Variables[key] = value
		}
		return nil
	})
	if err != nil {
		return PostmanImportResult{}, err
	}
	return PostmanImportResult{Collection: collection, Warnings: uniqueStrings(warnings)}, nil
}

func (a *App) RunRequest(projectID string, request Request) (runResult, error) {
	if err := domain.ValidateRequest(request); err != nil {
		return runResult{}, err
	}
	project := domain.FindProject(a.store.Snapshot(), projectID)
	if project == nil {
		return runResult{}, os.ErrNotExist
	}
	credentialsByTarget := map[string]domain.AuthCredentials{}
	if hasConfiguredAuth(project.Auth) {
		raw, err := a.credentials.Get(projectID)
		if err != nil {
			if credentials.IsNotFound(err) {
				return runResult{}, errors.New("configured authentication credentials are missing from the system keyring")
			}
			return runResult{}, err
		}
		if err := json.Unmarshal([]byte(raw), &credentialsByTarget); err != nil {
			return runResult{}, fmt.Errorf("decode credentials from system keyring: %w", err)
		}
	}
	return comparison.Compare(a.client, *project, request, credentialsByTarget), nil
}
