package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
	data, err = json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)

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
