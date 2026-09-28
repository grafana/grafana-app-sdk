package cuekind

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	crdvalidation "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"k8s.io/kube-openapi/pkg/validation/strfmt"
	"k8s.io/kube-openapi/pkg/validation/validate"

	v1alpha2 "github.com/grafana/grafana-app-sdk/app/appmanifest/v1alpha2"
	"github.com/grafana/grafana-app-sdk/codegen/jennies"
)

func TestSecureValuesGeneration(t *testing.T) {
	c := testingCue(t)
	c.Root = c.Root.Context().CompileString(`manifest: {
  appName: "secure-example"
  versions: v1: kinds: [{
   kind: "Connection"
   schema: spec: endpoint: string
   secure: [{key: "apiKey", description: "API key from the service"}, {key: "token"}]
  }]
 }`)
	parser, err := NewParser(c, false)
	require.NoError(t, err)
	manifest, err := parser.ParseManifest("manifest")
	require.NoError(t, err)
	kind := manifest.Versions()[0].Kinds()[0]
	require.Len(t, kind.SecureValues, 2)
	generator := &jennies.ManifestGenerator{ManifestVersion: jennies.VersionV1Alpha2, IncludeSchemas: true, Encoder: json.Marshal}
	files, err := generator.Generate(manifest)
	require.NoError(t, err)
	require.Len(t, files, 1)
	var document struct {
		Spec v1alpha2.AppManifestSpec `json:"spec"`
	}
	require.NoError(t, json.Unmarshal(files[0].Data, &document))
	require.Len(t, document.Spec.Versions[0].Kinds[0].Secure, 2)
	data, err := document.Spec.ToManifestData()
	require.NoError(t, err)
	converted := data.Versions[0].Kinds[0]
	assert.Equal(t, kind.SecureValues, converted.SecureValues)
	assert.NotContains(t, converted.Subresources(), "secure")
	definitions, err := converted.Schema.AsKubeOpenAPI(schema.GroupVersionKind{Group: data.Group, Version: "v1", Kind: "Connection"}, spec.MustCreateRef, "example", converted.SecureValues...)
	require.NoError(t, err)
	secure := definitions["example.Connection"].Schema.Properties["secure"]
	assert.Equal(t, "API key from the service", secure.Properties["apiKey"].Description)
	require.NotNil(t, secure.AdditionalProperties)
	require.NotNil(t, secure.AdditionalProperties.Schema)
	assert.Len(t, secure.AdditionalProperties.Schema.OneOf, 3)

	goFiles, err := ResourceGenerator("example.org/test", "generated", false).Generate(manifest)
	require.NoError(t, err)
	found := false
	for _, file := range goFiles {
		if file.RelativePath == "connection/v1/connection_codec_gen.go" {
			assert.Contains(t, string(file.Data), "resource.NewJSONCodec().Write(writer, from)")
		}
		if file.RelativePath == "connection/v1/connection_object_gen.go" {
			found = true
			assert.Contains(t, string(file.Data), "Secure resource.InlineSecureValues")
			assert.Contains(t, string(file.Data), "GetSecureValues()")
			assert.Contains(t, string(file.Data), "resource.CopySecureValues(o.Secure)")
			assert.NotContains(t, string(file.Data), `"secure": o.Secure`)
		}
	}
	assert.True(t, found)
	tsFiles, err := TypeScriptResourceGenerator().Generate(manifest)
	require.NoError(t, err)
	found = false
	for _, file := range tsFiles {
		if file.RelativePath == "connection/v1/connection_object_gen.ts" {
			found = true
			assert.Contains(t, string(file.Data), "secure?: Record<string, InlineSecureValue>")
		}
	}
	assert.True(t, found)
	crd, err := jennies.KindVersionToCRDSpecVersion(kind.Schema, kind, "v1", true)
	require.NoError(t, err)
	assert.NotContains(t, crd.Subresources, "secure")
	assert.Contains(t, crd.Schema["openAPIV3Schema"].(map[string]any)["properties"], "secure")

	for _, source := range []any{crd.Schema["openAPIV3Schema"], func() any {
		schema, err := converted.Schema.AsCRDOpenAPI3("Connection")
		require.NoError(t, err)
		return schema
	}()} {
		encoded, err := json.Marshal(source)
		require.NoError(t, err)
		var wireSchema apiextv1.JSONSchemaProps
		require.NoError(t, json.Unmarshal(encoded, &wireSchema))
		secureValue := wireSchema.Properties["secure"].AdditionalProperties.Schema
		require.NotNil(t, secureValue.Not)
		assert.Equal(t, []string{"description"}, secureValue.Not.Required)
		require.NotNil(t, secureValue.Not.Not)
		assert.Equal(t, []string{"create"}, secureValue.Not.Not.Required)
		// A structurally valid schema must also accept valid secure operations.
		valueJSON, err := json.Marshal(secureValue)
		require.NoError(t, err)
		var valueSchema spec.Schema
		require.NoError(t, json.Unmarshal(valueJSON, &valueSchema))
		for _, tc := range []struct {
			value map[string]any
			valid bool
		}{
			{map[string]any{"create": "secret", "description": "from service"}, true},
			{map[string]any{"name": "existing"}, true},
			{map[string]any{"remove": true}, true},
			{map[string]any{"create": "secret", "name": "existing"}, false},
			{map[string]any{"name": "existing", "description": "not allowed"}, false},
			{map[string]any{"remove": true, "description": "not allowed"}, false},
		} {
			result := validate.NewSchemaValidator(&valueSchema, nil, "", strfmt.Default).Validate(tc.value)
			assert.Equal(t, tc.valid, result.IsValid(), "value %v: %v", tc.value, result.Errors)
		}
		var validationSchema apiext.JSONSchemaProps
		require.NoError(t, apiextv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(&wireSchema, &validationSchema, nil))
		// Validate the new secure property independently of existing spec generation.
		validationSchema = apiext.JSONSchemaProps{Type: "object", Properties: map[string]apiext.JSONSchemaProps{"secure": validationSchema.Properties["secure"]}}
		definition := &apiext.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "connections.example.grafana.app"},
			Spec: apiext.CustomResourceDefinitionSpec{
				Group: "example.grafana.app", Scope: apiext.NamespaceScoped,
				Names:      apiext.CustomResourceDefinitionNames{Plural: "connections", Singular: "connection", Kind: "Connection", ListKind: "ConnectionList"},
				Versions:   []apiext.CustomResourceDefinitionVersion{{Name: "v1", Served: true, Storage: true}},
				Validation: &apiext.CustomResourceValidation{OpenAPIV3Schema: &validationSchema},
			},
			Status: apiext.CustomResourceDefinitionStatus{StoredVersions: []string{"v1"}},
		}
		assert.Empty(t, crdvalidation.ValidateCustomResourceDefinition(context.Background(), definition))
	}

	// Check that Go manifest generation retains declarations as well as schemas.
	goManifest := &jennies.ManifestGoGenerator{Package: "manifestdata", ProjectRepo: "example.org/test", CodegenPath: "generated", GroupByKind: true, IncludeSchemas: true, SkipImportsProcess: true}
	manifestFiles, err := goManifest.Generate(manifest)
	require.NoError(t, err)
	require.Len(t, manifestFiles, 1)
	assert.Contains(t, string(manifestFiles[0].Data), "SecureValues: []app.ManifestVersionKindSecureValue")
	assert.Contains(t, string(manifestFiles[0].Data), `Key: "apiKey"`)

}
