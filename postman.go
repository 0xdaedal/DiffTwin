package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/jrcasanya/difftwin/internal/domain"
)

type postmanCollection struct {
	Info struct {
		Name   string `json:"name"`
		Schema string `json:"schema"`
	} `json:"info"`
	Items     []postmanItem     `json:"item"`
	Variables []postmanVariable `json:"variable"`
	Events    []json.RawMessage `json:"event"`
	Auth      json.RawMessage   `json:"auth"`
}

type postmanItem struct {
	Name    string            `json:"name"`
	Items   []postmanItem     `json:"item"`
	Request json.RawMessage   `json:"request"`
	Events  []json.RawMessage `json:"event"`
}

type postmanVariable struct {
	Key      string          `json:"key"`
	Value    json.RawMessage `json:"value"`
	Disabled bool            `json:"disabled"`
}

type postmanRequest struct {
	Method  string             `json:"method"`
	Headers []postmanHeader    `json:"header"`
	URL     json.RawMessage    `json:"url"`
	Body    postmanRequestBody `json:"body"`
	Auth    json.RawMessage    `json:"auth"`
	Events  []json.RawMessage  `json:"event"`
}

type postmanHeader struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
}

type postmanRequestBody struct {
	Mode    string `json:"mode"`
	Raw     string `json:"raw"`
	Options struct {
		Raw struct {
			Language string `json:"language"`
		} `json:"raw"`
	} `json:"options"`
}

type postmanURL struct {
	Raw      string            `json:"raw"`
	Protocol string            `json:"protocol"`
	Host     json.RawMessage   `json:"host"`
	Path     []json.RawMessage `json:"path"`
	Query    []struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Disabled bool   `json:"disabled"`
	} `json:"query"`
}

func decodePostmanCollection(raw []byte) (Collection, map[string]string, []string, error) {
	var source postmanCollection
	if err := json.Unmarshal(raw, &source); err != nil {
		return Collection{}, nil, nil, fmt.Errorf("el archivo no es una colección JSON válida de Postman: %w", err)
	}
	if strings.TrimSpace(source.Info.Name) == "" || len(source.Items) == 0 {
		return Collection{}, nil, nil, fmt.Errorf("la colección de Postman debe tener nombre y al menos un elemento")
	}
	if source.Info.Schema != "" && !strings.Contains(source.Info.Schema, "schema.getpostman.com/json/collection/v2") {
		return Collection{}, nil, nil, fmt.Errorf("formato de colección no compatible: se requiere Postman Collection v2.0 o v2.1")
	}

	collection := Collection{ID: domain.NewID(), Name: strings.TrimSpace(source.Info.Name)}
	var warnings []string
	if len(source.Events) > 0 {
		warnings = append(warnings, "scripts pre-request/tests omitidos")
	}
	if unsupportedPostmanAuth(source.Auth) {
		warnings = append(warnings, "configuración de autenticación de Postman no importada")
	}
	for _, item := range source.Items {
		if err := convertPostmanItem(item, &collection.Requests, &collection.Folders, &warnings); err != nil {
			return Collection{}, nil, nil, err
		}
	}
	variables := make(map[string]string)
	for _, variable := range source.Variables {
		if variable.Disabled || strings.TrimSpace(variable.Key) == "" {
			continue
		}
		value, err := postmanValue(variable.Value)
		if err != nil {
			return Collection{}, nil, nil, fmt.Errorf("variable Postman %q: %w", variable.Key, err)
		}
		variables[variable.Key] = value
	}
	return collection, variables, uniqueStrings(warnings), nil
}

func convertPostmanItem(item postmanItem, requests *[]Request, folders *[]Folder, warnings *[]string) error {
	if len(item.Items) > 0 {
		folder := Folder{ID: domain.NewID(), Name: strings.TrimSpace(item.Name)}
		if folder.Name == "" {
			folder.Name = "Carpeta"
		}
		for _, child := range item.Items {
			if err := convertPostmanItem(child, &folder.Requests, &folder.Folders, warnings); err != nil {
				return err
			}
		}
		*folders = append(*folders, folder)
		return nil
	}
	if len(item.Request) == 0 || string(item.Request) == "null" {
		return fmt.Errorf("el elemento Postman %q no contiene petición ni carpeta", item.Name)
	}
	var source postmanRequest
	if err := json.Unmarshal(item.Request, &source); err != nil {
		return fmt.Errorf("leer petición Postman %q: %w", item.Name, err)
	}
	method := strings.ToUpper(strings.TrimSpace(source.Method))
	if method == "" {
		method = "GET"
	}
	requestPath, err := postmanRequestPath(source.URL)
	if err != nil {
		return fmt.Errorf("URL de petición Postman %q: %w", item.Name, err)
	}
	headers := make(map[string]string)
	headerNames := make(map[string]struct{})
	for _, header := range source.Headers {
		if header.Disabled || strings.TrimSpace(header.Key) == "" {
			continue
		}
		lowerName := strings.ToLower(header.Key)
		if _, duplicate := headerNames[lowerName]; duplicate {
			*warnings = append(*warnings, "headers Postman duplicados combinados")
		}
		headerNames[lowerName] = struct{}{}
		headers[header.Key] = header.Value
	}
	if len(source.Events) > 0 || len(item.Events) > 0 {
		*warnings = append(*warnings, "scripts pre-request/tests omitidos")
	}
	if unsupportedPostmanAuth(source.Auth) {
		*warnings = append(*warnings, "configuración de autenticación de Postman no importada")
	}
	if source.Body.Mode != "" && source.Body.Mode != "raw" {
		*warnings = append(*warnings, "cuerpos Postman que no son raw omitidos")
	}
	body := ""
	if source.Body.Mode == "raw" {
		body = source.Body.Raw
	}
	if source.Body.Mode == "raw" && source.Body.Options.Raw.Language == "json" && !hasHeader(headers, "Content-Type") {
		headers["Content-Type"] = "application/json"
	}

	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = method + " " + requestPath
	}
	if err := domain.ValidateRequest(Request{Name: name, Method: method, Path: requestPath, Headers: headers}); err != nil {
		return fmt.Errorf("petición Postman %q: %w", item.Name, err)
	}
	*requests = append(*requests, Request{
		ID: domain.NewID(), Name: name, Method: method, Path: requestPath, Headers: headers, Body: body,
	})
	return nil
}

func unsupportedPostmanAuth(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var auth struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &auth); err != nil {
		return true
	}
	return auth.Type != "" && auth.Type != "noauth"
}

func hasHeader(headers map[string]string, name string) bool {
	for header := range headers {
		if strings.EqualFold(header, name) {
			return true
		}
	}
	return false
}

func postmanRequestPath(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("falta la URL")
	}
	var rawURL string
	if json.Unmarshal(raw, &rawURL) == nil {
		return relativePostmanURL(rawURL)
	}
	var structured postmanURL
	if err := json.Unmarshal(raw, &structured); err != nil {
		return "", fmt.Errorf("formato de URL no válido: %w", err)
	}
	if structured.Raw != "" {
		return relativePostmanURL(structured.Raw)
	}
	host := ""
	if err := json.Unmarshal(structured.Host, &host); err != nil {
		var parts []string
		if err := json.Unmarshal(structured.Host, &parts); err != nil {
			return "", fmt.Errorf("host no válido")
		}
		host = strings.Join(parts, ".")
	}
	if host == "" {
		return "", fmt.Errorf("falta el host")
	}
	pathParts := make([]string, 0, len(structured.Path))
	for _, rawPart := range structured.Path {
		var part string
		if err := json.Unmarshal(rawPart, &part); err == nil {
			pathParts = append(pathParts, part)
			continue
		}
		var object struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(rawPart, &object); err != nil {
			return "", fmt.Errorf("segmento de ruta no válido")
		}
		pathParts = append(pathParts, object.Value)
	}
	scheme := structured.Protocol
	if scheme == "" {
		scheme = "https"
	}
	rawURL = scheme + "://" + host + "/" + strings.Join(pathParts, "/")
	var query []string
	for _, param := range structured.Query {
		if param.Disabled {
			continue
		}
		escapedValue := url.QueryEscape(param.Value)
		escapedValue = strings.ReplaceAll(escapedValue, "%7B%7B", "{{")
		escapedValue = strings.ReplaceAll(escapedValue, "%7D%7D", "}}")
		query = append(query, url.QueryEscape(param.Key)+"="+escapedValue)
	}
	if len(query) > 0 {
		rawURL += "?" + strings.Join(query, "&")
	}
	return relativePostmanURL(rawURL)
}

func relativePostmanURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/", nil
	}
	if !strings.Contains(raw, "://") && !strings.HasPrefix(raw, "/") {
		hostEnd := strings.Index(raw, "/")
		host := raw
		if hostEnd >= 0 {
			host = raw[:hostEnd]
		}
		variableHost := strings.HasPrefix(host, "{{") && strings.Contains(host, "}}")
		if strings.Contains(host, ".") || variableHost {
			if hostEnd < 0 {
				if queryIndex := strings.Index(raw, "?"); queryIndex >= 0 {
					raw = "/" + raw[queryIndex:]
				} else {
					raw = "/"
				}
			} else {
				raw = raw[hostEnd:]
			}
		}
	}
	protected, placeholders := protectPostmanVariables(raw)
	parsed, err := url.Parse(protected)
	if err != nil {
		return "", fmt.Errorf("URL no válida: %w", err)
	}
	if parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("solo se admiten URL HTTP o HTTPS")
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	result := path
	if parsed.RawQuery != "" {
		result += "?" + parsed.RawQuery
	}
	for placeholder, variable := range placeholders {
		result = strings.ReplaceAll(result, placeholder, variable)
	}
	return result, nil
}

func protectPostmanVariables(value string) (string, map[string]string) {
	var protected strings.Builder
	variables := make(map[string]string)
	for i := 0; i < len(value); {
		if strings.HasPrefix(value[i:], "{{") {
			end := strings.Index(value[i+2:], "}}")
			if end >= 0 {
				end += i + 2
				token := fmt.Sprintf("DIFFBENCHVAR%d", len(variables))
				variable := value[i : end+2]
				variables[token] = variable
				protected.WriteString(token)
				i = end + 2
				continue
			}
		}
		protected.WriteByte(value[i])
		i++
	}
	return protected.String(), variables
}

func postmanValue(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	if !json.Valid(raw) {
		return "", fmt.Errorf("valor JSON no válido")
	}
	return string(raw), nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
