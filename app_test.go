package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrcasanya/difftwin/internal/domain"
	"github.com/jrcasanya/difftwin/internal/storage"
	"github.com/zalando/go-keyring"
)

type memoryCredentialStore struct {
	values map[string]string
	err    error
}

func (store *memoryCredentialStore) Get(projectID string) (string, error) {
	if store.err != nil {
		return "", store.err
	}
	value, ok := store.values[projectID]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (store *memoryCredentialStore) Set(projectID, value string) error {
	if store.err != nil {
		return store.err
	}
	store.values[projectID] = value
	return nil
}

func (store *memoryCredentialStore) Delete(projectID string) error {
	if store.err != nil {
		return store.err
	}
	delete(store.values, projectID)
	return nil
}

func TestAppSaveProjectPreservesCollectionsAndSavesNestedCollection(t *testing.T) {
	projectStore, err := storage.New(filepath.Join(t.TempDir(), "projects"))
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	project, err := app.SaveProject(Project{
		Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test", Secondary: "https://stable-2.test",
		Variables: map[string]string{"region": "global"},
		DestinationVariables: map[string]map[string]string{
			"candidate": {"region": "staging"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	collection, err := app.SaveCollection(project.ID, Collection{
		Name: "Users",
		Folders: []Folder{{
			ID: "folder", Name: "Nested",
			Requests: []Request{{ID: "request", Name: "List", Method: "GET", Path: "/users"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	project.Name = "Updated API"
	if _, err := app.SaveProject(project); err != nil {
		t.Fatal(err)
	}
	saved := app.GetProjects()
	if len(saved) != 1 || saved[0].Name != "Updated API" || len(saved[0].Collections) != 1 {
		t.Fatalf("project save did not preserve collections: %#v", saved)
	}
	if saved[0].Collections[0].ID != collection.ID || saved[0].Collections[0].Folders[0].Requests[0].Path != "/users" {
		t.Fatalf("nested collection did not persist: %#v", saved[0].Collections[0])
	}
	if saved[0].DestinationVariables["candidate"]["region"] != "staging" || saved[0].Variables["region"] != "global" {
		t.Fatalf("shared and destination variables did not persist: %#v", saved[0])
	}
}

func TestAppMoveCollectionItemPersistsAcrossCollections(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	project, err := app.SaveProject(Project{
		Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := app.SaveCollection(project.ID, Collection{
		ID: "source", Name: "Source",
		Folders: []Folder{{ID: "source-folder", Name: "Source folder", Requests: []Request{{
			ID: "request", Name: "List", Method: "GET", Path: "/users",
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	destination, err := app.SaveCollection(project.ID, Collection{
		ID: "destination", Name: "Destination",
		Folders: []Folder{{ID: "destination-folder", Name: "Destination folder"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := app.MoveCollectionItem(project.ID, source.ID, "source-folder", "request", "request", destination.ID, "destination-folder"); err != nil {
		t.Fatal(err)
	}
	saved := domain.FindProject(app.GetProjects(), project.ID)
	if saved == nil || len(saved.Collections[0].Folders[0].Requests) != 0 ||
		saved.Collections[1].Folders[0].Requests[0].ID != "request" {
		t.Fatalf("move was not persisted across collections: %#v", saved)
	}
}

func TestAppSaveProjectAllowsMissingSecondaryDestination(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	project, err := app.SaveProject(Project{
		Name: "API sin secundaria", Primary: " https://stable.test ", Candidate: "https://candidate.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.Secondary != "" || len(app.GetProjects()) != 1 {
		t.Fatalf("expected project with no secondary destination: %#v", project)
	}
	if _, err := app.SaveProject(Project{
		Name: "API con secundaria inválida", Primary: "https://stable.test",
		Candidate: "https://candidate.test", Secondary: "not-a-url",
	}); err == nil {
		t.Fatal("expected invalid optional secondary destination to be rejected")
	}
}

func TestAppCreateDemoProjectIsIdempotent(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	first, err := app.CreateDemoProject()
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.CreateDemoProject()
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(app.GetProjects()) != 1 {
		t.Fatalf("demo project should be created only once: first=%q second=%q projects=%d", first.ID, second.ID, len(app.GetProjects()))
	}
	if len(first.Collections) != 1 || len(first.Collections[0].Requests) != 1 {
		t.Fatalf("demo project is missing its example request: %#v", first)
	}
	request := first.Collections[0].Requests[0]
	if request.Method != "GET" || request.Path != "echo?sample=difftwin" {
		t.Fatalf("unexpected demo request: %#v", request)
	}
	if first.Primary != first.Secondary || first.Primary == first.Candidate {
		t.Fatalf("demo destinations should model two matching stable APIs and one candidate: %#v", first)
	}
}

func TestAppImportPostmanCollectionMergesVariablesAndReportsConflicts(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	project, err := app.SaveProject(Project{
		Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Secondary: "https://stable-2.test", Variables: map[string]string{"token": "project-token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"info":{"name":"Imported","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},"variable":[{"key":"token","value":"collection-token"},{"key":"region","value":"eu"}],"item":[{"name":"Folder","item":[{"name":"List","request":{"method":"GET","url":"https://legacy.test/{{region}}/users?token={{token}}"}}]}]}`
	result, err := app.ImportPostmanCollection(project.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Collection.Name != "Imported" || len(result.Collection.Folders) != 1 {
		t.Fatalf("unexpected imported collection: %#v", result.Collection)
	}
	variables := app.GetProjects()[0].Variables
	if variables["token"] != "project-token" || variables["region"] != "eu" {
		t.Fatalf("unexpected merged variables: %#v", variables)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected variable conflict warning, got %#v", result.Warnings)
	}
}

func TestAppImportProjectConfig(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	raw, err := json.Marshal(Project{
		ID: "imported-api", Name: "Imported API", Primary: "https://stable.test",
		Candidate: "https://candidate.test", Secondary: "https://stable-2.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := app.ImportProjectConfig(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if project.ID == "" || project.Name != "Imported API" {
		t.Fatalf("unexpected imported project: %#v", project)
	}
	if _, err := app.ImportProjectConfig(string(raw)); err == nil {
		t.Fatal("expected duplicate project ID to require an explicit choice")
	}
	copy, err := app.ImportProjectConfigWithAction(string(raw), "copy")
	if err != nil {
		t.Fatal(err)
	}
	if copy.ID == project.ID || copy.Name != "Imported API (copia)" {
		t.Fatalf("unexpected imported copy: %#v", copy)
	}
	copy, err = app.ImportProjectConfigWithAction(string(raw), "copy")
	if err != nil {
		t.Fatal(err)
	}
	if copy.Name != "Imported API (copia 2)" {
		t.Fatalf("expected a unique copy name, got %q", copy.Name)
	}
}

func TestAppReplaceImportedProjectRemovesOldCredentials(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	credentialStore := &memoryCredentialStore{values: map[string]string{}}
	app.credentials = credentialStore

	existing, err := app.SaveProjectWithAuth(Project{
		Name: "Existing API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth:        map[string]domain.DestinationAuth{"primary": {Type: "bearer"}},
		Collections: []Collection{{ID: "old", Name: "Old collection"}},
	}, map[string]domain.AuthCredentials{"primary": {Token: "old-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Project{
		ID: existing.ID, Name: "Replacement API", Primary: "https://new-stable.test", Candidate: "https://new-candidate.test",
		Collections: []Collection{{ID: "new", Name: "New collection"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	replaced, err := app.ImportProjectConfigWithAction(string(raw), "replace")
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Name != "Replacement API" || len(replaced.Collections) != 1 || replaced.Collections[0].ID != "new" {
		t.Fatalf("project configuration was not replaced: %#v", replaced)
	}
	if hasConfiguredAuth(replaced.Auth) {
		t.Fatalf("replaced project retained stale authentication metadata: %#v", replaced.Auth)
	}
	if _, exists := credentialStore.values[existing.ID]; exists {
		t.Fatal("credentials for the replaced project were not removed")
	}
}

func TestExportProjectConfigOmitsAuthentication(t *testing.T) {
	project := Project{
		ID: "exported-api", Name: "Exported API",
		Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{
			"primary": {Type: "bearer"},
		},
		Collections: []Collection{{
			ID: "users", Name: "Users",
			Requests: []Request{{ID: "list-users", Name: "List users", Method: "GET", Path: "/users"}},
		}},
	}

	raw, err := json.Marshal(exportProjectConfig(project))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"auth"`) || strings.Contains(string(raw), "token-secret") {
		t.Fatalf("export contains authentication data: %s", raw)
	}
	var exported projectConfigExport
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatal(err)
	}
	if exported.ID != project.ID || len(exported.Collections) != 1 ||
		exported.Collections[0].Requests[0].Path != "/users" {
		t.Fatalf("export did not preserve project configuration: %#v", exported)
	}
}

func TestProjectExportFilename(t *testing.T) {
	for _, test := range []struct {
		name string
		want string
	}{
		{name: "Mi API / Pruebas", want: "Mi-API-Pruebas.json"},
		{name: "///", want: "proyecto.json"},
	} {
		if got := projectExportFilename(test.name); got != test.want {
			t.Errorf("projectExportFilename(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestAppStoresDestinationCredentialsOutsideProjectJSON(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	credentialStore := &memoryCredentialStore{values: map[string]string{}}
	app.credentials = credentialStore

	project, err := app.SaveProjectWithAuth(Project{
		Name: "Secure API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{
			"primary":   {Type: "bearer"},
			"candidate": {Type: "api_key", APIKeyName: "X-API-Key", APIKeyLocation: "header"},
		},
	}, map[string]domain.AuthCredentials{
		"primary":   {Token: "primary-secret"},
		"candidate": {APIKey: "candidate-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(projectStore.Directory(), project.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "primary-secret") || strings.Contains(string(raw), "candidate-secret") {
		t.Fatalf("project file must not contain credentials: %s", raw)
	}
	if !strings.Contains(credentialStore.values[project.ID], "primary-secret") ||
		!strings.Contains(credentialStore.values[project.ID], "candidate-secret") {
		t.Fatal("credentials were not stored in the credential store")
	}
	settings, err := app.GetProjectAuth(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Credentials["primary"].Token != "primary-secret" ||
		settings.Credentials["candidate"].APIKey != "candidate-secret" {
		t.Fatalf("unexpected auth settings returned to editor: %#v", settings)
	}
}

func TestAppAuthSaveDoesNotRequireCredentialStoreWhenAuthDisabled(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	app.credentials = &memoryCredentialStore{err: errors.New("keyring unavailable")}
	if _, err := app.SaveProjectWithAuth(Project{
		Name: "Public API", Primary: "https://stable.test", Candidate: "https://candidate.test",
	}, nil); err != nil {
		t.Fatalf("disabled auth should not access the keyring: %v", err)
	}
}

func TestAppDisablingAuthenticationDeletesStoredCredentials(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	credentialStore := &memoryCredentialStore{values: map[string]string{}}
	app.credentials = credentialStore
	project, err := app.SaveProjectWithAuth(Project{
		Name: "Private API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{"primary": {Type: "bearer"}},
	}, map[string]domain.AuthCredentials{"primary": {Token: "saved-token"}})
	if err != nil {
		t.Fatal(err)
	}

	project.Auth["primary"] = domain.DestinationAuth{Type: "none"}
	if _, err := app.SaveProjectWithAuth(project, map[string]domain.AuthCredentials{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := credentialStore.values[project.ID]; ok {
		t.Fatal("credentials should be removed when authentication is disabled")
	}
	if hasConfiguredAuth(app.GetProjects()[0].Auth) {
		t.Fatal("authentication metadata should be disabled")
	}
}

func TestAppImportProjectDoesNotImportAuthenticationMetadata(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	raw := `{"id":"imported-auth","name":"Imported","primary":"https://stable.test","candidate":"https://candidate.test","auth":{"primary":{"type":"bearer"}}}`
	imported, err := app.ImportProjectConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if hasConfiguredAuth(imported.Auth) {
		t.Fatalf("imported project should require credentials to be configured locally: %#v", imported.Auth)
	}
}

func TestAppAuthCanBeReconfiguredWhenStoredCredentialsAreMissing(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	credentialStore := &memoryCredentialStore{values: map[string]string{}}
	app.credentials = credentialStore
	project, err := app.SaveProjectWithAuth(Project{
		Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{"primary": {Type: "bearer"}},
	}, map[string]domain.AuthCredentials{"primary": {Token: "original-token"}})
	if err != nil {
		t.Fatal(err)
	}
	delete(credentialStore.values, project.ID)
	settings, err := app.GetProjectAuth(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.CredentialsMissing || settings.Auth["primary"].Type != "bearer" {
		t.Fatalf("missing keyring entry should preserve editable auth metadata: %#v", settings)
	}
}

func TestAppDeleteProjectRemovesProjectFileAndCredentials(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	credentialStore := &memoryCredentialStore{values: map[string]string{}}
	app.credentials = credentialStore
	project, err := app.SaveProjectWithAuth(Project{
		Name: "Private API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{"primary": {Type: "bearer"}},
	}, map[string]domain.AuthCredentials{"primary": {Token: "saved-token"}})
	if err != nil {
		t.Fatal(err)
	}

	if err := app.DeleteProject(project.ID); err != nil {
		t.Fatal(err)
	}
	if got := app.GetProjects(); len(got) != 0 {
		t.Fatalf("deleted project remains in store: %#v", got)
	}
	if _, exists := credentialStore.values[project.ID]; exists {
		t.Fatal("project credentials were not deleted")
	}
	if _, err := os.Stat(filepath.Join(projectStore.Directory(), project.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("project file still exists, stat error = %v", err)
	}
	if err := app.DeleteProject(project.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleting missing project error = %v, want os.ErrNotExist", err)
	}
}

func TestAppDeleteCollectionRemovesOnlyRequestedCollection(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	project, err := app.SaveProject(Project{
		Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := app.SaveCollection(project.ID, Collection{ID: "first", Name: "First"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveCollection(project.ID, Collection{ID: "second", Name: "Second"}); err != nil {
		t.Fatal(err)
	}

	if err := app.DeleteCollection(project.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	saved := app.GetProjects()
	if len(saved) != 1 || len(saved[0].Collections) != 1 || saved[0].Collections[0].ID != "second" {
		t.Fatalf("deleting a collection changed unexpected data: %#v", saved)
	}
	if err := app.DeleteCollection(project.ID, "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleting missing collection error = %v, want os.ErrNotExist", err)
	}
	if err := app.DeleteCollection("missing-project", "second"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleting collection from missing project error = %v, want os.ErrNotExist", err)
	}
}

func TestAppRunRequestValidatesRequestAndUsesDestinationCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer run-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	credentialStore := &memoryCredentialStore{values: map[string]string{}}
	app.credentials = credentialStore
	project, err := app.SaveProjectWithAuth(Project{
		Name: "Private API", Primary: server.URL, Candidate: server.URL,
		Auth: map[string]domain.DestinationAuth{
			"primary": {Type: "bearer"}, "candidate": {Type: "bearer"},
		},
	}, map[string]domain.AuthCredentials{
		"primary": {Token: "run-secret"}, "candidate": {Token: "run-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}

	request := Request{Name: "Health", Method: http.MethodGet, Path: "/health"}
	result, err := app.RunRequest(project.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Primary.StatusCode != http.StatusOK || result.Candidate.StatusCode != http.StatusOK {
		t.Fatalf("request did not use stored credentials: %#v", result)
	}

	if _, err := app.RunRequest(project.ID, Request{Name: "Invalid", Method: "TRACE", Path: "/"}); err == nil {
		t.Fatal("RunRequest accepted an unsupported HTTP method")
	}
	delete(credentialStore.values, project.ID)
	if _, err := app.RunRequest(project.ID, request); err == nil ||
		!strings.Contains(err.Error(), "credentials are missing") {
		t.Fatalf("RunRequest with missing credentials error = %v", err)
	}
	if _, err := app.RunRequest("missing-project", request); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("RunRequest for missing project error = %v, want os.ErrNotExist", err)
	}
}

func TestAppProjectSaveAndAuthenticationValidation(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	credentialStore := &memoryCredentialStore{values: map[string]string{}}
	app.credentials = credentialStore

	project, err := app.SaveProject(Project{
		Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	project.Auth = map[string]domain.DestinationAuth{"primary": {Type: "bearer"}}
	if _, err := app.SaveProject(project); err == nil ||
		!strings.Contains(err.Error(), "SaveProjectWithAuth") {
		t.Fatalf("SaveProject() auth change error = %v, want secure auth API guidance", err)
	}
	if _, err := app.SaveProject(Project{
		Name: "New secured API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{"primary": {Type: "bearer"}},
	}); err == nil || !strings.Contains(err.Error(), "SaveProjectWithAuth") {
		t.Fatalf("SaveProject() new auth error = %v, want secure auth API guidance", err)
	}

	valid := Project{
		Name: "Private API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{"primary": {Type: "bearer"}},
	}
	if _, err := app.SaveProjectWithAuth(valid, map[string]domain.AuthCredentials{
		"unknown": {Token: "unused"},
	}); err == nil || !strings.Contains(err.Error(), "credential target") {
		t.Fatalf("SaveProjectWithAuth() unknown credential target error = %v", err)
	}
	if _, err := app.SaveProjectWithAuth(valid, nil); err == nil ||
		!strings.Contains(err.Error(), "bearer token is required") {
		t.Fatalf("SaveProjectWithAuth() missing credential error = %v", err)
	}

	secondaryOnly := Project{
		Name: "Two destinations", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{"secondary": {Type: "bearer"}},
	}
	saved, err := app.SaveProjectWithAuth(secondaryOnly, map[string]domain.AuthCredentials{
		"secondary": {Token: "removed-without-destination"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := saved.Auth["secondary"]; exists {
		t.Fatalf("secondary auth remained configured without a secondary destination: %#v", saved.Auth)
	}
}

func TestAppGetProjectAuthReportsKeyringFailures(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	credentialStore := &memoryCredentialStore{values: map[string]string{}}
	app.credentials = credentialStore
	project, err := app.SaveProjectWithAuth(Project{
		Name: "Private API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{"primary": {Type: "bearer"}},
	}, map[string]domain.AuthCredentials{"primary": {Token: "saved-token"}})
	if err != nil {
		t.Fatal(err)
	}

	credentialStore.err = errors.New("keyring unavailable")
	if _, err := app.GetProjectAuth(project.ID); err == nil ||
		!strings.Contains(err.Error(), "keyring unavailable") {
		t.Fatalf("GetProjectAuth() backend error = %v, want keyring error", err)
	}
	credentialStore.err = nil
	credentialStore.values[project.ID] = "{"
	if _, err := app.GetProjectAuth(project.ID); err == nil ||
		!strings.Contains(err.Error(), "decode credentials") {
		t.Fatalf("GetProjectAuth() malformed data error = %v, want decode error", err)
	}
	if _, err := app.GetProjectAuth("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GetProjectAuth() missing project error = %v, want os.ErrNotExist", err)
	}
}

func TestAppDeleteProjectPreservesProjectWhenKeyringFails(t *testing.T) {
	projectStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(projectStore)
	credentialStore := &memoryCredentialStore{values: map[string]string{}}
	app.credentials = credentialStore
	project, err := app.SaveProjectWithAuth(Project{
		Name: "Private API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]domain.DestinationAuth{"primary": {Type: "bearer"}},
	}, map[string]domain.AuthCredentials{"primary": {Token: "saved-token"}})
	if err != nil {
		t.Fatal(err)
	}

	credentialStore.err = errors.New("keyring unavailable")
	if err := app.DeleteProject(project.ID); err == nil ||
		!strings.Contains(err.Error(), "keyring unavailable") {
		t.Fatalf("DeleteProject() keyring error = %v, want keyring failure", err)
	}
	if len(app.GetProjects()) != 1 {
		t.Fatal("project was removed even though the credential store failed")
	}
}
