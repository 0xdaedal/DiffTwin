package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jrcasanya/difftwin/internal/domain"
)

func TestDecodePostmanCollectionPreservesFoldersAndRelativeURLs(t *testing.T) {
	raw := []byte(`{
		"info":{"name":"Demo API","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
		"variable":[{"key":"token","value":"secret"}],
		"item":[
			{"name":"Health","request":{"method":"GET","url":"https://old.example/health"}},
			{"name":"Users","item":[
				{"name":"Create","request":{"method":"POST","url":{"raw":"https://old.example/v1/users?token={{token}}&active=true"},"header":[{"key":"X-Test","value":"yes"}],"body":{"mode":"raw","raw":"{\"name\":\"Ada\"}","options":{"raw":{"language":"json"}}}}},
				{"name":"Nested","item":[
					{"name":"Show","request":{"method":"GET","url":{"protocol":"https","host":["old","example"],"path":["v1","users","{{id}}"],"query":[{"key":"expand","value":"profile"},{"key":"token","value":"{{token}}"},{"key":"skip","value":"x","disabled":true}]}}}
				]}
			]}
		]
	}`)
	collection, variables, warnings, err := decodePostmanCollection(raw)
	if err != nil {
		t.Fatal(err)
	}
	if collection.Name != "Demo API" || len(collection.Requests) != 1 || len(collection.Folders) != 1 {
		t.Fatalf("unexpected collection structure: %#v", collection)
	}
	if collection.Requests[0].Path != "/health" {
		t.Fatalf("expected host to be discarded, got %q", collection.Requests[0].Path)
	}
	users := collection.Folders[0]
	if users.Name != "Users" || len(users.Requests) != 1 || len(users.Folders) != 1 {
		t.Fatalf("unexpected imported folder structure: %#v", users)
	}
	if users.Requests[0].Path != "/v1/users?token={{token}}&active=true" {
		t.Fatalf("unexpected relative request URL: %q", users.Requests[0].Path)
	}
	if users.Requests[0].Headers["X-Test"] != "yes" || users.Requests[0].Headers["Content-Type"] != "application/json" {
		t.Fatalf("request headers were not imported: %#v", users.Requests[0].Headers)
	}
	nested := users.Folders[0]
	if nested.Requests[0].Path != "/v1/users/{{id}}?expand=profile&token={{token}}" {
		t.Fatalf("unexpected structured URL: %q", nested.Requests[0].Path)
	}
	if variables["token"] != "secret" {
		t.Fatalf("collection variable was not imported: %#v", variables)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected import warnings: %v", warnings)
	}
}

func TestDecodePostmanCollectionReportsUnsupportedFeatures(t *testing.T) {
	raw := []byte(`{
		"info":{"name":"Limited","schema":"https://schema.getpostman.com/json/collection/v2.0.0/collection.json"},
		"item":[{"name":"Upload","event":[{}],"request":{"method":"POST","url":"https://example.test/upload","auth":{"type":"bearer"},"body":{"mode":"formdata"}}}]
	}`)
	_, _, warnings, err := decodePostmanCollection(raw)
	if err != nil {
		t.Fatal(err)
	}
	warningText := strings.Join(warnings, "; ")
	for _, expected := range []string{"scripts pre-request/tests omitidos", "autenticación", "no son raw"} {
		if !strings.Contains(warningText, expected) {
			t.Errorf("expected warning %q in %q", expected, warningText)
		}
	}
}

func TestRelativePostmanURLDiscardsHostAndKeepsVariables(t *testing.T) {
	got, err := relativePostmanURL("https://{{apiHost}}/v1/{{user}}?key={{key}}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/v1/{{user}}?key={{key}}" {
		t.Fatalf("unexpected relative URL: %q", got)
	}
	got, err = relativePostmanURL("{{baseUrl}}/v2/items")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/v2/items" {
		t.Fatalf("variable host was not discarded: %q", got)
	}
}

func TestCollectionFolderRequestsCanBeLookedUpUpdatedAndRemoved(t *testing.T) {
	request := Request{ID: "req", Name: "before"}
	collection := Collection{Folders: []Folder{{ID: "folder", Folders: []Folder{{ID: "nested", Requests: []Request{request}}}}}}
	if domain.RequestByID(&collection, "req") == nil {
		t.Fatal("nested request could not be found")
	}
	if !domain.UpdateRequest(&collection, "req", func(saved *Request) { saved.Name = "after" }) {
		t.Fatal("nested request could not be updated")
	}
	if got := domain.RequestByID(&collection, "req").Name; got != "after" {
		t.Fatalf("request update did not persist: %q", got)
	}
	if !domain.RemoveRequest(&collection, "req") || domain.RequestByID(&collection, "req") != nil {
		t.Fatal("nested request could not be removed")
	}
	if !domain.AppendFolder(&collection.Folders, "folder", Folder{ID: "added"}) || domain.FolderByID(&collection, "added") == nil {
		t.Fatal("nested folder could not be added")
	}
	if !domain.RemoveFolder(&collection.Folders, "folder") || domain.FolderByID(&collection, "added") != nil {
		t.Fatal("nested folder tree could not be removed")
	}
}

func TestNestedFoldersRoundTripThroughJSONAndClone(t *testing.T) {
	project := Project{Collections: []Collection{{
		ID:      "collection",
		Folders: []Folder{{ID: "folder", Requests: []Request{{ID: "request", Headers: map[string]string{"X": "one"}}}}},
	}}}
	raw, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	var restored Project
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	cloned := domain.CloneProjects([]Project{restored})
	cloned[0].Collections[0].Folders[0].Requests[0].Headers["X"] = "changed"
	if restored.Collections[0].Folders[0].Requests[0].Headers["X"] != "one" {
		t.Fatal("clone shares mutable nested request headers")
	}
}
