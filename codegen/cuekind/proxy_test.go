package cuekind

import (
	"encoding/json"
	"fmt"
	"testing"
	"testing/fstest"

	cueerrors "cuelang.org/go/cue/errors"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/app/appmanifest/v1alpha2"
	"github.com/grafana/grafana-app-sdk/codegen/jennies"
	"github.com/grafana/grafana-app-sdk/routes"
)

func TestManifestGenerator_Proxy(t *testing.T) {
	for _, source := range []struct {
		name, declaration, path string
	}{
		{"namespaced", `routes: namespaced: "/proxy": GET: {operationId: "getProxy", extensions: "x-grafana-proxy": %s}`, "/namespaces/{namespace}/proxy"},
		{"cluster", `routes: cluster: "/proxy": GET: {operationId: "getProxy", extensions: "x-grafana-proxy": %s}`, "/proxy"},
		{"kind", `kinds: [{kind: "Thing", schema: spec: title: string, routes: "/proxy": GET: {operationId: "getProxy", extensions: "x-grafana-proxy": %s}}]`, "/namespaces/{namespace}/things/{name}/proxy"},
		{"imported", `importOpenAPIFile: "proxy.json"`, "/proxy"},
		{"openapi", `openapi: paths: "/proxy": get: {operationId: "getProxy", "x-grafana-proxy": %s}`, "/proxy"},
	} {
		t.Run(source.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, value, wantErr string
			}{
				{"valid", `{url: "{{.URL}}/api", reqRole: "Editor", headers: [{name: "Authorization", content: "Bearer {{.SecureJsonData.token | orEmpty}}"}], body: {id: "{{.JsonData.id}}"}}`, ""},
				{"lowercase role", `{url: "{{.URL}}/api", reqRole: "editor", headers: [{name: "Authorization", content: "Bearer {{.SecureJsonData.token | orEmpty}}"}], body: {id: "{{.JsonData.id}}"}}`, ""},
				{"invalid role", `{reqRole: "SuperAdmin"}`, "SuperAdmin"},
				{"unknown field", `{reqRol: "Admin"}`, "reqRol"},
				{"incorrect field case", `{ReqRole: "Admin"}`, "ReqRole"},
				{"incorrect nested case", `{tokenAuth: {URL: "https://example.com"}}`, "URL"},
				{"null role", `{reqRole: null}`, "reqRole"},
				{"missing header content", `{headers: [{name: "Authorization"}]}`, "content"},
				{"wrong type", `{url: 123}`, "url"},
				{"invalid auth", `{authType: "unknown"}`, "authType"},
				{"missing token", `{authType: "azure"}`, "requires tokenAuth"},
				{"invalid template", `{url: "{{"}`, "url"},
				{"invalid nested type", `{tokenAuth: {params: {secret: 123}}}`, "params"},
				{"invalid nested field", `{tokenAuth: {uri: "https://example.com"}}`, "uri"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					c := testingCue(t)
					declaration := source.declaration
					if source.name == "imported" {
						value, err := c.Root.Context().CompileString(tc.value).MarshalJSON()
						require.NoError(t, err)
						c.files = fstest.MapFS{"proxy.json": &fstest.MapFile{Data: []byte(fmt.Sprintf(`{"openapi":"3.0.3","paths":{"/proxy":{"get":{"operationId":"getProxy","x-grafana-proxy":%s}}}}`, value))}}
					} else {
						declaration = fmt.Sprintf(declaration, tc.value)
					}
					c.Root = c.Root.Context().CompileString(fmt.Sprintf(`manifest: {
						appName: "proxy-app"
						versions: v1: {%s}
					}`, declaration))
					parser, err := NewParser(c, false)
					require.NoError(t, err)
					manifest, err := parser.ParseManifest("manifest")
					if err != nil {
						require.NotEmpty(t, tc.wantErr)
						require.Contains(t, cueerrors.Details(err, nil), tc.wantErr)
						return
					}
					versions := []string{jennies.VersionV1Alpha1, jennies.VersionV1Alpha2}
					if source.name == "openapi" || source.name == "imported" {
						versions = []string{jennies.VersionV1Alpha2}
					}
					for _, version := range versions {
						files, err := ManifestGenerator(ManifestGeneratorConfig{Extension: "json", Version: version}).Generate(manifest)
						if tc.wantErr != "" {
							require.ErrorContains(t, err, tc.wantErr)
							require.ErrorContains(t, err, "v1 GET "+source.path+": invalid "+routes.ExtensionProxy)
							continue
						}
						require.NoError(t, err)
						require.Len(t, files, 1)
						if version == jennies.VersionV1Alpha2 {
							var output struct {
								Spec v1alpha2.AppManifestSpec `json:"spec"`
							}
							require.NoError(t, json.Unmarshal(files[0].Data, &output))
							data, err := output.Spec.ToManifestData()
							require.NoError(t, err)
							op := data.Versions[0].OpenAPI.Paths[source.path].Get
							require.NotNil(t, op)
							target, declared, err := routes.ProxyTargetFromOperation(op)
							require.NoError(t, err)
							require.True(t, declared)
							require.Equal(t, types.RoleEditor, target.ReqRole)
							require.Equal(t, "{{.URL}}/api", target.URL)
							require.Equal(t, "Bearer {{.SecureJsonData.token | orEmpty}}", target.Headers[0].Content)
						}
					}
					_, err = ManifestGoGenerator(ManifestGoGeneratorConfig{Package: "manifestdata", SkipImportsProcess: true}).Generate(manifest)
					if tc.wantErr != "" {
						require.ErrorContains(t, err, tc.wantErr)
					} else {
						require.NoError(t, err)
					}
				})
			}
		})
	}
	require.Equal(t, app.OpenAPIExtensionProxy, routes.ExtensionProxy)
}

func TestParseOpenAPIRoutesPreservesLargeProxyIntegers(t *testing.T) {
	doc := `openapi: 3.0.3
paths:
  /things:
    get:
      x-grafana-proxy:
        body: {id: 9007199254740993, t: "{{if eq .X ` + "`<`" + `}}y{{end}}"}
      responses: {"200": {description: ok}}
`
	routes, err := parseOpenAPIRoutes([]byte(doc), "g.ext", "v1")
	require.NoError(t, err)
	var found bool
	for _, p := range routes.Namespaced {
		if p.Get != nil {
			raw, err := json.Marshal(p.Get.Extensions["x-grafana-proxy"])
			require.NoError(t, err)
			require.Contains(t, string(raw), "9007199254740993")
			found = true
		}
	}
	for _, p := range routes.Cluster {
		if p.Get != nil {
			raw, err := json.Marshal(p.Get.Extensions["x-grafana-proxy"])
			require.NoError(t, err)
			require.Contains(t, string(raw), "9007199254740993")
			found = true
		}
	}
	require.True(t, found)
}
