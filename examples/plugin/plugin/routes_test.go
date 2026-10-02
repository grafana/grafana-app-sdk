package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
)

func TestPluginRoutes(t *testing.T) {
	p := &ManagedApp{}

	routes, err := p.ProvideRoutes("v1")
	require.NoError(t, err)

	ws, err := routes.WebService("v1", &app.ManifestData{
		Versions: []app.ManifestVersion{
			{Name: "v1", Served: true, Kinds: []app.ManifestVersionKind{}},
		},
	})
	require.NoError(t, err)

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
