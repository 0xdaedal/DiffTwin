package comparison

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jrcasanya/difftwin/internal/domain"
)

func TestCompareWithoutSecondaryRunsOnlyPrimaryAndCandidate(t *testing.T) {
	var requests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	result := Compare(http.DefaultClient, domain.Project{
		Primary: target.URL, Candidate: target.URL,
	}, domain.Request{Name: "Health", Method: http.MethodGet, Path: "/"}, nil)

	if result.CandidateDiffers {
		t.Fatal("expected matching primary and candidate responses")
	}
	if result.SecondaryEnabled {
		t.Fatal("secondary should be disabled when no destination is configured")
	}
	if result.NoiseDetected {
		t.Fatal("noise must not be inferred without a secondary destination")
	}
	if requests.Load() != 2 {
		t.Fatalf("expected exactly two requests, got %d", requests.Load())
	}
}

func TestCompareUsesIndependentAuthenticationForEachDestination(t *testing.T) {
	tests := []struct {
		name  string
		auth  domain.DestinationAuth
		creds domain.AuthCredentials
		check func(*http.Request) bool
	}{
		{
			name:  "primary",
			auth:  domain.DestinationAuth{Type: "bearer"},
			creds: domain.AuthCredentials{Token: "primary-secret"},
			check: func(request *http.Request) bool {
				return request.Header.Get("Authorization") == "Bearer primary-secret"
			},
		},
		{
			name:  "candidate",
			auth:  domain.DestinationAuth{Type: "basic"},
			creds: domain.AuthCredentials{Username: "candidate-user", Password: "candidate-password"},
			check: func(request *http.Request) bool {
				username, password, ok := request.BasicAuth()
				return ok && username == "candidate-user" && password == "candidate-password"
			},
		},
		{
			name:  "secondary",
			auth:  domain.DestinationAuth{Type: "api_key", APIKeyName: "api_key", APIKeyLocation: "query"},
			creds: domain.AuthCredentials{APIKey: "secondary-key"},
			check: func(request *http.Request) bool {
				return request.URL.Query().Get("api_key") == "secondary-key"
			},
		},
	}
	servers := make(map[string]*httptest.Server, len(tests))
	var validRequests atomic.Int32
	for _, test := range tests {
		test := test
		servers[test.name] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if !test.check(request) {
				http.Error(w, "unexpected authentication", http.StatusUnauthorized)
				return
			}
			validRequests.Add(1)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer servers[test.name].Close()
	}

	project := domain.Project{
		Primary: servers["primary"].URL, Candidate: servers["candidate"].URL, Secondary: servers["secondary"].URL,
		Auth: map[string]domain.DestinationAuth{
			"primary": tests[0].auth, "candidate": tests[1].auth, "secondary": tests[2].auth,
		},
	}
	credentials := map[string]domain.AuthCredentials{
		"primary": tests[0].creds, "candidate": tests[1].creds, "secondary": tests[2].creds,
	}
	result := Compare(http.DefaultClient, project, domain.Request{Name: "Check", Method: "GET", Path: "/"}, credentials)
	if result.Primary.StatusCode != http.StatusOK || result.Candidate.StatusCode != http.StatusOK ||
		result.Secondary.StatusCode != http.StatusOK || validRequests.Load() != 3 {
		t.Fatalf("destination authentication was not applied: result=%#v valid=%d", result, validRequests.Load())
	}
}

func TestAPIKeyCanBeAppliedAsHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-API-Key") != "header-secret" {
			http.Error(w, "missing API key", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	response := execute(http.DefaultClient, server.URL, domain.Request{Name: "Check", Method: "GET", Path: "/"}, nil,
		domain.DestinationAuth{Type: "api_key", APIKeyName: "X-API-Key", APIKeyLocation: "header"},
		domain.AuthCredentials{APIKey: "header-secret"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("API key header was not applied: %#v", response)
	}
}

func TestTransportErrorsRedactAPIKeyQueryValues(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("request failed at %s", request.URL.String())
	})}
	response := execute(client, "https://api.example", domain.Request{Name: "Check", Method: "GET", Path: "/"}, nil,
		domain.DestinationAuth{Type: "api_key", APIKeyName: "key", APIKeyLocation: "query"},
		domain.AuthCredentials{APIKey: "do-not-leak&other"})
	if strings.Contains(response.Error, "do-not-leak") || !strings.Contains(response.Error, "[REDACTED]") {
		t.Fatalf("transport error should redact credential: %q", response.Error)
	}
}

func TestBodiesDifferIgnoringJSONPaths(t *testing.T) {
	tests := []struct {
		name    string
		left    string
		right   string
		ignored []string
		differs bool
	}{
		{
			name:    "ignore exact nested property",
			left:    `{"data":{"id":"a","value":1}}`,
			right:   `{"data":{"id":"b","value":1}}`,
			ignored: []string{"$.data.id"},
		},
		{
			name:    "ignore property in every array item",
			left:    `{"items":[{"id":"a","value":1},{"id":"b","value":2}]}`,
			right:   `{"items":[{"id":"x","value":1},{"id":"y","value":2}]}`,
			ignored: []string{"$.items[*].id"},
		},
		{
			name:    "keep non-ignored differences",
			left:    `{"data":{"id":"a","value":1}}`,
			right:   `{"data":{"id":"b","value":2}}`,
			ignored: []string{"$.data.id"},
			differs: true,
		},
		{
			name:    "invalid json is compared as text",
			left:    `not json a`,
			right:   `not json b`,
			ignored: []string{"$"},
			differs: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if differs := BodiesDifferIgnoring(test.left, test.right, test.ignored); differs != test.differs {
				t.Fatalf("BodiesDifferIgnoring() = %v, want %v", differs, test.differs)
			}
		})
	}
}

func TestResponsesDifferIncludesMeaningfulHeadersAndIgnoresDate(t *testing.T) {
	base := Response{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json", "Content-Length": "13", "Date": "Mon, 1 Jan 2024 00:00:00 GMT",
			"CF-Ray": "abc123-MAD",
		},
		Body: `{"ok":true}`,
	}
	same := base
	same.Headers = map[string]string{
		"content-type": " application/json ", "content-length": "999", "Date": "Tue, 2 Jan 2024 00:00:00 GMT",
		"cf-ray": "def456-MAD",
	}
	same.DurationMS = 1500
	if ResponsesDiffer(base, same) {
		t.Fatal("header casing, whitespace, volatile infrastructure headers, Date, and latency should not create a difference")
	}
	same.Headers["NEL"] = `{"report_to":"cf-nel","max_age":86400}`
	same.Headers["Report-To"] = `{"group":"cf-nel","endpoints":[{"url":"https://b.example/report"}]}`
	if !ResponsesDiffer(base, same) {
		t.Fatal("NEL and Report-To changes should be reported unless explicitly configured otherwise")
	}
	changed := same
	changed.Headers = map[string]string{"content-type": "text/plain"}
	if !ResponsesDiffer(base, changed) {
		t.Fatal("meaningful response header changes should be reported")
	}
}

func TestCompareUsesIgnoredJSONPathsForCandidateAndStableNoise(t *testing.T) {
	makeServer := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
	}
	primary := makeServer(`{"id":"stable-a","value":1}`)
	defer primary.Close()
	candidate := makeServer(`{"id":"candidate","value":1}`)
	defer candidate.Close()
	secondary := makeServer(`{"id":"stable-b","value":1}`)
	defer secondary.Close()

	result := Compare(http.DefaultClient, domain.Project{
		Primary: primary.URL, Candidate: candidate.URL, Secondary: secondary.URL,
		IgnoredJSONPaths: []string{"$.id"},
	}, domain.Request{Name: "Check", Method: http.MethodGet, Path: "/"}, nil)
	if result.CandidateDiffers || result.NoiseDetected {
		t.Fatalf("ignored paths should affect both candidate and stable comparisons: %#v", result)
	}
}

func TestCompareInterpolatesDestinationOverridesAcrossRequestParts(t *testing.T) {
	tests := []struct {
		name   string
		region string
	}{
		{name: "primary", region: "production"},
		{name: "candidate", region: "staging"},
		{name: "secondary", region: "secondary"},
	}
	servers := make(map[string]*httptest.Server, len(tests))
	for _, test := range tests {
		test := test
		servers[test.name] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				http.Error(w, "failed reading request", http.StatusInternalServerError)
				return
			}
			if request.URL.Path != "/"+test.region ||
				request.URL.Query().Get("common") != "global-value" ||
				request.Header.Get("X-Region") != test.region ||
				string(body) != `{"region":"`+test.region+`"}` {
				http.Error(w, "destination variables were not interpolated", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer servers[test.name].Close()
	}

	result := Compare(http.DefaultClient, domain.Project{
		Primary: servers["primary"].URL, Candidate: servers["candidate"].URL, Secondary: servers["secondary"].URL,
		Variables: map[string]string{"region": "global", "shared": "global-value"},
		DestinationVariables: map[string]map[string]string{
			"primary":   {"region": "production"},
			"candidate": {"region": "staging"},
			"secondary": {"region": "secondary"},
		},
	}, domain.Request{
		Name: "Check", Method: "POST", Path: "/{{region}}?common={{shared}}",
		Headers: map[string]string{"X-Region": "{{region}}"},
		Body:    `{"region":"{{region}}"}`,
	}, nil)
	if result.Primary.StatusCode != http.StatusOK || result.Candidate.StatusCode != http.StatusOK ||
		result.Secondary.StatusCode != http.StatusOK {
		t.Fatalf("expected per-destination variable interpolation for all request parts: %#v", result)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}
