package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"text/template"

	"golang.org/x/net/http/httpguts"
	"k8s.io/kube-openapi/pkg/spec3"
)

const (
	ProxyAuthTypeAzure = "azure"
	ProxyAuthTypeGCE   = "gce"
	ProxyAuthTypeJWT   = "jwt"
)

// ProxyTarget contains the plugin proxy options of an OpenAPI operation. Its
// path and method are supplied by the operation. Template values are resolved
// against plugin settings at runtime.
type ProxyTarget struct {
	ReqRole      string          `json:"reqRole,omitempty"`
	ReqAction    string          `json:"reqAction,omitempty"`
	URL          string          `json:"url,omitempty"`
	URLParams    []ProxyURLParam `json:"urlParams,omitempty"`
	Headers      []ProxyHeader   `json:"headers,omitempty"`
	AuthType     string          `json:"authType,omitempty"`
	TokenAuth    *ProxyTokenAuth `json:"tokenAuth,omitempty"`
	JwtTokenAuth *ProxyTokenAuth `json:"jwtTokenAuth,omitempty"`
	Body         json.RawMessage `json:"body,omitempty"`
}

type ProxyURLParam struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type ProxyHeader struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type ProxyTokenAuth struct {
	URL    string            `json:"url,omitempty"`
	Scopes []string          `json:"scopes,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

// ProxyTargetFromOperation decodes and validates ExtensionProxy, if present.
// Validation parses templates without executing them, since settings and secrets
// are only available to the server handling the request.
func ProxyTargetFromOperation(op *spec3.Operation) (*ProxyTarget, bool, error) {
	if op == nil {
		return nil, false, nil
	}
	raw, ok := op.Extensions[ExtensionProxy]
	if !ok {
		return nil, false, nil
	}
	target, err := decodeProxyTarget(raw)
	if err != nil {
		return nil, true, fmt.Errorf("invalid %s: %w", ExtensionProxy, err)
	}
	return target, true, nil
}

func decodeProxyTarget(raw any) (*ProxyTarget, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var target *ProxyTarget
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&target); err != nil {
		return nil, err
	}
	if target == nil {
		return nil, errors.New("must be an object")
	}
	// JSON decoding accepts null for strings and list entries as zero values.
	// Reject those declarations instead of silently dropping an access check.
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	delete(fields, "body") // A body can be any JSON value, including null.
	if err := rejectProxyNulls(fields, ""); err != nil {
		return nil, err
	}
	for _, field := range []string{"headers", "urlParams"} {
		entries, ok := fields[field].([]any)
		if !ok {
			continue
		}
		for i, entry := range entries {
			values, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s[%d] must be an object", field, i)
			}
			if _, ok := values["content"]; !ok {
				return nil, fmt.Errorf("%s[%d].content is required", field, i)
			}
		}
	}
	if err := target.validate(); err != nil {
		return nil, err
	}
	return target, nil
}

func rejectProxyNulls(value any, path string) error {
	switch v := value.(type) {
	case nil:
		return fmt.Errorf("%s must not be null", path)
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(v)) {
			child := key
			if path != "" {
				child = path + "." + key
			}
			if err := rejectProxyNulls(v[key], child); err != nil {
				return err
			}
		}
	case []any:
		for i, entry := range v {
			if err := rejectProxyNulls(entry, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	return nil
}

func (p ProxyTarget) validate() error {
	switch p.ReqRole {
	case "", "None", "Viewer", "Editor", "Admin":
	default:
		return errors.New("reqRole must be None, Viewer, Editor, or Admin")
	}
	switch p.AuthType {
	case "":
	case ProxyAuthTypeAzure:
		if p.TokenAuth == nil {
			return errors.New("authType azure requires tokenAuth")
		}
	case ProxyAuthTypeGCE, ProxyAuthTypeJWT:
		if p.JwtTokenAuth == nil {
			return fmt.Errorf("authType %s requires jwtTokenAuth", p.AuthType)
		}
	default:
		return errors.New("authType must be azure, gce, or jwt")
	}
	if err := validateProxyTemplate("url", p.URL); err != nil {
		return err
	}
	for i, header := range p.Headers {
		if !httpguts.ValidHeaderFieldName(header.Name) {
			return fmt.Errorf("headers[%d].name must be a valid HTTP header name", i)
		}
		if err := validateProxyTemplate(fmt.Sprintf("headers[%d].content", i), header.Content); err != nil {
			return err
		}
	}
	for i, param := range p.URLParams {
		if param.Name == "" {
			return fmt.Errorf("urlParams[%d].name must not be empty", i)
		}
		if err := validateProxyTemplate(fmt.Sprintf("urlParams[%d].name", i), param.Name); err != nil {
			return err
		}
		if err := validateProxyTemplate(fmt.Sprintf("urlParams[%d].content", i), param.Content); err != nil {
			return err
		}
	}
	for _, entry := range []struct {
		name string
		auth *ProxyTokenAuth
	}{{"tokenAuth", p.TokenAuth}, {"jwtTokenAuth", p.JwtTokenAuth}} {
		if entry.auth == nil {
			continue
		}
		if err := validateProxyTemplate(entry.name+".url", entry.auth.URL); err != nil {
			return err
		}
		for _, key := range slices.Sorted(maps.Keys(entry.auth.Params)) {
			if err := validateProxyTemplate(entry.name+".params."+key, entry.auth.Params[key]); err != nil {
				return err
			}
		}
	}
	return validateProxyTemplate("body", string(p.Body))
}

func validateProxyTemplate(field, value string) error {
	// The plugin proxy adds orEmpty to text/template's built-in functions.
	_, err := template.New(field).Funcs(template.FuncMap{
		"orEmpty": func(v any) any { return v },
	}).Parse(value)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}
