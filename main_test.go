package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrcasanya/difftwin/internal/comparison"
	"github.com/jrcasanya/difftwin/internal/domain"
	"github.com/jrcasanya/difftwin/internal/storage"
)

func TestBuildTargetURL(t *testing.T) {
	got, err := comparison.BuildTargetURL("https://example.test/api/", "/users?active=true")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.test/users?active=true" {
		t.Fatalf("unexpected URL: %s", got)
	}
}

func TestBuildTargetURLRejectsAbsoluteRequestURL(t *testing.T) {
	if _, err := comparison.BuildTargetURL("https://example.test", "https://other.test/private"); err == nil {
		t.Fatal("expected absolute request URL to be rejected")
	}
}

func TestBodiesDifferIgnoresJSONObjectKeyOrder(t *testing.T) {
	if comparison.BodiesDiffer(`{"a":1,"b":2}`, `{"b":2,"a":1}`) {
		t.Fatal("JSON key order should not create a difference")
	}
}

func TestResponsesDifferIncludesStatusAndBody(t *testing.T) {
	base := responseResult{StatusCode: http.StatusOK, Body: `{"ok":true}`}
	if comparison.ResponsesDiffer(base, responseResult{StatusCode: http.StatusCreated, Body: base.Body}) != true {
		t.Fatal("status code difference was not detected")
	}
	if comparison.ResponsesDiffer(base, responseResult{StatusCode: base.StatusCode, Body: `{"ok":false}`}) != true {
		t.Fatal("body difference was not detected")
	}
	if comparison.ResponsesDiffer(base, responseResult{StatusCode: base.StatusCode, Body: base.Body}) {
		t.Fatal("identical responses were reported as different")
	}
}

func TestInterpolate(t *testing.T) {
	got := domain.Interpolate("/users/{{userId}}?token={{token}}", map[string]string{
		"userId": "42",
		"token":  "abc",
	})
	if got != "/users/42?token=abc" {
		t.Fatalf("unexpected interpolation: %s", got)
	}
}

func TestRunComparesThreeDestinations(t *testing.T) {
	newTarget := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
	}
	primary := newTarget(`{"value":"old"}`)
	defer primary.Close()
	candidate := newTarget(`{"value":"new"}`)
	defer candidate.Close()
	secondary := newTarget(`{"value":"old"}`)
	defer secondary.Close()

	data, err := storage.New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	project := Project{
		ID:        "project-1",
		Primary:   primary.URL,
		Candidate: candidate.URL,
		Secondary: secondary.URL,
	}
	if err := data.Update(func(projects *[]Project) error {
		*projects = append(*projects, project)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(runInput{
		ProjectID: project.ID,
		Request:   Request{Name: "test", Method: http.MethodGet, Path: "/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	app := &server{store: data, client: http.DefaultClient}
	recorder := httptest.NewRecorder()
	app.run(recorder, httptest.NewRequest(http.MethodPost, "/api/run", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d (%s)", recorder.Code, recorder.Body.String())
	}
	var result runResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.CandidateDiffers {
		t.Fatal("expected candidate difference")
	}
	if result.NoiseDetected {
		t.Fatal("identical stable destinations should not be marked noisy")
	}
}

func TestStoreWritesOneFilePerProjectAndReloads(t *testing.T) {
	data, err := storage.New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	projects := []Project{
		{ID: "project-one", Name: "One", Primary: "https://one.test", Candidate: "https://one.test", Secondary: "https://one.test"},
		{ID: "project-two", Name: "Two", Primary: "https://two.test", Candidate: "https://two.test", Secondary: "https://two.test"},
	}
	if err := data.Update(func(saved *[]Project) error {
		*saved = append(*saved, projects...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(data.Directory(), "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(projects) {
		t.Fatalf("expected %d project files, got %d", len(projects), len(files))
	}
	reloaded, err := storage.New(data.Directory())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(reloaded.Snapshot()); got != len(projects) {
		t.Fatalf("expected %d reloaded projects, got %d", len(projects), got)
	}
}

func TestStoreMigratesLegacyProjectsFile(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "projects.json")
	legacy := database{Projects: []Project{{
		ID: "legacy-project", Name: "Legacy", Primary: "https://api.test",
		Candidate: "https://candidate.test", Secondary: "https://stable-2.test",
	}}}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := storage.New(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Snapshot()) != 1 {
		t.Fatalf("expected legacy project migration, got %d projects", len(data.Snapshot()))
	}
	if _, err := os.Stat(filepath.Join(data.Directory(), "legacy-project.json")); err != nil {
		t.Fatalf("expected migrated project file: %v", err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy source file should remain as backup: %v", err)
	}
	if err := data.Update(func(projects *[]Project) error {
		*projects = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := storage.New(data.Directory())
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Snapshot()) != 0 {
		t.Fatal("deleted migrated project was restored from the legacy backup")
	}
}

func TestImportProjectConfiguration(t *testing.T) {
	data, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := Project{
		ID: "imported-project", Name: "Imported", Primary: "https://api.test",
		Candidate: "https://candidate.test", Secondary: "https://stable-2.test",
	}
	raw, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeProjectConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.ImportProject(decoded); err != nil {
		t.Fatal(err)
	}
	if err := data.ImportProject(decoded); err == nil {
		t.Fatal("expected duplicate project ID to be rejected")
	}
	if got := data.Snapshot(); len(got) != 1 || got[0].Name != project.Name {
		t.Fatalf("unexpected imported projects: %+v", got)
	}
}

func TestRootServesEmbeddedWebApp(t *testing.T) {
	data, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &server{store: data, client: http.DefaultClient}
	recorder := httptest.NewRecorder()
	app.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	body, err := io.ReadAll(recorder.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "<title>DiffTwin</title>") {
		t.Fatal("embedded UI was not served")
	}
}

func TestResolveProjectDataPathPrefersDiffTwinEnvironmentVariable(t *testing.T) {
	t.Setenv("DIFFTWIN_DATA", "/tmp/difftwin-projects")
	t.Setenv("DIFFBENCH_DATA", "/tmp/diffbench-projects")

	got, err := resolveProjectDataPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/difftwin-projects" {
		t.Fatalf("resolveProjectDataPath() = %q, want DiffTwin override", got)
	}
}

func TestResolveProjectDataPathSupportsLegacyEnvironmentVariable(t *testing.T) {
	t.Setenv("DIFFTWIN_DATA", "")
	t.Setenv("DIFFBENCH_DATA", "/tmp/diffbench-projects")

	got, err := resolveProjectDataPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/diffbench-projects" {
		t.Fatalf("resolveProjectDataPath() = %q, want legacy override", got)
	}
}

func TestDefaultProjectDataPathPreservesExistingDiffBenchData(t *testing.T) {
	configDir := t.TempDir()
	legacyPath := filepath.Join(configDir, "DiffBench", "projects")
	if err := os.MkdirAll(legacyPath, 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := defaultProjectDataPath(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != legacyPath {
		t.Fatalf("defaultProjectDataPath() = %q, want existing legacy path %q", got, legacyPath)
	}
}

func TestDefaultProjectDataPathUsesDiffTwinForNewInstall(t *testing.T) {
	configDir := t.TempDir()

	got, err := defaultProjectDataPath(configDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(configDir, "DiffTwin", "projects")
	if got != want {
		t.Fatalf("defaultProjectDataPath() = %q, want %q", got, want)
	}
}
