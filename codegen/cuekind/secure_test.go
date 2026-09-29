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
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
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
	assert.False(t, secure.AdditionalProperties.Allows)
	assert.Nil(t, secure.AdditionalProperties.Schema)
	assert.Len(t, secure.Properties["apiKey"].OneOf, 3)
	for _, tc := range []struct {
		name  string
		value map[string]any
		valid bool
	}{
		{"declared key", map[string]any{"apiKey": map[string]any{"create": "secret"}}, true},
		{"undeclared key", map[string]any{"other": map[string]any{"create": "secret"}}, false},
		{"mixed keys", map[string]any{"apiKey": map[string]any{"name": "existing"}, "other": map[string]any{"remove": true}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := validate.NewSchemaValidator(&secure, nil, "", strfmt.Default).Validate(tc.value)
			assert.Equal(t, tc.valid, result.IsValid(), "%v", result.Errors)
		})
	}

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
	assert.NotContains(t, crd.Schema["openAPIV3Schema"].(map[string]any)["properties"], "secure")
	kindWithoutSecure := kind
	kindWithoutSecure.SecureValues = nil
	crdWithoutSecure, err := jennies.KindVersionToCRDSpecVersion(kind.Schema, kindWithoutSecure, "v1", true)
	require.NoError(t, err)
	assert.Equal(t, crdWithoutSecure, crd)

	convertedCRD, err := converted.Schema.AsCRDOpenAPI3("Connection")
	require.NoError(t, err)
	encoded, err := json.Marshal(convertedCRD)
	require.NoError(t, err)
	var wireSchema apiextv1.JSONSchemaProps
	require.NoError(t, json.Unmarshal(encoded, &wireSchema))
	secureSchema := wireSchema.Properties["secure"]
	assert.Nil(t, secureSchema.AdditionalProperties)
	require.Len(t, secureSchema.Properties, 2)
	secureValue := secureSchema.Properties["apiKey"]
	assert.Equal(t, "API key from the service", secureValue.Description)
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
	structural, err := structuralschema.NewStructural(&validationSchema)
	require.NoError(t, err)
	value := map[string]any{"secure": map[string]any{
		"apiKey":     map[string]any{"name": "existing"},
		"undeclared": map[string]any{"name": "other"},
	}}
	pruning.Prune(value, structural, true)
	assert.Equal(t, map[string]any{"apiKey": map[string]any{"name": "existing"}}, value["secure"])

	// Check that Go manifest generation retains declarations as well as schemas.
	goManifest := &jennies.ManifestGoGenerator{Package: "manifestdata", ProjectRepo: "example.org/test", CodegenPath: "generated", GroupByKind: true, IncludeSchemas: true, SkipImportsProcess: true}
	manifestFiles, err := goManifest.Generate(manifest)
	require.NoError(t, err)
	require.Len(t, manifestFiles, 1)
	assert.Contains(t, string(manifestFiles[0].Data), "SecureValues: []app.ManifestVersionKindSecureValue")
	assert.Contains(t, string(manifestFiles[0].Data), `Key: "apiKey"`)

}

func TestWildcardSecureValuesGeneration(t *testing.T) {
	for _, tc := range []struct {
		name, declarations string
		named              bool
	}{
		{"wildcard only", `[{key: "*", description: "Any credential"}]`, false},
		{"wildcard first", `[{key: "*", description: "Any credential"}, {key: "apiKey", description: "Specific credential"}]`, true},
		{"wildcard last", `[{key: "apiKey", description: "Specific credential"}, {key: "*", description: "Any credential"}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testingCue(t)
			c.Root = c.Root.Context().CompileString(`manifest: {
    appName: "secure-example"
    versions: v1: kinds: [{
     kind: "Connection"
     schema: spec: endpoint: string
     secure: ` + tc.declarations + `
    }]
   }`)
			parser, err := NewParser(c, false)
			require.NoError(t, err)
			manifest, err := parser.ParseManifest("manifest")
			require.NoError(t, err)
			kind := manifest.Versions()[0].Kinds()[0]
			generator := &jennies.ManifestGenerator{ManifestVersion: jennies.VersionV1Alpha2, IncludeSchemas: true, Encoder: json.Marshal}
			files, err := generator.Generate(manifest)
			require.NoError(t, err)
			require.Len(t, files, 1)
			var document struct {
				Spec v1alpha2.AppManifestSpec `json:"spec"`
			}
			require.NoError(t, json.Unmarshal(files[0].Data, &document))
			data, err := document.Spec.ToManifestData()
			require.NoError(t, err)
			converted := data.Versions[0].Kinds[0]
			assert.Equal(t, kind.SecureValues, converted.SecureValues)
			definitions, err := converted.Schema.AsKubeOpenAPI(schema.GroupVersionKind{Group: data.Group, Version: "v1", Kind: "Connection"}, spec.MustCreateRef, "example", converted.SecureValues...)
			require.NoError(t, err)
			secure := definitions["example.Connection"].Schema.Properties["secure"]
			require.NotNil(t, secure.AdditionalProperties)
			assert.True(t, secure.AdditionalProperties.Allows)
			require.NotNil(t, secure.AdditionalProperties.Schema)
			assert.Equal(t, "Any credential", secure.AdditionalProperties.Schema.Description)
			assert.NotContains(t, secure.Properties, "*")
			if tc.named {
				assert.Equal(t, "Specific credential", secure.Properties["apiKey"].Description)
			} else {
				assert.Empty(t, secure.Properties)
			}

			crd, err := jennies.KindVersionToCRDSpecVersion(kind.Schema, kind, "v1", true)
			require.NoError(t, err)
			assert.NotContains(t, crd.Subresources, "secure")
			assert.NotContains(t, crd.Schema["openAPIV3Schema"].(map[string]any)["properties"], "secure")
			kindWithoutSecure := kind
			kindWithoutSecure.SecureValues = nil
			crdWithoutSecure, err := jennies.KindVersionToCRDSpecVersion(kind.Schema, kindWithoutSecure, "v1", true)
			require.NoError(t, err)
			assert.Equal(t, crdWithoutSecure, crd)
			convertedCRD, err := converted.Schema.AsCRDOpenAPI3("Connection")
			require.NoError(t, err)
			schemas := map[string]any{"served OpenAPI": secure}
			encoded, err := json.Marshal(convertedCRD)
			require.NoError(t, err)
			var wire apiextv1.JSONSchemaProps
			require.NoError(t, json.Unmarshal(encoded, &wire))
			secureCRD := wire.Properties["secure"]
			assert.Empty(t, secureCRD.Properties)
			require.NotNil(t, secureCRD.AdditionalProperties)
			require.NotNil(t, secureCRD.AdditionalProperties.Schema)
			assert.Equal(t, "Any credential", secureCRD.AdditionalProperties.Schema.Description)
			var internal apiext.JSONSchemaProps
			require.NoError(t, apiextv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(&secureCRD, &internal, nil))
			root := &apiext.JSONSchemaProps{Type: "object", Properties: map[string]apiext.JSONSchemaProps{"secure": internal}}
			structural, err := structuralschema.NewStructural(root)
			require.NoError(t, err)
			require.Empty(t, structuralschema.ValidateStructural(nil, structural))
			value := map[string]any{"secure": map[string]any{"arbitrary": map[string]any{"name": "existing"}}}
			pruning.Prune(value, structural, true)
			assert.Equal(t, map[string]any{"arbitrary": map[string]any{"name": "existing"}}, value["secure"])
			schemas["converted CRD"] = secureCRD

			for name, source := range schemas {
				t.Run(name, func(t *testing.T) {
					encoded, err := json.Marshal(source)
					require.NoError(t, err)
					var sch spec.Schema
					require.NoError(t, json.Unmarshal(encoded, &sch))
					for _, value := range []map[string]any{
						{"create": "secret", "description": "new credential"}, {"name": "existing"}, {"remove": true},
					} {
						result := validate.NewSchemaValidator(&sch, nil, "", strfmt.Default).Validate(map[string]any{"arbitrary": value})
						assert.True(t, result.IsValid(), "%v", result.Errors)
					}
					for _, value := range []any{"plaintext", map[string]any{"create": "secret", "name": "existing"}, map[string]any{"name": "existing", "description": "not allowed"}} {
						result := validate.NewSchemaValidator(&sch, nil, "", strfmt.Default).Validate(map[string]any{"arbitrary": value})
						assert.False(t, result.IsValid())
					}
				})
			}
		})
	}
}
