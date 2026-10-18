package comparison

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jrcasanya/difftwin/internal/domain"
)

const MaxBodySize = 2 << 20

type Response struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	DurationMS int64             `json:"duration_ms"`
	Truncated  bool              `json:"truncated"`
	Error      string            `json:"error,omitempty"`
}

type Result struct {
	Primary          Response `json:"primary"`
	Candidate        Response `json:"candidate"`
	Secondary        Response `json:"secondary"`
	SecondaryEnabled bool     `json:"secondary_enabled"`
	CandidateDiffers bool     `json:"candidate_differs"`
	NoiseDetected    bool     `json:"noise_detected"`
	LatencyStatus    string   `json:"latency_status"`
}

const (
	LatencyOK    = "ok"
	LatencyWarn  = "warn"
	LatencyAlert = "alert"
)

// stableNoiseMultiplier keeps the candidate unflagged while its extra time stays
// close to the spread already observed between two identical stable destinations.
const stableNoiseMultiplier = 1.5

// EvaluateLatencyStatus reports how far the candidate deviates from the primary
// stable destination. A single request is a weak sample, so a deviation is only
// reported when it clears three guards: an absolute floor that absorbs jitter,
// the configured percentage thresholds, and the noise observed between stables.
// Latency never changes the comparison verdict; it only annotates it.
func EvaluateLatencyStatus(result Result, project domain.Project) string {
	if result.Primary.Error != "" || result.Candidate.Error != "" {
		return ""
	}
	delta := result.Candidate.DurationMS - result.Primary.DurationMS
	if delta <= 0 {
		return LatencyOK
	}
	warnPercent := project.LatencyWarnPercent
	if warnPercent <= 0 {
		warnPercent = domain.DefaultLatencyWarnPercent
	}
	alertPercent := project.LatencyAlertPercent
	if alertPercent <= 0 {
		alertPercent = domain.DefaultLatencyAlertPercent
	}
	floorMS := project.LatencyFloorMS
	if floorMS <= 0 {
		floorMS = domain.DefaultLatencyFloorMS
	}
	if delta < int64(floorMS) {
		return LatencyOK
	}
	if result.SecondaryEnabled && result.Secondary.Error == "" {
		spread := result.Secondary.DurationMS - result.Primary.DurationMS
		if spread < 0 {
			spread = -spread
		}
		if float64(delta) <= stableNoiseMultiplier*float64(spread) {
			return LatencyOK
		}
	}
	if result.Primary.DurationMS <= 0 {
		return LatencyAlert
	}
	percent := float64(delta) * 100 / float64(result.Primary.DurationMS)
	switch {
	case percent >= float64(alertPercent):
		return LatencyAlert
	case percent >= float64(warnPercent):
		return LatencyWarn
	}
	return LatencyOK
}

func Compare(client *http.Client, project domain.Project, request domain.Request, credentials map[string]domain.AuthCredentials) Result {
	type destination struct {
		name string
		url  string
	}
	targets := []destination{
		{name: "primary", url: project.Primary},
		{name: "candidate", url: project.Candidate},
	}
	if project.Secondary != "" {
		targets = append(targets, destination{name: "secondary", url: project.Secondary})
	}
	results := make([]Response, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(index int, targetName, base string) {
			defer wg.Done()
			variables := domain.MergeVariables(project.Variables, project.DestinationVariables[targetName])
			results[index] = execute(client, base, request, variables, project.Auth[targetName], credentials[targetName])
		}(i, target.name, target.url)
	}
	wg.Wait()
	result := Result{Primary: results[0], Candidate: results[1], SecondaryEnabled: len(results) == 3}
	if len(results) == 3 {
		result.Secondary = results[2]
		result.NoiseDetected = ResponsesDifferIgnoring(result.Primary, result.Secondary, project.IgnoredJSONPaths)
	}
	result.Candidate = results[1]
	result.CandidateDiffers = ResponsesDifferIgnoring(result.Primary, result.Candidate, project.IgnoredJSONPaths)
	result.LatencyStatus = EvaluateLatencyStatus(result, project)
	return result
}

func execute(client *http.Client, base string, request domain.Request, variables map[string]string, auth domain.DestinationAuth, credentials domain.AuthCredentials) Response {
	method := strings.ToUpper(request.Method)
	path := domain.Interpolate(request.Path, variables)
	targetURL, err := BuildTargetURL(base, path)
	if err != nil {
		return Response{Error: err.Error()}
	}
	body := domain.Interpolate(request.Body, variables)
	req, err := http.NewRequest(method, targetURL, strings.NewReader(body))
	if err != nil {
		return Response{Error: err.Error()}
	}
	for key, value := range request.Headers {
		req.Header.Set(domain.Interpolate(key, variables), domain.Interpolate(value, variables))
	}
	switch auth.Type {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+credentials.Token)
	case "basic":
		req.SetBasicAuth(credentials.Username, credentials.Password)
	case "api_key":
		if auth.APIKeyLocation == "header" {
			req.Header.Set(auth.APIKeyName, credentials.APIKey)
		} else {
			query := req.URL.Query()
			query.Set(auth.APIKeyName, credentials.APIKey)
			req.URL.RawQuery = query.Encode()
		}
	}

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return Response{Error: redactCredentials(err.Error(), credentials), DurationMS: time.Since(started).Milliseconds()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodySize+1))
	if err != nil {
		return Response{StatusCode: resp.StatusCode, Error: err.Error(), DurationMS: time.Since(started).Milliseconds()}
	}
	truncated := len(data) > MaxBodySize
	if truncated {
		data = data[:MaxBodySize]
	}
	headers := make(map[string]string, len(resp.Header))
	for key, values := range resp.Header {
		headers[key] = strings.Join(values, ", ")
	}
	return Response{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       string(data),
		DurationMS: time.Since(started).Milliseconds(),
		Truncated:  truncated,
	}
}

func redactCredentials(message string, credentials domain.AuthCredentials) string {
	for _, secret := range []string{credentials.Token, credentials.Username, credentials.Password, credentials.APIKey} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
			message = strings.ReplaceAll(message, url.QueryEscape(secret), "[REDACTED]")
		}
	}
	return message
}

func BuildTargetURL(base, requestPath string) (string, error) {
	if strings.TrimSpace(requestPath) == "" {
		requestPath = "/"
	}
	parsedPath, err := url.Parse(requestPath)
	if err != nil || parsedPath.IsAbs() || parsedPath.Host != "" {
		return "", errors.New("request path must be relative to the configured destination")
	}
	baseURL, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return "", errors.New("destination must be a valid http or https URL")
	}
	return baseURL.ResolveReference(parsedPath).String(), nil
}

func ResponsesDiffer(a, b Response) bool {
	return ResponsesDifferIgnoring(a, b, nil)
}

func ResponsesDifferIgnoring(a, b Response, ignoredJSONPaths []string) bool {
	if a.Error != "" || b.Error != "" {
		return a.Error != b.Error || a.StatusCode != b.StatusCode ||
			!maps.Equal(comparableHeaders(a.Headers), comparableHeaders(b.Headers)) ||
			BodiesDifferIgnoring(a.Body, b.Body, ignoredJSONPaths)
	}
	return a.StatusCode != b.StatusCode ||
		!maps.Equal(comparableHeaders(a.Headers), comparableHeaders(b.Headers)) ||
		BodiesDifferIgnoring(a.Body, b.Body, ignoredJSONPaths)
}

func BodiesDiffer(a, b string) bool {
	return BodiesDifferIgnoring(a, b, nil)
}

func BodiesDifferIgnoring(a, b string, ignoredJSONPaths []string) bool {
	canonical := func(value string) []byte {
		var decoded any
		if json.Unmarshal([]byte(value), &decoded) == nil {
			parsedPaths := make([][]domain.JSONPathSegment, 0, len(ignoredJSONPaths))
			for _, path := range ignoredJSONPaths {
				segments, err := domain.ParseJSONPath(path)
				if err == nil {
					parsedPaths = append(parsedPaths, segments)
				}
			}
			decoded = pruneIgnoredJSON(decoded, nil, parsedPaths)
			if encoded, err := json.Marshal(decoded); err == nil {
				return encoded
			}
		}
		return []byte(value)
	}
	return !bytes.Equal(canonical(a), canonical(b))
}

var ignoredResponseHeaders = map[string]struct{}{
	"connection": {}, "content-length": {}, "date": {}, "keep-alive": {}, "proxy-authenticate": {},
	"proxy-authorization": {}, "te": {}, "trailer": {}, "transfer-encoding": {}, "upgrade": {},
	"cf-ray": {},
}

func comparableHeaders(headers map[string]string) map[string]string {
	comparable := make(map[string]string, len(headers))
	for name, value := range headers {
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		if _, ignored := ignoredResponseHeaders[normalizedName]; ignored {
			continue
		}
		comparable[normalizedName] = strings.Join(strings.Fields(value), " ")
	}
	return comparable
}

func pruneIgnoredJSON(value any, path []domain.JSONPathSegment, patterns [][]domain.JSONPathSegment) any {
	if pathMatchesIgnored(path, patterns) {
		return nil
	}
	switch node := value.(type) {
	case map[string]any:
		pruned := make(map[string]any, len(node))
		for key, child := range node {
			childPath := appendPath(path, domain.JSONPathSegment{Key: key})
			if pathMatchesIgnored(childPath, patterns) {
				continue
			}
			pruned[key] = pruneIgnoredJSON(child, childPath, patterns)
		}
		return pruned
	case []any:
		pruned := make([]any, len(node))
		for index, child := range node {
			childPath := appendPath(path, domain.JSONPathSegment{Index: index, IsIndex: true})
			if pathMatchesIgnored(childPath, patterns) {
				continue
			}
			pruned[index] = pruneIgnoredJSON(child, childPath, patterns)
		}
		return pruned
	default:
		return value
	}
}

func appendPath(path []domain.JSONPathSegment, segment domain.JSONPathSegment) []domain.JSONPathSegment {
	next := make([]domain.JSONPathSegment, len(path)+1)
	copy(next, path)
	next[len(path)] = segment
	return next
}

func pathMatchesIgnored(path []domain.JSONPathSegment, patterns [][]domain.JSONPathSegment) bool {
	for _, pattern := range patterns {
		if len(path) != len(pattern) {
			continue
		}
		matches := true
		for index, segment := range pattern {
			if segment.Wildcard {
				continue
			}
			actual := path[index]
			if segment.IsIndex != actual.IsIndex || (segment.IsIndex && segment.Index != actual.Index) ||
				(!segment.IsIndex && segment.Key != actual.Key) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}
