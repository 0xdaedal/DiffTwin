package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jrcasanya/difftwin/internal/domain"
)

type Store struct {
	mu         sync.RWMutex
	path       string
	legacyPath string
	projects   []domain.Project
}

func New(path string) (*Store, error) {
	s := &Store{path: path, projects: []domain.Project{}}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		s.legacyPath = path
		s.path = strings.TrimSuffix(path, filepath.Ext(path)) + ".d"
	} else {
		s.legacyPath = filepath.Join(filepath.Dir(path), "projects.json")
	}
	entries, err := os.ReadDir(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read project directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		filePath := filepath.Join(s.path, entry.Name())
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read project %q: %w", entry.Name(), err)
		}
		var project domain.Project
		if err := json.Unmarshal(raw, &project); err != nil {
			return nil, fmt.Errorf("parse project %q: %w", entry.Name(), err)
		}
		if project.ID == "" || entry.Name() != project.ID+".json" {
			return nil, fmt.Errorf("project file %q must be named after its project ID", entry.Name())
		}
		s.projects = append(s.projects, project)
	}
	migrationMarker := s.legacyPath + ".migrated"
	if _, err := os.Stat(migrationMarker); errors.Is(err, os.ErrNotExist) {
		raw, err := os.ReadFile(s.legacyPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read legacy project data: %w", err)
		}
		if err == nil {
			var legacy struct {
				Projects []domain.Project `json:"projects"`
			}
			if err := json.Unmarshal(raw, &legacy); err != nil {
				return nil, fmt.Errorf("parse legacy project data: %w", err)
			}
			existingIDs := make(map[string]struct{}, len(s.projects))
			for _, project := range s.projects {
				existingIDs[project.ID] = struct{}{}
			}
			for _, project := range legacy.Projects {
				if _, exists := existingIDs[project.ID]; !exists {
					s.projects = append(s.projects, project)
				}
			}
			if err := s.persist(s.projects); err != nil {
				return nil, fmt.Errorf("migrate legacy project data: %w", err)
			}
			if err := os.WriteFile(migrationMarker, []byte("migrated\n"), 0o600); err != nil {
				return nil, fmt.Errorf("mark legacy project migration: %w", err)
			}
		}
	} else if err != nil {
		return nil, fmt.Errorf("check legacy migration marker: %w", err)
	}
	return s, nil
}

func (s *Store) Snapshot() []domain.Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return domain.CloneProjects(s.projects)
}

func (s *Store) Directory() string {
	return s.path
}

func (s *Store) Update(fn func(*[]domain.Project) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	original := domain.CloneProjects(s.projects)
	if err := fn(&s.projects); err != nil {
		s.projects = original
		return err
	}
	if err := s.persist(s.projects); err != nil {
		s.projects = original
		return err
	}
	return nil
}

func (s *Store) ImportProject(project domain.Project) error {
	if err := domain.PrepareProject(&project); err != nil {
		return err
	}
	if project.ID == "" {
		project.ID = domain.NewID()
	}
	return s.Update(func(projects *[]domain.Project) error {
		for _, existing := range *projects {
			if existing.ID == project.ID {
				return fmt.Errorf("a project with ID %q is already imported", project.ID)
			}
		}
		*projects = append(*projects, project)
		return nil
	})
}

func (s *Store) persist(projects []domain.Project) error {
	if err := os.MkdirAll(s.path, 0o700); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}
	keep := make(map[string]struct{}, len(projects))
	for _, project := range projects {
		if project.ID == "" || filepath.Base(project.ID) != project.ID || strings.ContainsAny(project.ID, `/\`) {
			return fmt.Errorf("invalid project ID %q", project.ID)
		}
		raw, err := json.MarshalIndent(project, "", "  ")
		if err != nil {
			return fmt.Errorf("encode project %q: %w", project.ID, err)
		}
		filePath := filepath.Join(s.path, project.ID+".json")
		tmpPath := filePath + ".tmp"
		if err := os.WriteFile(tmpPath, raw, 0o600); err != nil {
			return fmt.Errorf("write project %q: %w", project.Name, err)
		}
		if err := os.Rename(tmpPath, filePath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("save project %q: %w", project.Name, err)
		}
		keep[project.ID+".json"] = struct{}{}
	}
	entries, err := os.ReadDir(s.path)
	if err != nil {
		return fmt.Errorf("list project files: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if _, ok := keep[entry.Name()]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(s.path, entry.Name())); err != nil {
			return fmt.Errorf("remove deleted project file %q: %w", entry.Name(), err)
		}
	}
	return nil
}
