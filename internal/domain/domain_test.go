package domain

import "testing"

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		wantErr bool
	}{
		{
			name:    "valid method is case insensitive",
			request: Request{Name: " List users ", Method: "get", Path: " /users "},
		},
		{
			name:    "missing name",
			request: Request{Method: "GET", Path: "/users"},
			wantErr: true,
		},
		{
			name:    "missing path",
			request: Request{Name: "List users", Method: "GET"},
			wantErr: true,
		},
		{
			name:    "unsupported method",
			request: Request{Name: "Trace", Method: "TRACE", Path: "/users"},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRequest(test.request)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateRequest() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateAuthCredentials(t *testing.T) {
	tests := []struct {
		name        string
		auth        DestinationAuth
		credentials AuthCredentials
		wantErr     bool
	}{
		{name: "none", auth: DestinationAuth{Type: "none"}},
		{name: "empty type", auth: DestinationAuth{}},
		{name: "bearer", auth: DestinationAuth{Type: "bearer"}, credentials: AuthCredentials{Token: "token"}},
		{name: "bearer token required", auth: DestinationAuth{Type: "bearer"}, wantErr: true},
		{name: "basic", auth: DestinationAuth{Type: "basic"}, credentials: AuthCredentials{Password: "password"}},
		{name: "basic password required", auth: DestinationAuth{Type: "basic"}, wantErr: true},
		{
			name:        "API key header",
			auth:        DestinationAuth{Type: "api_key", APIKeyName: "X-API-Key", APIKeyLocation: "header"},
			credentials: AuthCredentials{APIKey: "key"},
		},
		{
			name:        "API key query",
			auth:        DestinationAuth{Type: "api_key", APIKeyName: "key", APIKeyLocation: "query"},
			credentials: AuthCredentials{APIKey: "key"},
		},
		{
			name:    "API key value required",
			auth:    DestinationAuth{Type: "api_key", APIKeyName: "key", APIKeyLocation: "query"},
			wantErr: true,
		},
		{
			name:        "API key name and location required",
			auth:        DestinationAuth{Type: "api_key"},
			credentials: AuthCredentials{APIKey: "key"},
			wantErr:     true,
		},
		{name: "unsupported type", auth: DestinationAuth{Type: "digest"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateAuthCredentials(test.auth, test.credentials)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateAuthCredentials() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestPrepareProjectRejectsInvalidDestinationAndAuthenticationSettings(t *testing.T) {
	validProject := func() Project {
		return Project{
			Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		}
	}
	tests := []struct {
		name   string
		mutate func(*Project)
	}{
		{
			name: "primary must use HTTP",
			mutate: func(project *Project) {
				project.Primary = "ftp://stable.test"
			},
		},
		{
			name: "secondary must use HTTP when configured",
			mutate: func(project *Project) {
				project.Secondary = "https://"
			},
		},
		{
			name: "unknown destination variable target",
			mutate: func(project *Project) {
				project.DestinationVariables = map[string]map[string]string{"unknown": {"region": "eu"}}
			},
		},
		{
			name: "unknown authentication target",
			mutate: func(project *Project) {
				project.Auth = map[string]DestinationAuth{"unknown": {Type: "none"}}
			},
		},
		{
			name: "unsupported authentication type",
			mutate: func(project *Project) {
				project.Auth = map[string]DestinationAuth{"primary": {Type: "digest"}}
			},
		},
		{
			name: "API key name required",
			mutate: func(project *Project) {
				project.Auth = map[string]DestinationAuth{"primary": {Type: "api_key", APIKeyLocation: "header"}}
			},
		},
		{
			name: "API key location required",
			mutate: func(project *Project) {
				project.Auth = map[string]DestinationAuth{"primary": {Type: "api_key", APIKeyName: "X-Key", APIKeyLocation: "body"}}
			},
		},
		{
			name: "invalid ignored JSON path",
			mutate: func(project *Project) {
				project.IgnoredJSONPaths = []string{"$.invalid."}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := validProject()
			test.mutate(&project)
			if err := PrepareProject(&project); err == nil {
				t.Fatal("PrepareProject() succeeded for invalid project configuration")
			}
		})
	}
}

func TestPrepareProjectInitializesCollectionsAndNormalizesAuthType(t *testing.T) {
	project := Project{
		Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		Auth: map[string]DestinationAuth{
			"candidate": {},
			"primary":   {Type: "api_key", APIKeyName: "X-API-Key", APIKeyLocation: "header"},
		},
		DestinationVariables: map[string]map[string]string{"candidate": nil},
	}
	if err := PrepareProject(&project); err != nil {
		t.Fatal(err)
	}
	if project.Auth["candidate"].Type != "none" {
		t.Fatalf("empty auth type was not normalized: %#v", project.Auth)
	}
	if project.Auth["primary"].APIKeyName != "X-API-Key" {
		t.Fatalf("API key name was not retained: %#v", project.Auth["primary"])
	}
	if project.Collections == nil || project.Variables == nil ||
		project.DestinationVariables["candidate"] == nil {
		t.Fatalf("project maps or collections were not initialized: %#v", project)
	}
}

func TestCloneProjectsCopiesNestedRequestsAndFolders(t *testing.T) {
	original := Project{Collections: []Collection{{
		Requests: []Request{{ID: "root", Headers: map[string]string{"X-Root": "before"}}},
		Folders: []Folder{{
			ID:       "folder",
			Requests: []Request{{ID: "nested", Headers: map[string]string{"X-Nested": "before"}}},
			Folders: []Folder{{
				ID:       "child",
				Requests: []Request{{ID: "deep", Headers: map[string]string{"X-Deep": "before"}}},
			}},
		}},
	}}}
	cloned := CloneProjects([]Project{original})
	cloned[0].Collections[0].Requests[0].Headers["X-Root"] = "after"
	cloned[0].Collections[0].Folders[0].Requests[0].Headers["X-Nested"] = "after"
	cloned[0].Collections[0].Folders[0].Folders[0].Requests[0].Headers["X-Deep"] = "after"
	if original.Collections[0].Requests[0].Headers["X-Root"] != "before" ||
		original.Collections[0].Folders[0].Requests[0].Headers["X-Nested"] != "before" ||
		original.Collections[0].Folders[0].Folders[0].Requests[0].Headers["X-Deep"] != "before" {
		t.Fatal("cloned request headers share mutable maps with source project")
	}
}

func TestInterpolateFindProjectAndNewID(t *testing.T) {
	if got := Interpolate("{{known}}/{{unknown}}/{{known}}", map[string]string{"known": "value"}); got != "value/{{unknown}}/value" {
		t.Fatalf("Interpolate() = %q", got)
	}
	projects := []Project{{ID: "first"}, {ID: "second"}}
	if got := FindProject(projects, "second"); got == nil || got.ID != "second" {
		t.Fatalf("FindProject() = %#v, want second project", got)
	}
	if FindProject(projects, "missing") != nil {
		t.Fatal("FindProject() returned a project for a missing ID")
	}
	firstID, secondID := NewID(), NewID()
	if len(firstID) != 32 || firstID == secondID {
		t.Fatalf("NewID() returned invalid or repeated IDs: %q and %q", firstID, secondID)
	}
}

func TestParseJSONPath(t *testing.T) {
	tests := []struct {
		path    string
		length  int
		wantErr bool
	}{
		{path: "$", length: 0},
		{path: "$.data.id", length: 2},
		{path: "$.items[*].timestamp", length: 3},
		{path: `$["property.with.dots"]`, length: 1},
		{path: "$[0].value", length: 2},
		{path: "data.id", wantErr: true},
		{path: "$.data.", wantErr: true},
		{path: "$[x]", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			segments, err := ParseJSONPath(test.path)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParseJSONPath(%q) error = %v, wantErr %v", test.path, err, test.wantErr)
			}
			if err == nil && len(segments) != test.length {
				t.Fatalf("ParseJSONPath(%q) returned %d segments, want %d", test.path, len(segments), test.length)
			}
		})
	}
}

func TestPrepareProjectValidatesAndNormalizesIgnoredJSONPaths(t *testing.T) {
	project := Project{
		Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		IgnoredJSONPaths: []string{" $.meta.generatedAt ", "", "$.meta.generatedAt"},
	}
	if err := PrepareProject(&project); err != nil {
		t.Fatal(err)
	}
	if len(project.IgnoredJSONPaths) != 1 || project.IgnoredJSONPaths[0] != "$.meta.generatedAt" {
		t.Fatalf("ignored paths were not normalized: %#v", project.IgnoredJSONPaths)
	}
	project.IgnoredJSONPaths = []string{"meta.generatedAt"}
	if err := PrepareProject(&project); err == nil {
		t.Fatal("invalid ignored JSON path should be rejected")
	}
}

func TestMergeVariablesUsesDestinationOverridesAndCommonFallback(t *testing.T) {
	merged := MergeVariables(
		map[string]string{"region": "global", "token": "shared", "locale": "en"},
		map[string]string{"region": "candidate", "token": ""},
	)
	if merged["region"] != "candidate" || merged["token"] != "" || merged["locale"] != "en" {
		t.Fatalf("unexpected merged variables: %#v", merged)
	}
}

func TestPrepareProjectValidatesDestinationVariableTargets(t *testing.T) {
	project := Project{
		Name: "API", Primary: "https://stable.test", Candidate: "https://candidate.test",
		DestinationVariables: map[string]map[string]string{"unknown": {"region": "eu"}},
	}
	if err := PrepareProject(&project); err == nil {
		t.Fatal("unknown destination variable target should be rejected")
	}
}

func TestCloneProjectsCopiesDestinationVariables(t *testing.T) {
	original := Project{
		DestinationVariables: map[string]map[string]string{"primary": {"region": "eu"}},
	}
	cloned := CloneProjects([]Project{original})
	cloned[0].DestinationVariables["primary"]["region"] = "us"
	if original.DestinationVariables["primary"]["region"] != "eu" {
		t.Fatal("cloning projects should deep-copy destination variable maps")
	}
}

func TestMoveCollectionItemMovesRequestsAndFolders(t *testing.T) {
	project := Project{Collections: []Collection{
		{
			ID: "source",
			Folders: []Folder{{
				ID: "source-folder", Name: "Source",
				Requests: []Request{{ID: "request", Name: "List", Method: "GET", Path: "/users", Headers: map[string]string{"X-Test": "yes"}}},
				Folders:  []Folder{{ID: "nested", Name: "Nested"}},
			}},
		},
		{
			ID: "destination",
			Folders: []Folder{{
				ID: "destination-folder", Name: "Destination",
			}},
		},
	}}

	if err := MoveCollectionItem(&project, "source", "source-folder", "request", "request", "destination", "destination-folder"); err != nil {
		t.Fatal(err)
	}
	if err := MoveCollectionItem(&project, "source", "", "folder", "source-folder", "destination", ""); err != nil {
		t.Fatal(err)
	}

	source := collectionByID(&project, "source")
	destination := collectionByID(&project, "destination")
	if len(source.Folders) != 0 {
		t.Fatalf("source folder was not removed: %#v", source.Folders)
	}
	if len(destination.Folders[0].Requests) != 1 || destination.Folders[0].Requests[0].Headers["X-Test"] != "yes" {
		t.Fatalf("moved request data was not preserved: %#v", destination.Folders[0].Requests)
	}
	movedFolder := destination.Folders[1]
	if movedFolder.ID != "source-folder" || len(movedFolder.Folders) != 1 || movedFolder.Folders[0].ID != "nested" {
		t.Fatalf("moved folder contents were not preserved: %#v", movedFolder)
	}
}

func TestMoveCollectionItemRejectsFolderDescendantsWithoutMutation(t *testing.T) {
	project := Project{Collections: []Collection{{
		ID: "collection",
		Folders: []Folder{{
			ID:      "parent",
			Folders: []Folder{{ID: "child"}},
		}},
	}}}

	if err := MoveCollectionItem(&project, "collection", "", "folder", "parent", "collection", "child"); err == nil {
		t.Fatal("moving a folder into its descendant should fail")
	}
	if len(project.Collections[0].Folders) != 1 ||
		project.Collections[0].Folders[0].Folders[0].ID != "child" {
		t.Fatalf("invalid move changed the folder tree: %#v", project.Collections[0].Folders)
	}
}
