package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/stretchr/testify/require"
)

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
