package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrcasanya/difftwin/internal/domain"
)

func TestStoreUpdatePersistsProjectsAndSnapshotIsIsolated(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	project := domain.Project{
		ID: "project", Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Variables: map[string]string{"region": "eu"},
		Collections: []domain.Collection{{
			ID: "collection",
			Requests: []domain.Request{{
				ID: "request", Headers: map[string]string{"X-Test": "before"},
			}},
		}},
	}
	if err := store.Update(func(projects *[]domain.Project) error {
		*projects = append(*projects, project)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	snapshot := store.Snapshot()
	snapshot[0].Variables["region"] = "changed"
	snapshot[0].Collections[0].Requests[0].Headers["X-Test"] = "changed"
	if got := store.Snapshot()[0]; got.Variables["region"] != "eu" ||
		got.Collections[0].Requests[0].Headers["X-Test"] != "before" {
		t.Fatalf("snapshot mutation changed stored project: %#v", got)
	}

	reloaded, err := New(store.Directory())
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Snapshot()[0]; got.ID != project.ID || got.Name != project.Name ||
		got.Collections[0].Requests[0].Headers["X-Test"] != "before" {
		t.Fatalf("unexpected project after reload: %#v", got)
	}
}

func TestStoreUpdateRollsBackWhenCallbackFails(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	project := domain.Project{ID: "project", Name: "Before"}
	if err := store.Update(func(projects *[]domain.Project) error {
		*projects = append(*projects, project)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("reject update")
	err = store.Update(func(projects *[]domain.Project) error {
		(*projects)[0].Name = "After"
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Update error = %v, want %v", err, wantErr)
	}
	if got := store.Snapshot()[0].Name; got != "Before" {
		t.Fatalf("failed update changed in-memory state: %q", got)
	}
	reloaded, err := New(store.Directory())
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Snapshot()[0].Name; got != "Before" {
		t.Fatalf("failed update changed persisted state: %q", got)
	}
}

func TestStoreUpdateRollsBackWhenPersistenceFails(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(projects *[]domain.Project) error {
		*projects = append(*projects, domain.Project{ID: "project", Name: "Before"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	unwritablePath := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(unwritablePath, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.path = filepath.Join(unwritablePath, "projects")
	err = store.Update(func(projects *[]domain.Project) error {
		(*projects)[0].Name = "After"
		return nil
	})
	if err == nil {
		t.Fatal("Update succeeded although its storage directory cannot be created")
	}
	if got := store.Snapshot()[0].Name; got != "Before" {
		t.Fatalf("persistence failure changed in-memory state: %q", got)
	}
}

func TestStoreNewRejectsMalformedAndMisnamedProjectFiles(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		wantErr string
	}{
		{
			name: "malformed JSON", file: "project.json", content: "{",
			wantErr: "parse project",
		},
		{
			name: "file does not match project ID", file: "other.json",
			content: `{"id":"project","name":"API"}`,
			wantErr: "must be named after its project ID",
		},
		{
			name: "missing project ID", file: ".json",
			content: `{"name":"API"}`,
			wantErr: "must be named after its project ID",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			storageDirectory := filepath.Join(directory, "projects.d")
			if err := os.MkdirAll(storageDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(storageDirectory, test.file)
			if err := os.WriteFile(file, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := New(filepath.Join(directory, "projects.json"))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("New() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestStoreImportProjectValidatesAndRejectsDuplicateIDs(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	project := domain.Project{
		ID: "project", Name: " API ", Primary: " https://stable.test ",
		Candidate: "https://candidate.test",
	}
	if err := store.ImportProject(project); err != nil {
		t.Fatal(err)
	}
	if got := store.Snapshot()[0]; got.Name != "API" || got.Primary != "https://stable.test" {
		t.Fatalf("project was not prepared before import: %#v", got)
	}
	if err := store.ImportProject(project); err == nil {
		t.Fatal("duplicate project ID was imported")
	}
	if err := store.ImportProject(domain.Project{ID: "invalid", Name: "Invalid"}); err == nil {
		t.Fatal("project with invalid destinations was imported")
	}
	if len(store.Snapshot()) != 1 {
		t.Fatalf("failed imports changed the project list: %#v", store.Snapshot())
	}
}

func TestStorePersistsWithRestrictivePermissionsAndRemovesDeletedFiles(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(projects *[]domain.Project) error {
		*projects = append(*projects, domain.Project{ID: "project", Name: "API"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	projectFile := filepath.Join(store.Directory(), "project.json")
	info, err := os.Stat(projectFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("project file permissions = %04o, want 0600", got)
	}

	if err := store.Update(func(projects *[]domain.Project) error {
		*projects = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projectFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted project file still exists, stat error = %v", err)
	}
	entries, err := os.ReadDir(store.Directory())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temporary project file was left behind: %s", entry.Name())
		}
	}
}

func TestStoreLoadsOnlyProjectJSONFiles(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "projects.d")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(domain.Project{ID: "project", Name: "API"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "project.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "nested.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	store, err := New(filepath.Join(filepath.Dir(directory), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Snapshot(); len(got) != 1 || got[0].ID != "project" {
		t.Fatalf("unexpected projects loaded from directory: %#v", got)
	}
}

func TestStoreNewReportsMalformedLegacyFileAndSkipsCompletedMigration(t *testing.T) {
	t.Run("malformed legacy file", func(t *testing.T) {
		directory := t.TempDir()
		legacyPath := filepath.Join(directory, "projects.json")
		if err := os.WriteFile(legacyPath, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := New(legacyPath)
		if err == nil || !strings.Contains(err.Error(), "parse legacy project data") {
			t.Fatalf("New() error = %v, want malformed legacy data error", err)
		}
	})

	t.Run("migration marker skips legacy import", func(t *testing.T) {
		directory := t.TempDir()
		legacyPath := filepath.Join(directory, "projects.json")
		if err := os.WriteFile(legacyPath, []byte(`{"projects":[{"id":"legacy","name":"Legacy"}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacyPath+".migrated", []byte("migrated\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		store, err := New(legacyPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(store.Snapshot()) != 0 {
			t.Fatalf("completed migration reimported legacy projects: %#v", store.Snapshot())
		}
	})
}

func TestStoreNewMigrationKeepsDirectoryProjectsOverLegacyDuplicates(t *testing.T) {
	directory := t.TempDir()
	legacyPath := filepath.Join(directory, "projects.json")
	projectDirectory := filepath.Join(directory, "projects.d")
	if err := os.MkdirAll(projectDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	current, err := json.Marshal(domain.Project{ID: "shared", Name: "Current"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDirectory, "shared.json"), current, 0o600); err != nil {
		t.Fatal(err)
	}
	legacy, err := json.Marshal(struct {
		Projects []domain.Project `json:"projects"`
	}{Projects: []domain.Project{
		{ID: "shared", Name: "Legacy duplicate"},
		{ID: "legacy-only", Name: "Legacy only"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := New(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	projects := store.Snapshot()
	if len(projects) != 2 || projects[0].Name != "Current" || projects[1].ID != "legacy-only" {
		t.Fatalf("migration did not prefer existing files and import new IDs: %#v", projects)
	}
}

func TestStoreUpdateRollsBackWhenProjectFileCannotBeWritten(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(projects *[]domain.Project) error {
		*projects = append(*projects, domain.Project{ID: "blocked", Name: "Before"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store.Directory(), "blocked.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(store.Directory(), "blocked.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}

	err = store.Update(func(projects *[]domain.Project) error {
		(*projects)[0].Name = "After"
		return nil
	})
	if err == nil {
		t.Fatal("Update succeeded although the project temp-file path is a directory")
	}
	if got := store.Snapshot()[0].Name; got != "Before" {
		t.Fatalf("failed write changed in-memory state: %q", got)
	}
}
