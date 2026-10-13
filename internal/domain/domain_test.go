package domain

import "testing"

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
