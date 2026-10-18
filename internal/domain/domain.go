package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
)

type Project struct {
	ID                   string                       `json:"id"`
	Name                 string                       `json:"name"`
	Primary              string                       `json:"primary"`
	Candidate            string                       `json:"candidate"`
	Secondary            string                       `json:"secondary"`
	Auth                 map[string]DestinationAuth   `json:"auth,omitempty"`
	IgnoredJSONPaths     []string                     `json:"ignored_json_paths,omitempty"`
	LatencyWarnPercent   int                          `json:"latency_warn_percent,omitempty"`
	LatencyAlertPercent  int                          `json:"latency_alert_percent,omitempty"`
	LatencyFloorMS       int                          `json:"latency_floor_ms,omitempty"`
	Variables            map[string]string            `json:"variables,omitempty"`
	DestinationVariables map[string]map[string]string `json:"destination_variables,omitempty"`
	Collections          []Collection                 `json:"collections"`
}

// Latency thresholds are relative to the primary stable destination. A zero
// value means the project never configured them, so the defaults apply.
const (
	DefaultLatencyWarnPercent  = 20
	DefaultLatencyAlertPercent = 50
	DefaultLatencyFloorMS      = 50
	maxLatencyPercent          = 10000
	maxLatencyFloorMS          = 600000
)

type DestinationAuth struct {
	Type           string `json:"type"`
	APIKeyName     string `json:"api_key_name,omitempty"`
	APIKeyLocation string `json:"api_key_location,omitempty"`
}

type AuthCredentials struct {
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
}

func ValidateAuthCredentials(auth DestinationAuth, credentials AuthCredentials) error {
	switch auth.Type {
	case "", "none":
		return nil
	case "bearer":
		if credentials.Token == "" {
			return errors.New("bearer token is required")
		}
	case "basic":
		if credentials.Password == "" {
			return errors.New("basic authentication password is required")
		}
	case "api_key":
		if credentials.APIKey == "" {
			return errors.New("API key value is required")
		}
		if auth.APIKeyName == "" || (auth.APIKeyLocation != "header" && auth.APIKeyLocation != "query") {
			return errors.New("API key name and location are required")
		}
	default:
		return errors.New("unsupported destination authentication type")
	}
	return nil
}

type Collection struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Requests []Request `json:"requests"`
	Folders  []Folder  `json:"folders,omitempty"`
}

type Folder struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Requests []Request `json:"requests,omitempty"`
	Folders  []Folder  `json:"folders,omitempty"`
}

type Request struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

func ValidateRequest(request Request) error {
	if strings.TrimSpace(request.Name) == "" {
		return errors.New("request name is required")
	}
	if strings.TrimSpace(request.Path) == "" {
		return errors.New("request path is required")
	}
	switch strings.ToUpper(request.Method) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return nil
	default:
		return errors.New("unsupported HTTP method")
	}
}

func PrepareProject(project *Project) error {
	project.Name = strings.TrimSpace(project.Name)
	if project.Name == "" {
		return errors.New("project name is required")
	}
	project.Primary = strings.TrimSpace(project.Primary)
	project.Candidate = strings.TrimSpace(project.Candidate)
	project.Secondary = strings.TrimSpace(project.Secondary)
	for _, destination := range []string{project.Primary, project.Candidate} {
		u, err := url.Parse(strings.TrimSpace(destination))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("primary and candidate destinations must be valid http or https URLs")
		}
	}
	if project.Secondary != "" {
		u, err := url.Parse(project.Secondary)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("secondary destination must be a valid http or https URL when provided")
		}
	}
	if project.Variables == nil {
		project.Variables = map[string]string{}
	}
	if project.DestinationVariables == nil {
		project.DestinationVariables = map[string]map[string]string{}
	}
	for target, variables := range project.DestinationVariables {
		if target != "primary" && target != "candidate" && target != "secondary" {
			return errors.New("el destino de las variables debe ser primary, candidate o secondary")
		}
		if variables == nil {
			project.DestinationVariables[target] = map[string]string{}
		}
	}
	if project.Auth == nil {
		project.Auth = map[string]DestinationAuth{}
	}
	normalizedIgnoredPaths := make([]string, 0, len(project.IgnoredJSONPaths))
	for _, path := range project.IgnoredJSONPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, err := ParseJSONPath(path); err != nil {
			return err
		}
		if !containsString(normalizedIgnoredPaths, path) {
			normalizedIgnoredPaths = append(normalizedIgnoredPaths, path)
		}
	}
	project.IgnoredJSONPaths = normalizedIgnoredPaths
	if err := normalizeLatencyThresholds(project); err != nil {
		return err
	}
	for target, auth := range project.Auth {
		if target != "primary" && target != "candidate" && target != "secondary" {
			return errors.New("authentication target must be primary, candidate, or secondary")
		}
		switch auth.Type {
		case "", "none", "bearer", "basic", "api_key":
			if auth.Type == "" {
				auth.Type = "none"
			}
		default:
			return errors.New("unsupported destination authentication type")
		}
		if auth.Type == "api_key" {
			auth.APIKeyName = strings.TrimSpace(auth.APIKeyName)
			if auth.APIKeyName == "" {
				return errors.New("API key name is required")
			}
			if auth.APIKeyLocation != "header" && auth.APIKeyLocation != "query" {
				return errors.New("API key location must be header or query")
			}
			if auth.APIKeyLocation == "header" && textproto.CanonicalMIMEHeaderKey(auth.APIKeyName) == "" {
				return errors.New("API key header name is invalid")
			}
		}
		project.Auth[target] = auth
	}
	if project.Collections == nil {
		project.Collections = []Collection{}
	}
	return nil
}

type JSONPathSegment struct {
	Key      string
	Index    int
	IsIndex  bool
	Wildcard bool
}

func ParseJSONPath(path string) ([]JSONPathSegment, error) {
	if path == "$" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "$") {
		return nil, errors.New("la ruta JSON debe empezar por $")
	}
	segments := make([]JSONPathSegment, 0, 4)
	for i := 1; i < len(path); {
		switch path[i] {
		case '.':
			i++
			if i >= len(path) {
				return nil, errors.New("la ruta JSON no puede terminar en un punto")
			}
			if path[i] == '*' {
				segments = append(segments, JSONPathSegment{Wildcard: true})
				i++
				continue
			}
			start := i
			for i < len(path) && (path[i] == '_' || path[i] == '$' ||
				(path[i] >= 'a' && path[i] <= 'z') || (path[i] >= 'A' && path[i] <= 'Z') ||
				(path[i] >= '0' && path[i] <= '9')) {
				i++
			}
			if start == i {
				return nil, errors.New("la ruta JSON contiene un nombre de propiedad no válido")
			}
			segments = append(segments, JSONPathSegment{Key: path[start:i]})
		case '[':
			i++
			if i >= len(path) {
				return nil, errors.New("la ruta JSON tiene un corchete sin cerrar")
			}
			if path[i] == '*' {
				i++
				if i >= len(path) || path[i] != ']' {
					return nil, errors.New("el comodín de la ruta JSON no es válido")
				}
				i++
				segments = append(segments, JSONPathSegment{Wildcard: true})
				continue
			}
			if path[i] == '"' {
				start := i
				i++
				escaped := false
				for i < len(path) {
					if escaped {
						escaped = false
					} else if path[i] == '\\' {
						escaped = true
					} else if path[i] == '"' {
						i++
						break
					}
					i++
				}
				if i > len(path) || path[i-1] != '"' {
					return nil, errors.New("la propiedad entre comillas de la ruta JSON no es válida")
				}
				key, err := strconv.Unquote(path[start:i])
				if err != nil {
					return nil, errors.New("la propiedad entre comillas de la ruta JSON no es válida")
				}
				if i >= len(path) || path[i] != ']' {
					return nil, errors.New("la ruta JSON tiene un corchete sin cerrar")
				}
				i++
				segments = append(segments, JSONPathSegment{Key: key})
				continue
			}
			start := i
			for i < len(path) && path[i] >= '0' && path[i] <= '9' {
				i++
			}
			if start == i || i >= len(path) || path[i] != ']' {
				return nil, errors.New("el índice de array de la ruta JSON no es válido")
			}
			index, err := strconv.Atoi(path[start:i])
			if err != nil {
				return nil, errors.New("el índice de array de la ruta JSON no es válido")
			}
			i++
			segments = append(segments, JSONPathSegment{Index: index, IsIndex: true})
		default:
			return nil, errors.New("la ruta JSON contiene un segmento no válido")
		}
	}
	return segments, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeLatencyThresholds(project *Project) error {
	if project.LatencyWarnPercent == 0 {
		project.LatencyWarnPercent = DefaultLatencyWarnPercent
	}
	if project.LatencyAlertPercent == 0 {
		project.LatencyAlertPercent = DefaultLatencyAlertPercent
	}
	if project.LatencyFloorMS == 0 {
		project.LatencyFloorMS = DefaultLatencyFloorMS
	}
	if project.LatencyWarnPercent < 1 || project.LatencyWarnPercent > maxLatencyPercent {
		return errors.New("el umbral de aviso de latencia debe estar entre 1 y 10000 por ciento")
	}
	if project.LatencyAlertPercent < 1 || project.LatencyAlertPercent > maxLatencyPercent {
		return errors.New("el umbral de alerta de latencia debe estar entre 1 y 10000 por ciento")
	}
	if project.LatencyAlertPercent < project.LatencyWarnPercent {
		return errors.New("el umbral de alerta de latencia no puede ser menor que el de aviso")
	}
	if project.LatencyFloorMS < 1 || project.LatencyFloorMS > maxLatencyFloorMS {
		return errors.New("el margen mínimo de latencia debe estar entre 1 y 600000 milisegundos")
	}
	return nil
}

func CloneProjects(projects []Project) []Project {
	cloned := make([]Project, len(projects))
	for i, project := range projects {
		cloned[i] = project
		cloned[i].Auth = make(map[string]DestinationAuth, len(project.Auth))
		for target, auth := range project.Auth {
			cloned[i].Auth[target] = auth
		}
		cloned[i].IgnoredJSONPaths = append([]string(nil), project.IgnoredJSONPaths...)
		cloned[i].Variables = make(map[string]string, len(project.Variables))
		for key, value := range project.Variables {
			cloned[i].Variables[key] = value
		}
		cloned[i].DestinationVariables = make(map[string]map[string]string, len(project.DestinationVariables))
		for target, variables := range project.DestinationVariables {
			clonedVariables := make(map[string]string, len(variables))
			for key, value := range variables {
				clonedVariables[key] = value
			}
			cloned[i].DestinationVariables[target] = clonedVariables
		}
		cloned[i].Collections = make([]Collection, len(project.Collections))
		for j, collection := range project.Collections {
			cloned[i].Collections[j] = collection
			cloned[i].Collections[j].Requests = cloneRequests(collection.Requests)
			cloned[i].Collections[j].Folders = cloneFolders(collection.Folders)
		}
	}
	return cloned
}

func cloneRequests(requests []Request) []Request {
	cloned := make([]Request, len(requests))
	for i, request := range requests {
		cloned[i] = request
		cloned[i].Headers = make(map[string]string, len(request.Headers))
		for key, value := range request.Headers {
			cloned[i].Headers[key] = value
		}
	}
	return cloned
}

func cloneFolders(folders []Folder) []Folder {
	cloned := make([]Folder, len(folders))
	for i, folder := range folders {
		cloned[i] = folder
		cloned[i].Requests = cloneRequests(folder.Requests)
		cloned[i].Folders = cloneFolders(folder.Folders)
	}
	return cloned
}

func Interpolate(value string, variables map[string]string) string {
	for key, replacement := range variables {
		value = strings.ReplaceAll(value, "{{"+key+"}}", replacement)
	}
	return value
}

func MergeVariables(common, destination map[string]string) map[string]string {
	merged := make(map[string]string, len(common)+len(destination))
	for key, value := range common {
		merged[key] = value
	}
	for key, value := range destination {
		merged[key] = value
	}
	return merged
}

func FindProject(projects []Project, id string) *Project {
	for i := range projects {
		if projects[i].ID == id {
			return &projects[i]
		}
	}
	return nil
}

func NewID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}
