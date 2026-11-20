package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"text/template"

	"github.com/grafana/authlib/types"
	"golang.org/x/net/http/httpguts"
	"k8s.io/kube-openapi/pkg/spec3"
	strictjson "sigs.k8s.io/json"
)

// ProxyTarget contains the plugin proxy options of an OpenAPI operation. Its
// path and method are supplied by the operation. Template values are resolved
// against the parent resource at runtime.
type ProxyTarget struct {
	ReqRole   types.RoleType  `json:"reqRole,omitempty"`
	ReqAction string          `json:"reqAction,omitempty"`
	URL       string          `json:"url,omitempty"`
	URLParams []ProxyURLParam `json:"urlParams,omitempty"`
	Headers   []ProxyHeader   `json:"headers,omitempty"`
	// AuthType selects the upstream token provider. When empty, TokenAuth takes
	// precedence over JwtTokenAuth; neither means no token is added.
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

const (
	// ProxyAuthTypeAzure uses TokenAuth with Azure managed identity or client-secret credentials.
	ProxyAuthTypeAzure = "azure"
	// ProxyAuthTypeGCE uses Google application default credentials and JwtTokenAuth's scopes.
	ProxyAuthTypeGCE = "gce"
	// ProxyAuthTypeJWT uses Google service-account credentials from JwtTokenAuth.
	ProxyAuthTypeJWT = "jwt"
)

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
	if err == nil {
		err = target.validate()
	}
	if err != nil {
		return nil, true, fmt.Errorf("invalid %s: %w", ExtensionProxy, err)
	}
	return target, true, nil
}

func decodeProxyTarget(raw any) (*ProxyTarget, error) {
	var data []byte
	switch value := raw.(type) {
	case ProxyTarget:
		// Re-encoding a body can escape characters inside template expressions,
		// changing comparisons and other template behavior.
		return &value, nil
	case *ProxyTarget:
		if value == nil {
			return nil, errors.New("must be an object")
		}
		target := *value
		return &target, nil
	case json.RawMessage:
		data = value
	default:
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(raw); err != nil {
			return nil, err
		}
		data = buf.Bytes()
	}
	var target *ProxyTarget
	strictErrors, err := strictjson.UnmarshalStrict(data, &target)
	if err != nil {
		return nil, err
	}
	if len(strictErrors) > 0 {
		return nil, errors.Join(strictErrors...)
	}
	if target == nil {
		return nil, errors.New("must be an object")
	}
	// JSON decoding accepts null for strings and list entries as zero values.
	// Reject those declarations instead of silently dropping an access check.
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
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
	if len(p.Body) > 0 && !json.Valid(p.Body) {
		return errors.New("body must be valid JSON")
	}
	if p.ReqRole != "" && !p.ReqRole.IsValid() {
		return fmt.Errorf("invalid reqRole %q", p.ReqRole)
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
