package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrcasanya/difftwin/internal/comparison"
	"github.com/jrcasanya/difftwin/internal/domain"
	"github.com/jrcasanya/difftwin/internal/storage"
)

//go:embed web/index.html
var webFiles embed.FS

const maxBodySize = comparison.MaxBodySize

type Project = domain.Project
type Collection = domain.Collection
type Folder = domain.Folder
type Request = domain.Request
type store = storage.Store
type responseResult = comparison.Response
type runResult = comparison.Result

type database struct {
	Projects []Project `json:"projects"`
}

type server struct {
	store  *store
	client *http.Client
}

type runInput struct {
	ProjectID string  `json:"project_id"`
	Request   Request `json:"request"`
}

func main() {
	dataPath, err := resolveProjectDataPath()
	if err != nil {
		log.Fatal(err)
	}
	s, err := storage.New(dataPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := prepareWailsLocale(); err != nil {
		log.Fatal(err)
	}
	if err := runWails(s); err != nil {
		log.Fatal(err)
	}
}

func resolveProjectDataPath() (string, error) {
	if dataPath := os.Getenv("DIFFTWIN_DATA"); dataPath != "" {
		return dataPath, nil
	}
	if dataPath := os.Getenv("DIFFBENCH_DATA"); dataPath != "" {
		return dataPath, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return defaultProjectDataPath(configDir)
}

func defaultProjectDataPath(configDir string) (string, error) {
	legacyPath := filepath.Join(configDir, "DiffBench", "projects")
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check legacy project directory: %w", err)
	}
	return filepath.Join(configDir, "DiffTwin", "projects"), nil
}

func decodeProjectConfig(raw []byte) (Project, error) {
	var project Project
	if err := json.Unmarshal(raw, &project); err != nil {
		return Project{}, fmt.Errorf("el archivo no contiene una configuración JSON válida: %w", err)
	}
	if project.ID != "" {
		return project, nil
	}
	var legacy database
	if err := json.Unmarshal(raw, &legacy); err == nil && len(legacy.Projects) == 1 {
		return legacy.Projects[0], nil
	}
	return Project{}, errors.New("el archivo debe contener un proyecto DiffTwin (un objeto Project o un projects.json con un único proyecto)")
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/projects", s.projects)
	mux.HandleFunc("/api/projects/", s.project)
	mux.HandleFunc("/api/run", s.run)
	mux.HandleFunc("/", s.index)
	return mux
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, webFiles, "web/index.html")
}

func (s *server) projects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.Snapshot())
	case http.MethodPost:
		var p Project
		if !decodeJSON(w, r, &p) {
			return
		}
		if err := domain.PrepareProject(&p); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p.ID = domain.NewID()
		if err := s.store.Update(func(projects *[]Project) error {
			*projects = append(*projects, p)
			return nil
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)
	default:
		methodNotAllowed(w)
	}
}

func (s *server) project(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]

	switch {
	case len(parts) == 1 && r.Method == http.MethodPut:
		var replacement Project
		if !decodeJSON(w, r, &replacement) {
			return
		}
		if err := domain.PrepareProject(&replacement); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		replacement.ID = id
		err := s.store.Update(func(projects *[]Project) error {
			for i := range *projects {
				if (*projects)[i].ID == id {
					replacement.Collections = (*projects)[i].Collections
					(*projects)[i] = replacement
					return nil
				}
			}
			return os.ErrNotExist
		})
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
		} else {
			writeJSON(w, http.StatusOK, replacement)
		}
	case len(parts) == 1 && r.Method == http.MethodDelete:
		err := s.store.Update(func(projects *[]Project) error {
			for i := range *projects {
				if (*projects)[i].ID == id {
					*projects = append((*projects)[:i], (*projects)[i+1:]...)
					return nil
				}
			}
			return os.ErrNotExist
		})
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	case len(parts) == 2 && parts[1] == "collections" && r.Method == http.MethodPost:
		var c Collection
		if !decodeJSON(w, r, &c) {
			return
		}
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" {
			writeError(w, http.StatusBadRequest, "collection name is required")
			return
		}
		c.ID = domain.NewID()
		if c.Requests == nil {
			c.Requests = []Request{}
		}
		err := s.store.Update(func(projects *[]Project) error {
			p := domain.FindProject(*projects, id)
			if p == nil {
				return os.ErrNotExist
			}
			p.Collections = append(p.Collections, c)
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
		} else {
			writeJSON(w, http.StatusCreated, c)
		}
	case len(parts) == 3 && parts[1] == "collections":
		s.collection(w, r, id, parts[2])
	default:
		methodNotAllowed(w)
	}
}

func (s *server) collection(w http.ResponseWriter, r *http.Request, projectID, collectionID string) {
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	var replacement Collection
	if r.Method == http.MethodPut {
		if !decodeJSON(w, r, &replacement) {
			return
		}
		replacement.Name = strings.TrimSpace(replacement.Name)
		if replacement.Name == "" {
			writeError(w, http.StatusBadRequest, "collection name is required")
			return
		}
		replacement.ID = collectionID
		if replacement.Requests == nil {
			replacement.Requests = []Request{}
		}
	}
	err := s.store.Update(func(projects *[]Project) error {
		p := domain.FindProject(*projects, projectID)
		if p == nil {
			return os.ErrNotExist
		}
		for i := range p.Collections {
			if p.Collections[i].ID == collectionID {
				if r.Method == http.MethodDelete {
					p.Collections = append(p.Collections[:i], p.Collections[i+1:]...)
				} else {
					p.Collections[i] = replacement
				}
				return nil
			}
		}
		return os.ErrNotExist
	})
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
	} else if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
	} else {
		writeJSON(w, http.StatusOK, replacement)
	}
}

func (s *server) run(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input runInput
	if !decodeJSON(w, r, &input) {
		return
	}
	var project *Project
	for _, p := range s.store.Snapshot() {
		if p.ID == input.ProjectID {
			project = &p
			break
		}
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err := domain.ValidateRequest(input.Request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, comparison.Compare(s.client, *project, input.Request, nil))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON value")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
