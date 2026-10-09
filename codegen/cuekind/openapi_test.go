package cuekind

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/grafana/codejen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"sigs.k8s.io/yaml"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/app/appmanifest/v1alpha2"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/jennies"
	"github.com/grafana/grafana-app-sdk/routes"
)

const externalManifestCUE = `package test
manifest: {
    appName: "external"
    versions: {
        v1: {
            kinds: [{kind: "Widget", schema: spec: title: string}]
            routes: cluster: "/query": {
                GET: {name: "getCueQuery", response: {original: string}}
                POST: {name: "createCuePost", response: {retained: string}}
            }
        }
        v2: {kinds: []}
    }
    roles: reader: {title: "Reader", routes: ["externalQuery"]}
}`

const externalOpenAPI = `openapi: 3.0.3
info: {title: External API, version: v1}
paths:
  /apis/external.ext.grafana.app/v1/query:
    get:
      operationId: externalQuery
      x-grafana-declared-authz-resource: query
      x-grafana-declared-authz-verb: get
      parameters:
        - $ref: '#/components/parameters/Query'
      responses:
        '200':
          $ref: '#/components/responses/Result'
  /namespaces/{namespace}/query:
    post:
      operationId: externalNamespacedQuery
      requestBody:
        $ref: '#/components/requestBodies/Query'
      responses:
        '200':
          $ref: '#/components/responses/Result'
components:
  schemas:
    Result:
      type: object
      required: [message]
      properties:
        message: {type: string, description: A message}
        next: {$ref: '#/components/schemas/Result'}
      x-kubernetes-preserve-unknown-fields: true
  parameters:
    Query: {name: q, in: query, schema: {type: string}}
  requestBodies:
    Query:
      content:
        application/json:
          schema: {$ref: '#/components/schemas/Result'}
  responses:
    Result:
      description: A result
      content:
        application/json:
          schema: {$ref: '#/components/schemas/Result'}
`

func externalTestFiles(cueSource string) fstest.MapFS {
	return fstest.MapFS{
		"cue.mod/module.cue": &fstest.MapFile{Data: []byte("module: \"example.com/test\"\nlanguage: version: \"v0.8.2\"")},
		"manifest.cue":       &fstest.MapFile{Data: []byte(cueSource)},
	}
}

func TestManifestExternalOpenAPI(t *testing.T) {
	for _, encoding := range []string{"json", "yaml"} {
		t.Run(encoding, func(t *testing.T) {
			files := externalTestFiles(externalManifestCUE)
			doc := []byte(externalOpenAPI)
			if encoding == "json" {
				var err error
				doc, err = yaml.YAMLToJSON(doc)
				require.NoError(t, err)
			}
			files["openapi.v1."+encoding] = &fstest.MapFile{Data: doc}
			c, err := LoadCue(files)
			require.NoError(t, err)
			parser, err := NewParser(c, false)
			require.NoError(t, err)
			manifest, err := parser.ParseManifest("manifest")
			require.NoError(t, err)
			// External operations never become inputs to the type generators.
			assert.Equal(t, "getCueQuery", manifest.Versions()[0].Routes().Cluster["/query"]["GET"].Name)
			assert.Empty(t, manifest.Versions()[0].Routes().Namespaced)
			delete(files, "openapi.v1."+encoding)
			baseCue, err := LoadCue(files)
			require.NoError(t, err)
			baseParser, err := NewParser(baseCue, false)
			require.NoError(t, err)
			baseManifest, err := baseParser.ParseManifest("manifest")
			require.NoError(t, err)
			// Compare actual generated files, not just the parser's CUE view.
			for _, generator := range []*codejen.JennyList[codegen.AppManifest]{
				ResourceGenerator("example.com/test", "generated", true),
				TypeScriptResourceGenerator(),
			} {
				baseline, err := generator.Generate(baseManifest)
				require.NoError(t, err)
				withOpenAPI, err := generator.Generate(manifest)
				require.NoError(t, err)
				assert.ElementsMatch(t, baseline, withOpenAPI)
			}
			routeTypes := &jennies.CustomRouteGoTypesJenny{
				SkipImportsProcess: true,
				OpenAPINamer:       func(info jennies.OpenAPINamerInfo) string { return info.TypeName },
			}
			baselineTypes, err := routeTypes.Generate(baseManifest)
			require.NoError(t, err)
			require.NotEmpty(t, baselineTypes)
			importedTypes, err := routeTypes.Generate(manifest)
			require.NoError(t, err)
			assert.ElementsMatch(t, baselineTypes, importedTypes)
			for _, includeSchemas := range []bool{true, false} {
				generator := &jennies.ManifestGenerator{Encoder: json.Marshal, FileExtension: "json", IncludeSchemas: includeSchemas, ManifestVersion: "v1alpha2"}
				if encoding == "yaml" {
					generator.Encoder = yaml.Marshal
					generator.FileExtension = "yaml"
				}
				generated, err := generator.Generate(manifest)
				require.NoError(t, err)
				var output struct {
					Spec v1alpha2.AppManifestSpec `json:"spec"`
				}
				require.NoError(t, yaml.Unmarshal(generated[0].Data, &output))
				data, err := output.Spec.ToManifestData()
				require.NoError(t, err)
				assertExternalRoutes(t, data.Versions[0].Routes)
				assert.Empty(t, data.Versions[1].Routes)
			}
			legacy := &jennies.ManifestGenerator{Encoder: json.Marshal, ManifestVersion: "v1alpha1"}
			_, err = legacy.Generate(manifest)
			require.ErrorContains(t, err, "require manifestVersion v1alpha2")
			goGenerator := &jennies.ManifestGoGenerator{Package: "manifestdata", ProjectRepo: "example.com/test", CodegenPath: "generated", IncludeSchemas: true, SkipImportsProcess: true}
			generated, err := goGenerator.Generate(manifest)
			require.NoError(t, err)
			goSource := string(generated[0].Data)
			assert.Contains(t, goSource, "externalNamespacedQuery")
			assert.Contains(t, goSource, "CreateCuePostResponse{}")
			assert.NotContains(t, goSource, "ExternalQueryResponse{}")
			assert.NotContains(t, goSource, "ExternalNamespacedQueryResponse{}")
			assert.NotContains(t, goSource, "GetCueQueryResponse{}")
			// Repeated generation must not mutate the imported document or CUE routes.
			again, err := goGenerator.Generate(manifest)
			require.NoError(t, err)
			assert.Equal(t, generated, again)
		})
	}
}

func TestExternalOpenAPIVersions(t *testing.T) {
	files := externalTestFiles(externalManifestCUE)
	files["openapi.v1.yaml"] = &fstest.MapFile{Data: []byte(externalOpenAPI)}
	v2 := strings.ReplaceAll(externalOpenAPI, "/v1/", "/v2/")
	v2 = strings.ReplaceAll(v2, "A message", "A v2 message")
	files["openapi.v2.yaml"] = &fstest.MapFile{Data: []byte(v2)}
	c, err := LoadCue(files)
	require.NoError(t, err)
	parser, err := NewParser(c, false)
	require.NoError(t, err)
	manifest, err := parser.ParseManifest("manifest")
	require.NoError(t, err)
	generator := &jennies.ManifestGenerator{Encoder: json.Marshal, ManifestVersion: "v1alpha2"}
	generated, err := generator.Generate(manifest)
	require.NoError(t, err)
	var output struct {
		Spec v1alpha2.AppManifestSpec `json:"spec"`
	}
	require.NoError(t, json.Unmarshal(generated[0].Data, &output))
	data, err := output.Spec.ToManifestData()
	require.NoError(t, err)
	assert.Equal(t, "A message", data.Versions[0].Routes.Schemas["Result"].Properties["message"].Description)
	assert.Equal(t, "A v2 message", data.Versions[1].Routes.Schemas["Result"].Properties["message"].Description)
	goGenerator := &jennies.ManifestGoGenerator{Package: "manifestdata", ProjectRepo: "example.com/test", CodegenPath: "generated", SkipImportsProcess: true}
	goFiles, err := goGenerator.Generate(manifest)
	require.NoError(t, err)
	// v2 contains only imported routes, so it has no generated Go package.
	assert.NotContains(t, string(goFiles[0].Data), `"example.com/test/generated/external/v2"`)
}

func assertExternalRoutes(t *testing.T, versionRoutes app.ManifestVersionRoutes) {
	t.Helper()
	cluster := versionRoutes.Cluster["/query"]
	require.NotNil(t, cluster.Get)
	assert.Equal(t, "externalQuery", cluster.Get.OperationId)
	require.NotNil(t, cluster.Post)
	assert.Equal(t, "createCuePost", cluster.Post.OperationId)
	assert.Equal(t, "get", cluster.Get.Extensions[routes.ExtensionAuthzVerb])
	require.Len(t, cluster.Get.Parameters, 1)
	assert.Equal(t, "q", cluster.Get.Parameters[0].Name)
	assert.Equal(t, "#/components/schemas/Result", cluster.Get.Responses.StatusCodeResponses[200].Content["application/json"].Schema.Ref.String())
	namespaced := versionRoutes.Namespaced["/query"]
	require.NotNil(t, namespaced.Post)
	require.NotNil(t, namespaced.Post.RequestBody)
	assert.Equal(t, "#/components/schemas/Result", namespaced.Post.RequestBody.Content["application/json"].Schema.Ref.String())
	schema := versionRoutes.Schemas["Result"]
	assert.Equal(t, []string{"message"}, schema.Required)
	assert.Equal(t, "A message", schema.Properties["message"].Description)
	assert.Equal(t, true, schema.Extensions["x-kubernetes-preserve-unknown-fields"])
	next := schema.Properties["next"]
	assert.Equal(t, "#/components/schemas/Result", next.Ref.String())
}

func TestExternalOpenAPIFileSelection(t *testing.T) {
	for _, tt := range []struct {
		name    string
		setting string
		files   []string
		wantErr string
	}{
		{name: "no external file"},
		{name: "explicit file", setting: `importOpenAPIFile: "saved.yaml"`, files: []string{"saved.yaml"}},
		{name: "yml", files: []string{"openapi.v1.yml"}},
		{name: "missing explicit file", setting: `importOpenAPIFile: "missing.json"`, wantErr: "missing.json"},
		{name: "ambiguous", files: []string{"openapi.v1.json", "openapi.v1.yaml"}, wantErr: "multiple OpenAPI files"},
		{name: "explicit resolves ambiguity", setting: `importOpenAPIFile: "openapi.v1.yaml"`, files: []string{"openapi.v1.json", "openapi.v1.yaml"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := externalTestFiles(`package test
manifest: {appName: "external", versions: v1: {kinds: [], ` + tt.setting + `}}`)
			for _, name := range tt.files {
				files[name] = &fstest.MapFile{Data: []byte(externalOpenAPI)}
			}
			c, err := LoadCue(files)
			require.NoError(t, err)
			parser, err := NewParser(c, false)
			require.NoError(t, err)
			_, err = parser.ParseManifest("manifest")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestOpenAPIImplicitPathParameters(t *testing.T) {
	explicit := &spec3.Parameter{ParameterProps: spec3.ParameterProps{
		Name: "name", In: "path", Required: true, Description: "Explicit constraint",
		Schema: &spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"string"}, Pattern: "^[a-z]+$"}},
	}}
	query := &spec3.Parameter{ParameterProps: spec3.ParameterProps{Name: "name", In: "query"}}
	for _, tt := range []struct {
		name       string
		path       string
		shared     []*spec3.Parameter
		parameters []*spec3.Parameter
		wantGet    int
		wantPost   int
	}{
		{name: "no placeholders", path: "/reports"},
		{name: "namespace", path: "/namespaces/{namespace}/reports", wantGet: 1, wantPost: 1},
		{name: "cluster object", path: "/foos/{name}/report", wantGet: 1, wantPost: 1},
		{name: "namespaced object", path: "/namespaces/{namespace}/foos/{name}/report", wantGet: 2, wantPost: 2},
		{name: "full path", path: "/apis/example.test/v1/namespaces/{namespace}/foos/{name}/report", wantGet: 2, wantPost: 2},
		{name: "shared explicit", path: "/foos/{name}/report", shared: []*spec3.Parameter{explicit}},
		{name: "operation explicit", path: "/foos/{name}/report", parameters: []*spec3.Parameter{explicit}, wantGet: 1, wantPost: 1},
		{name: "query is not a path parameter", path: "/foos/{name}/report", parameters: []*spec3.Parameter{query}, wantGet: 2, wantPost: 1},
		{name: "other placeholder", path: "/reports/{reportID}"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := spec3.PathProps{
				Parameters: tt.shared,
				Get:        &spec3.Operation{OperationProps: spec3.OperationProps{Parameters: tt.parameters}},
				Post:       &spec3.Operation{},
			}
			addImplicitPathParameters(tt.path, &path)
			// Reprocessing must not append duplicate parameters.
			addImplicitPathParameters(tt.path, &path)
			require.Len(t, path.Get.Parameters, tt.wantGet)
			require.Len(t, path.Post.Parameters, tt.wantPost)
			assert.Equal(t, tt.shared, path.Parameters)
			for _, parameter := range append(path.Get.Parameters, path.Post.Parameters...) {
				if parameter == explicit || parameter == query {
					continue
				}
				assert.Equal(t, "path", parameter.In)
				assert.True(t, parameter.Required)
				require.NotNil(t, parameter.Schema)
				assert.Equal(t, spec.StringOrArray{"string"}, parameter.Schema.Type)
			}
			if len(tt.parameters) > 0 {
				assert.Same(t, tt.parameters[0], path.Get.Parameters[0])
			}
		})
	}
	assert.Equal(t, "Explicit constraint", explicit.Description)
	assert.Equal(t, "^[a-z]+$", explicit.Schema.Pattern)
}

func TestExternalOpenAPIErrors(t *testing.T) {
	for _, tt := range []struct{ name, document, wantErr string }{
		{"invalid document", "[", "error"},
		{"unsupported version", "openapi: 3.1.0", "OpenAPI 3.0"},
		{"unresolved ref", strings.ReplaceAll(externalOpenAPI, "#/components/schemas/Result", "#/components/schemas/Missing"), "unresolved reference"},
		{"external ref", strings.ReplaceAll(externalOpenAPI, "#/components/schemas/Result", "other.json#/Result"), "must be local"},
		{"wrong version", strings.ReplaceAll(externalOpenAPI, "/v1/query", "/v2/query"), "does not belong"},
		{"null path", "openapi: 3.0.3\npaths: { /query: null }", "must be an object"},
		{"null shared parameter", `{"openapi":"3.0.3","paths":{"/query":{"parameters":[null],"get":{}}}}`, "parameter must be an object"},
		{"null operation parameter", `{"openapi":"3.0.3","paths":{"/query":{"get":{"parameters":[null]}}}}`, "parameter must be an object"},
		{"cyclic response", "openapi: 3.0.3\ncomponents: { responses: { Loop: { $ref: '#/components/responses/Loop' } } }", "cyclic non-schema reference"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseOpenAPIRoutes([]byte(tt.document), "external.ext.grafana.app", "v1")
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestOpenAPIDefaultResponseAndExamples(t *testing.T) {
	routes, err := parseOpenAPIRoutes([]byte(`{
  "openapi": "3.0.3",
  "paths": {"x-description": {"$ref": "literal extension data"}, "/reports": {"get": {"responses": {
    "default": {"$ref": "#/components/responses/Problem"}
  }}}},
  "components": {
    "responses": {"Problem": {
      "description": "A failure",
      "content": {"application/json": {
        "schema": {"type": "object"},
        "examples": {"failure": {"$ref": "#/components/examples/Failure"}}
      }}
    }},
    "examples": {"Failure": {"value": {"$ref": "literal example data"}}}
  }
}`), "example.test", "v1")
	require.NoError(t, err)
	response := routes.Cluster["/reports"].Get.Responses.Default
	require.NotNil(t, response)
	assert.Empty(t, response.Ref.String())
	assert.Equal(t, "A failure", response.Description)
	example := response.Content["application/json"].Examples["failure"]
	require.NotNil(t, example)
	assert.Empty(t, example.Ref.String())
	assert.Equal(t, map[string]any{"$ref": "literal example data"}, example.Value)
}

func TestOpenAPIPropertyNamesAreNotKeywords(t *testing.T) {
	for _, name := range []string{"default", "example", "enum", "x-field"} {
		t.Run(name, func(t *testing.T) {
			// These names are legal object properties, not literal schema values
			// or extensions. Their references must still be checked.
			doc := strings.ReplaceAll(`{
  "openapi": "3.0.3",
  "components": {"schemas": {"Result": {
    "type": "object", "properties": {"PROPERTY": {"$ref": "#/components/schemas/Missing"}}
  }}}
}`, "PROPERTY", name)
			_, err := parseOpenAPIRoutes([]byte(doc), "example.test", "v1")
			require.ErrorContains(t, err, "unresolved reference")
		})
	}
}

func TestManifestInlineOpenAPI(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(fmt.Sprintf("external=%t", external), func(t *testing.T) {
			source := `package test
manifest: {
 appName: "external"
 versions: v1: {
  kinds: [{kind: "Widget", scope: "Cluster", schema: spec: title: string,
   routes: "/details": GET: {name: "getDetails", response: {value: string}}}]
  openapi: {
   paths: "/query": {
    get: {operationId: "inlineGet", responses: "200": {$ref: "#/components/responses/Result"}}
    post: {operationId: "inlinePost", responses: "200": {description: "OK"}}
   }
   components: {
    responses: Result: {description: "Result", content: "application/json": schema: {$ref: "#/components/schemas/InlineResult"}}
    schemas: InlineResult: {type: "string"}
   }
  }
 }
}`
			files := externalTestFiles(source)
			if external {
				files["openapi.v1.yaml"] = &fstest.MapFile{Data: []byte(externalOpenAPI)}
			}
			c, err := LoadCue(files)
			require.NoError(t, err)
			parser, err := NewParser(c, true)
			require.NoError(t, err)
			manifest, err := parser.ParseManifest("manifest")
			require.NoError(t, err)
			for _, includeSchemas := range []bool{false, true} {
				generator := &jennies.ManifestGenerator{Encoder: json.Marshal, IncludeSchemas: includeSchemas, ManifestVersion: "v1alpha2"}
				generated, err := generator.Generate(manifest)
				require.NoError(t, err)
				var output struct {
					Spec v1alpha2.AppManifestSpec `json:"spec"`
				}
				require.NoError(t, json.Unmarshal(generated[0].Data, &output))
				data, err := output.Spec.ToManifestData()
				require.NoError(t, err)
				doc := data.Versions[0].OpenAPI
				require.NotNil(t, doc.Paths)
				require.Contains(t, doc.Paths, "/widgets/{name}/details")
				require.Contains(t, doc.Paths, "/query")
				query := doc.Paths["/query"]
				assert.Equal(t, "inlinePost", query.Post.OperationId)
				if external {
					assert.Equal(t, "externalQuery", query.Get.OperationId)
				} else {
					assert.Equal(t, "inlineGet", query.Get.OperationId)
					assert.Equal(t, "#/components/schemas/InlineResult", query.Get.Responses.StatusCodeResponses[200].Content["application/json"].Schema.Ref.String())
				}
				assert.Contains(t, doc.Components.Schemas, "InlineResult")
			}
		})
	}
}

func TestOpenAPISharedParameters(t *testing.T) {
	files := externalTestFiles(`package test
manifest: {
 appName: "external"
 versions: v1: {
  kinds: []
  openapi: paths: "/query": {
   parameters: [{name: "q", in: "query", schema: {type: "string"}}]
   get: {operationId: "inlineGet"}
   post: {operationId: "inlinePost"}
  }
 }
}`)
	files["openapi.v1.json"] = &fstest.MapFile{Data: []byte(`{
 "openapi": "3.0.3",
 "paths": {"/query": {
  "parameters": [{"$ref": "#/components/parameters/Limit"}],
  "get": {"operationId": "externalGet"},
  "put": {"operationId": "externalPut", "parameters": [
   {"name": "limit", "in": "query", "schema": {"type": "integer", "maximum": 10}},
   {"name": "limit", "in": "header", "schema": {"type": "string"}}
  ]}
 }},
 "components": {"parameters": {"Limit": {
  "name": "limit", "in": "query", "schema": {"type": "integer", "maximum": 100}
 }}}
}`)}
	c, err := LoadCue(files)
	require.NoError(t, err)
	parser, err := NewParser(c, false)
	require.NoError(t, err)
	manifest, err := parser.ParseManifest("manifest")
	require.NoError(t, err)
	generator := &jennies.ManifestGenerator{Encoder: json.Marshal, ManifestVersion: "v1alpha2"}
	generated, err := generator.Generate(manifest)
	require.NoError(t, err)
	var output struct {
		Spec v1alpha2.AppManifestSpec `json:"spec"`
	}
	require.NoError(t, json.Unmarshal(generated[0].Data, &output))
	data, err := output.Spec.ToManifestData()
	require.NoError(t, err)
	for _, route := range []spec3.PathProps{data.Versions[0].Routes.Cluster["/query"], data.Versions[0].OpenAPI.Paths["/query"]} {
		assert.Empty(t, route.Parameters)
		require.Len(t, route.Get.Parameters, 1)
		assert.Equal(t, "limit", route.Get.Parameters[0].Name)
		assert.Equal(t, float64(100), *route.Get.Parameters[0].Schema.Maximum)
		require.Len(t, route.Post.Parameters, 1)
		assert.Equal(t, "q", route.Post.Parameters[0].Name)
		require.Len(t, route.Put.Parameters, 2)
		assert.Equal(t, float64(10), *route.Put.Parameters[0].Schema.Maximum)
		assert.Equal(t, "header", route.Put.Parameters[1].In)
	}
}
