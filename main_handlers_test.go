package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrcasanya/difftwin/internal/storage"
)

func newTestServer(t *testing.T) *server {
	t.Helper()
	store, err := storage.New(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &server{store: store, client: http.DefaultClient}
}

func serveRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestProjectsEndpointListsCreatesAndValidates(t *testing.T) {
	server := newTestServer(t)
	handler := server.routes()

	list := serveRequest(handler, http.MethodGet, "/api/projects", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "[]") {
		t.Fatalf("GET projects = %d %s, want empty JSON array", list.Code, list.Body.String())
	}
	created := serveRequest(handler, http.MethodPost, "/api/projects", `{
		"name":"  Users API  ",
		"primary":" https://stable.test ",
		"candidate":"https://candidate.test"
	}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST projects = %d %s, want 201", created.Code, created.Body.String())
	}
	var project Project
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if project.ID == "" || project.Name != "Users API" || project.Primary != "https://stable.test" {
		t.Fatalf("created project was not normalized: %#v", project)
	}

	for _, test := range []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{name: "invalid JSON", method: http.MethodPost, body: `{`, want: http.StatusBadRequest},
		{name: "unknown field", method: http.MethodPost, body: `{"unknown":true}`, want: http.StatusBadRequest},
		{name: "multiple JSON values", method: http.MethodPost, body: `{} {}`, want: http.StatusBadRequest},
		{name: "invalid project", method: http.MethodPost, body: `{"name":"missing destinations"}`, want: http.StatusBadRequest},
		{name: "unsupported method", method: http.MethodDelete, want: http.StatusMethodNotAllowed},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serveRequest(handler, test.method, "/api/projects", test.body)
			if response.Code != test.want {
				t.Fatalf("response = %d %s, want %d", response.Code, response.Body.String(), test.want)
			}
		})
	}
}

func TestProjectEndpointUpdatesDeletesAndCreatesCollections(t *testing.T) {
	server := newTestServer(t)
	handler := server.routes()
	created := serveRequest(handler, http.MethodPost, "/api/projects", `{
		"name":"Users API","primary":"https://stable.test","candidate":"https://candidate.test"
	}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create project = %d %s", created.Code, created.Body.String())
	}
	var project Project
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if err := server.store.Update(func(projects *[]Project) error {
		(*projects)[0].Collections = []Collection{{ID: "original", Name: "Original"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	updated := serveRequest(handler, http.MethodPut, "/api/projects/"+project.ID, `{
		"name":"Renamed API","primary":"https://new-stable.test","candidate":"https://candidate.test"
	}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("PUT project = %d %s", updated.Code, updated.Body.String())
	}
	saved := server.store.Snapshot()[0]
	if saved.Name != "Renamed API" || len(saved.Collections) != 1 || saved.Collections[0].ID != "original" {
		t.Fatalf("project update did not preserve collection: %#v", saved)
	}

	collection := serveRequest(handler, http.MethodPost, "/api/projects/"+project.ID+"/collections", `{"name":"  Users  "}`)
	if collection.Code != http.StatusCreated {
		t.Fatalf("POST collection = %d %s", collection.Code, collection.Body.String())
	}
	var savedCollection Collection
	if err := json.Unmarshal(collection.Body.Bytes(), &savedCollection); err != nil {
		t.Fatal(err)
	}
	if savedCollection.ID == "" || savedCollection.Name != "Users" {
		t.Fatalf("created collection was not normalized: %#v", savedCollection)
	}

	replacedCollection := serveRequest(handler, http.MethodPut,
		"/api/projects/"+project.ID+"/collections/"+savedCollection.ID,
		`{"name":"  Renamed users  "}`)
	if replacedCollection.Code != http.StatusOK {
		t.Fatalf("PUT collection = %d %s", replacedCollection.Code, replacedCollection.Body.String())
	}
	saved = server.store.Snapshot()[0]
	if saved.Collections[1].Name != "Renamed users" || saved.Collections[1].ID != savedCollection.ID {
		t.Fatalf("collection update was not normalized or preserved its ID: %#v", saved.Collections[1])
	}

	deletedCollection := serveRequest(handler, http.MethodDelete,
		"/api/projects/"+project.ID+"/collections/"+savedCollection.ID, "")
	if deletedCollection.Code != http.StatusNoContent {
		t.Fatalf("DELETE collection = %d %s, want 204", deletedCollection.Code, deletedCollection.Body.String())
	}
	deletedProject := serveRequest(handler, http.MethodDelete, "/api/projects/"+project.ID, "")
	if deletedProject.Code != http.StatusNoContent || len(server.store.Snapshot()) != 0 {
		t.Fatalf("DELETE project = %d %s, remaining=%d", deletedProject.Code, deletedProject.Body.String(), len(server.store.Snapshot()))
	}
}

func TestProjectEndpointsReportMissingResourcesAndInvalidMethods(t *testing.T) {
	handler := newTestServer(t).routes()
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{name: "project missing", method: http.MethodPut, path: "/api/projects/missing", body: `{"name":"API","primary":"https://stable.test","candidate":"https://candidate.test"}`, want: http.StatusNotFound},
		{name: "delete project missing", method: http.MethodDelete, path: "/api/projects/missing", want: http.StatusNotFound},
		{name: "collection create project missing", method: http.MethodPost, path: "/api/projects/missing/collections", body: `{"name":"users"}`, want: http.StatusNotFound},
		{name: "collection route missing project", method: http.MethodDelete, path: "/api/projects/missing/collections/collection", want: http.StatusNotFound},
		{name: "project unsupported method", method: http.MethodGet, path: "/api/projects/id", want: http.StatusMethodNotAllowed},
		{name: "project path missing ID", method: http.MethodDelete, path: "/api/projects/", want: http.StatusNotFound},
		{name: "collection path unsupported method", method: http.MethodGet, path: "/api/projects/id/collections/collection", want: http.StatusMethodNotAllowed},
		{name: "unknown project subroute", method: http.MethodPost, path: "/api/projects/id/unknown", want: http.StatusMethodNotAllowed},
		{name: "unknown project path depth", method: http.MethodGet, path: "/api/projects/id/collections/id/nested", want: http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveRequest(handler, test.method, test.path, test.body)
			if response.Code != test.want {
				t.Fatalf("response = %d %s, want %d", response.Code, response.Body.String(), test.want)
			}
		})
	}
}

func TestCollectionEndpointRejectsInvalidInputAndMissingCollections(t *testing.T) {
	server := newTestServer(t)
	handler := server.routes()
	project := Project{ID: "project", Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test"}
	if err := server.store.Update(func(projects *[]Project) error {
		*projects = append(*projects, project)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{name: "blank collection name", method: http.MethodPost, path: "/api/projects/project/collections", body: `{"name":" "}`, want: http.StatusBadRequest},
		{name: "malformed collection", method: http.MethodPost, path: "/api/projects/project/collections", body: `{`, want: http.StatusBadRequest},
		{name: "blank replacement name", method: http.MethodPut, path: "/api/projects/project/collections/missing", body: `{"name":" "}`, want: http.StatusBadRequest},
		{name: "delete missing collection", method: http.MethodDelete, path: "/api/projects/project/collections/missing", want: http.StatusNotFound},
		{name: "replace missing collection", method: http.MethodPut, path: "/api/projects/project/collections/missing", body: `{"name":"users"}`, want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serveRequest(handler, test.method, test.path, test.body)
			if response.Code != test.want {
				t.Fatalf("response = %d %s, want %d", response.Code, response.Body.String(), test.want)
			}
		})
	}
}

func TestRunEndpointRejectsInvalidMethodInputAndProject(t *testing.T) {
	server := newTestServer(t)
	handler := server.routes()

	for _, test := range []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{name: "unsupported method", method: http.MethodGet, want: http.StatusMethodNotAllowed},
		{name: "invalid JSON", method: http.MethodPost, body: `{`, want: http.StatusBadRequest},
		{name: "unknown field", method: http.MethodPost, body: `{"unexpected":true}`, want: http.StatusBadRequest},
		{name: "missing project", method: http.MethodPost, body: `{"project_id":"missing","request":{"name":"Health","method":"GET","path":"/"}}`, want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serveRequest(handler, test.method, "/api/run", test.body)
			if response.Code != test.want {
				t.Fatalf("response = %d %s, want %d", response.Code, response.Body.String(), test.want)
			}
		})
	}
}
