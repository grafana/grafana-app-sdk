package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// newOpenAPIHandler registers the document's paths and methods and passes the
// matched path and operation (including vendor extensions) to the plugin callback.
// This prototype uses paths relative to /; it does not apply OpenAPI servers,
// validate request/response schemas, or enforce authorization itself.
func newOpenAPIHandler(doc *openapi3.T, callback func(*openapi3.PathItem, *openapi3.Operation, *restful.Request, *restful.Response)) http.Handler {
	ws := new(restful.WebService).Path("/").Produces(restful.MIME_JSON)
	for path, item := range doc.Paths.Map() {
		for method, operation := range item.Operations() {
			routePath := path
			// OpenAPI exports {path:*} as {path}; this extension preserves the
			// go-restful catch-all semantics when reconstructing the route.
			if parameter, ok := operation.Extensions["x-grafana-catch-all"].(string); ok {
				routePath = strings.TrimSuffix(path, "{"+parameter+"}") + "{" + parameter + ":*}"
			}
			ws.Route(ws.Method(method).Path(routePath).Operation(operation.OperationID).
				To(func(request *restful.Request, response *restful.Response) {
					callback(item, operation, request, response)
				}))
		}
	}
	container := restful.NewContainer()
	container.Add(ws)
	return container
}

// newDirectOpenAPIHandler builds standard-library routes directly from OpenAPI.
// Roles accumulate along the document hierarchy, not URL prefixes. Both extension
// spellings are accepted; each occurrence must be a string, and duplicates remain.
// Paths are relative to /, using ServeMux routing semantics (including GET matching
// HEAD). Servers and request/response validation are outside this prototype.
func newDirectOpenAPIHandler(doc *openapi3.T, callback func(*openapi3.PathItem, *openapi3.Operation, []string, http.ResponseWriter, *http.Request)) (http.Handler, error) {
	if doc == nil || doc.Paths == nil {
		return nil, fmt.Errorf("OpenAPI document must contain paths")
	}
	mux := http.NewServeMux()
	for path, item := range doc.Paths.Map() {
		for method, operation := range item.Operations() {
			var roles []string
			for _, level := range []struct {
				name       string
				extensions map[string]any
			}{
				{"document", doc.Extensions},
				{"paths", doc.Paths.Extensions},
				{"path " + path, item.Extensions},
				{method + " " + path, operation.Extensions},
			} {
				for _, key := range []string{"x-grafana-require-role", "x-grafana-requires-role"} {
					if value, exists := level.extensions[key]; exists {
						role, ok := value.(string)
						if !ok || role == "" {
							return nil, fmt.Errorf("%s: %s must be a nonempty string", level.name, key)
						}
						roles = append(roles, role)
					}
				}
			}
			routePath := path
			if value, exists := operation.Extensions["x-grafana-catch-all"]; exists {
				parameter, ok := value.(string)
				prefix, matches := strings.CutSuffix(path, "{"+parameter+"}")
				if !ok || parameter == "" || !matches {
					return nil, fmt.Errorf("%s %s: x-grafana-catch-all must name the final path parameter", method, path)
				}
				routePath = prefix + "{" + parameter + "...}"
			}
			mux.HandleFunc(method+" "+routePath, func(w http.ResponseWriter, r *http.Request) {
				// The callback may mutate its list without affecting future requests.
				callback(item, operation, slices.Clone(roles), w, r)
			})
		}
	}
	return mux, nil
}

func TestDirectOpenAPIHandler(t *testing.T) {
	snapshot := filepath.Join("..", "..", "..", "codegen", "cuekind", "testing", "integration.openapi.json")
	doc, err := openapi3.NewLoader().LoadFromFile(snapshot)
	require.NoError(t, err)
	doc.Extensions = map[string]any{"x-grafana-require-role": "signed-in"}
	doc.Paths.Extensions = map[string]any{"x-grafana-require-role": "api-user"}
	// Unrelated metadata is not an ancestor of an operation.
	doc.Info.Extensions = map[string]any{"x-grafana-require-role": "ignored"}

	for _, tc := range []struct {
		name, method, path, operation, parameter, value string
		roles                                           []string
		status                                          int
	}{
		{name: "all levels", method: http.MethodGet, path: "/foo", operation: "getFoo",
			roles: []string{"signed-in", "api-user", "some-role", "viewer"}, status: http.StatusOK},
		{name: "POST accumulates instead of overriding", method: http.MethodPost, path: "/foo", operation: "postFoo",
			roles: []string{"signed-in", "api-user", "some-role", "editor"}, status: http.StatusOK},
		{name: "catch-all is a sibling path", method: http.MethodGet, path: "/foo/a/b/file.json", operation: "getFooPath", parameter: "path", value: "a/b/file.json",
			roles: []string{"signed-in", "api-user", "viewer"}, status: http.StatusOK},
		{name: "path parameter with ancestor roles only", method: http.MethodGet, path: "/namespaces/default/things/bar", operation: "getBar", parameter: "namespace", value: "default",
			roles: []string{"signed-in", "api-user"}, status: http.StatusOK},
		{name: "unknown path", method: http.MethodGet, path: "/missing", status: http.StatusNotFound},
		{name: "unsupported method", method: http.MethodDelete, path: "/foo", status: http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			handler, err := newDirectOpenAPIHandler(doc, func(item *openapi3.PathItem, operation *openapi3.Operation, roles []string, w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, tc.operation, operation.OperationID)
				require.Same(t, item.GetOperation(tc.method), operation)
				require.Equal(t, tc.roles, roles)
				require.Equal(t, tc.value, r.PathValue(tc.parameter))
				require.Equal(t, "hello", r.URL.Query().Get("input"))
				roles[0] = "changed by callback"
				w.WriteHeader(http.StatusOK)
			})
			require.NoError(t, err)
			// Repeating the request also verifies callback mutation isolation.
			for range 2 {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path+"?input=hello", nil))
				require.Equal(t, tc.status, response.Code)
			}
			if tc.operation != "" {
				require.Equal(t, 2, calls)
			} else {
				require.Zero(t, calls)
			}
		})
	}

	t.Run("invalid role fails construction", func(t *testing.T) {
		doc.Extensions["x-grafana-require-role"] = 123
		_, err := newDirectOpenAPIHandler(doc, nil)
		require.ErrorContains(t, err, "document: x-grafana-require-role must be a nonempty string")
	})
}

func TestOpenAPIHandler(t *testing.T) {
	snapshot := filepath.Join("..", "..", "..", "codegen", "cuekind", "testing", "integration.openapi.json")
	doc, err := openapi3.NewLoader().LoadFromFile(snapshot)
	require.NoError(t, err)

	for _, tc := range []struct {
		name         string
		method       string
		path         string
		status       int
		operation    string
		role         string
		namespace    string
		input        string
		capturedPath string
	}{
		{
			name: "GET operation role", method: http.MethodGet, path: "/foo?input=hello",
			status: http.StatusOK, operation: "getFoo", role: "viewer", input: "hello",
		},
		{
			name: "path parameter", method: http.MethodGet, path: "/namespaces/default/things/bar",
			status: http.StatusOK, operation: "getBar", namespace: "default",
		},
		{
			name: "operation override", method: http.MethodPost, path: "/foo?input=hello",
			status: http.StatusOK, operation: "postFoo", role: "editor", input: "hello",
		},
		{
			name: "catch-all single segment", method: http.MethodGet, path: "/foo/a",
			status: http.StatusOK, operation: "getFooPath", role: "viewer", capturedPath: "a",
		},
		{
			name: "catch-all multiple segments", method: http.MethodGet, path: "/foo/a/b/file.json?input=hello",
			status: http.StatusOK, operation: "getFooPath", role: "viewer", capturedPath: "a/b/file.json", input: "hello",
		},
		{name: "unknown path", method: http.MethodGet, path: "/missing", status: http.StatusNotFound},
		{name: "unsupported method", method: http.MethodDelete, path: "/foo", status: http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			handler := newOpenAPIHandler(doc, func(item *openapi3.PathItem, operation *openapi3.Operation, request *restful.Request, response *restful.Response) {
				called = true
				require.Equal(t, tc.operation, operation.OperationID)
				// A plugin callback can use this metadata to perform its role check.
				role, hasRole := operation.Extensions["x-grafana-requires-role"]
				if !hasRole {
					role, hasRole = item.Extensions["x-grafana-requires-role"]
				}
				if tc.role != "" {
					require.Equal(t, tc.role, role)
				} else {
					require.False(t, hasRole)
				}
				require.Equal(t, tc.capturedPath, request.PathParameter("path"))
				require.Equal(t, tc.namespace, request.PathParameter("namespace"))
				require.Equal(t, tc.input, request.QueryParameter("input"))
				require.NoError(t, response.WriteEntity(map[string]string{"operation": operation.OperationID}))
			})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			require.Equal(t, tc.status, response.Code)
			require.Equal(t, tc.operation != "", called)
			if called {
				require.JSONEq(t, `{"operation":"`+tc.operation+`"}`, response.Body.String())
			}
		})
	}
}

func TestPluginRoutes(t *testing.T) {
	p := &ManagedApp{}

	ws, err := p.ProvideRoutes()
	require.NoError(t, err)

	ws = FilterWebService(ws, "/apis/group/v1")

	// Generate Swagger from route metadata and Go structs, then convert to the
	// OpenAPI 3.0 format accepted by the manifest importer. BuildOAS3 in v2.12.1
	// emits Swagger-only consumes/produces fields, so use this conversion path.
	swagger := restfulspec.BuildSwagger(restfulspec.Config{
		WebServices: []*restful.WebService{ws},
	})

	data, err := json.Marshal(swagger)
	require.NoError(t, err)
	var v2 openapi2.T
	require.NoError(t, json.Unmarshal(data, &v2))
	v2.Info.Title = "Plugin cluster routes"
	v2.Info.Version = "v1"
	doc, err := openapi2conv.ToV3(&v2)
	require.NoError(t, err)
	customizeRouteOpenAPI(doc)
	data, err = json.Marshal(doc)
	require.NoError(t, err)
	data = canonicalOpenAPISnapshot(t, data)

	// Go runs tests from the package directory (examples/plugin/plugin).
	snapshot := filepath.Join("..", "..", "..", "codegen", "cuekind", "testing", "integration.openapi.json")
	data = append(data, '\n')
	previous, err := os.ReadFile(snapshot)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read OpenAPI snapshot: %v", err)
	}
	if bytes.Equal(previous, data) {
		return
	}
	require.NoError(t, os.WriteFile(snapshot, data, 0o644))
	t.Fatalf("updated OpenAPI snapshot %s; review the diff and rerun the test", snapshot)
}

// canonicalOpenAPISnapshot sorts unordered OpenAPI collections recursively.
// JSON object keys are sorted by encoding/json. Payloads and ordered collections
// (for example, server preference lists) retain their array order.
func canonicalOpenAPISnapshot(t *testing.T, data []byte) []byte {
	t.Helper()
	var document any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&document))
	var normalize func(any, string)
	normalize = func(value any, field string) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				switch field {
				case "schemas", "definitions", "properties", "patternProperties", "paths", "responses", "content", "headers":
					// These keys are user-defined names, not OpenAPI keywords.
					normalize(child, "")
					continue
				case "securityRequirement":
					// Each scheme maps to an unordered list of scope names.
					normalize(child, "required")
					continue
				}
				if strings.HasPrefix(key, "x-") {
					continue
				}
				switch key {
				case "example", "examples", "default", "const":
					continue
				}
				normalize(child, key)
			}
		case []any:
			// Enum values can themselves contain arrays whose order is meaningful.
			if field != "enum" {
				for _, child := range value {
					childField := ""
					if field == "security" {
						childField = "securityRequirement"
					}
					normalize(child, childField)
				}
			}
			switch field {
			case "required", "enum", "allOf", "anyOf", "oneOf", "parameters", "tags", "security", "type", "consumes", "produces", "schemes":
				// Sort by canonical JSON so objects and mixed values have a total order.
				encoded := make([]json.RawMessage, len(value))
				for i, child := range value {
					var err error
					encoded[i], err = json.Marshal(child)
					require.NoError(t, err)
				}
				slices.SortFunc(encoded, func(a, b json.RawMessage) int { return bytes.Compare(a, b) })
				for i := range value {
					value[i] = encoded[i]
				}
			}
		}
	}
	normalize(document, "")
	result, err := json.MarshalIndent(document, "", "  ")
	require.NoError(t, err)
	return result
}

func TestCanonicalOpenAPISnapshot(t *testing.T) {
	first := []byte(`{"schemas":{"Thing":{"required":["Namespace","Name"],"allOf":[{"required":["z","a"]},{"type":"string"}]}},"parameters":[{"name":"z"},{"name":"a"}],"enum":[[2,1],[1,2]],"example":{"required":["z","a"],"id":9007199254740993},"servers":[{"url":"z"},{"url":"a"}],"x-custom":[2,1]}`)
	second := []byte(`{"x-custom":[2,1],"servers":[{"url":"z"},{"url":"a"}],"example":{"id":9007199254740993,"required":["z","a"]},"enum":[[1,2],[2,1]],"parameters":[{"name":"a"},{"name":"z"}],"schemas":{"Thing":{"allOf":[{"type":"string"},{"required":["a","z"]}],"required":["Name","Namespace"]}}}`)
	canonical := canonicalOpenAPISnapshot(t, first)
	require.Equal(t, canonical, canonicalOpenAPISnapshot(t, second))
	require.Equal(t, canonical, canonicalOpenAPISnapshot(t, canonical))
	require.Contains(t, string(canonical), "9007199254740993")
	var result map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(canonical, &result))
	require.JSONEq(t, `{"required":["z","a"],"id":9007199254740993}`, string(result["example"]))
	require.JSONEq(t, `[{"url":"z"},{"url":"a"}]`, string(result["servers"]))
	require.JSONEq(t, `[2,1]`, string(result["x-custom"]))
	require.JSONEq(t, `[[1,2],[2,1]]`, string(result["enum"]))
}

func TestFilterWebService(t *testing.T) {
	ws := new(restful.WebService).Path("/apis/group/v1").Produces(restful.MIME_JSON)
	handler := func(*restful.Request, *restful.Response) {}
	ws.Route(ws.GET("/foo").To(handler).Operation("getFoo").Doc("Get foo").
		Writes(map[string]string{}).Returns(200, "OK", map[string]string{}).
		AddExtension("x-grafana-requires-role", "viewer"))
	ws.Route(ws.GET("").To(handler))
	ws.Path("/apis/group/v2")
	ws.Route(ws.GET("/bar").To(handler))

	for _, tc := range []struct {
		prefix string
		paths  []string
	}{
		{"/apis/group/v1", []string{"/foo", "/"}},
		{"/apis/group/v1/", []string{"/foo", "/"}},
		{"/missing", nil},
		{"", []string{"/apis/group/v1/foo", "/apis/group/v1/", "/apis/group/v2/bar"}},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			filtered := FilterWebService(ws, tc.prefix)
			require.NotSame(t, ws, filtered)
			require.Equal(t, "/", filtered.RootPath())
			require.Len(t, filtered.Routes(), len(tc.paths))
			for i, path := range tc.paths {
				require.Equal(t, path, filtered.Routes()[i].Path)
			}
			if len(tc.paths) > 0 {
				original, copied := ws.Routes()[0], filtered.Routes()[0]
				require.Equal(t, original.Operation, copied.Operation)
				require.Equal(t, original.Doc, copied.Doc)
				require.Equal(t, original.Produces, copied.Produces)
				require.Equal(t, original.WriteSample, copied.WriteSample)
				require.Equal(t, original.ResponseErrors, copied.ResponseErrors)
				require.Equal(t, original.Extensions, copied.Extensions)
			}
			require.Equal(t, "/apis/group/v1/foo", ws.Routes()[0].Path)
			require.Len(t, ws.Routes(), 3)
		})
	}
	require.Nil(t, FilterWebService(nil, "/apis/group/v1"))
}

// FilterWebService copies matching routes for OpenAPI generation, removing prefix
// from their documented paths while preserving their metadata.
func FilterWebService(ws *restful.WebService, prefix string) *restful.WebService {
	if ws == nil {
		return nil
	}

	filtered := new(restful.WebService).
		Path("/").
		Doc(ws.Documentation()).
		ApiVersion(ws.Version())
	for _, parameter := range ws.PathParameters() {
		filtered.Param(parameter)
	}
	for _, route := range ws.Routes() {
		if after, ok := strings.CutPrefix(route.Path, prefix); ok {
			route.Path = "/" + strings.TrimLeft(after, "/")
			filtered.Route(filtered.Method(route.Method).Path(route.Path).To(route.Function))
			routes := filtered.Routes()
			routes[len(routes)-1] = route
		}
	}
	return filtered
}
