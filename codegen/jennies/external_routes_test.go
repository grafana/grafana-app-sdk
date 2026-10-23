package jennies

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
)

func TestMergeExternalRoutes(t *testing.T) {
	get := &spec3.Operation{OperationProps: spec3.OperationProps{OperationId: "getFromCue"}}
	post := &spec3.Operation{OperationProps: spec3.OperationProps{OperationId: "createFromCue"}}
	externalGet := &spec3.Operation{OperationProps: spec3.OperationProps{OperationId: "getFromOpenAPI"}}
	target := app.ManifestVersionRoutes{
		Cluster: map[string]spec3.PathProps{"/query": {Get: get, Post: post}},
		Schemas: map[string]spec.Schema{
			"Result":  {SchemaProps: spec.SchemaProps{Type: []string{"string"}}},
			"CueOnly": {SchemaProps: spec.SchemaProps{Type: []string{"boolean"}}},
		},
	}
	external := app.ManifestVersionRoutes{
		Cluster: map[string]spec3.PathProps{"/query": {Get: externalGet}},
		Schemas: map[string]spec.Schema{
			"Result": {SchemaProps: spec.SchemaProps{Type: []string{"integer"}}},
		},
	}
	require.NoError(t, mergeExternalRoutes(&target, external))
	assert.Equal(t, "getFromOpenAPI", target.Cluster["/query"].Get.OperationId)
	assert.Equal(t, "createFromCue", target.Cluster["/query"].Post.OperationId)
	assert.Equal(t, spec.StringOrArray{"integer"}, target.Schemas["Result"].Type)
	assert.Contains(t, target.Schemas, "CueOnly")
	assert.Equal(t, "getFromCue", get.OperationId)
	assert.Nil(t, external.Cluster["/query"].Post)
}

func TestMergeExternalVersionSubresources(t *testing.T) {
	for _, scope := range []string{"Namespaced", "Cluster"} {
		t.Run(scope, func(t *testing.T) {
			cueGet := &spec3.Operation{OperationProps: spec3.OperationProps{OperationId: "getCueDetails"}}
			cuePost := &spec3.Operation{OperationProps: spec3.OperationProps{OperationId: "createCueDetails"}}
			importedGet := &spec3.Operation{OperationProps: spec3.OperationProps{OperationId: "getExternalDetails"}}
			version := app.ManifestVersion{Kinds: []app.ManifestVersionKind{{
				Kind: "Foo", Plural: "foos", Scope: scope,
				Routes: map[string]spec3.PathProps{"/details": {Get: cueGet, Post: cuePost}},
			}}}
			source := app.ManifestVersionRoutes{
				Cluster:    map[string]spec3.PathProps{"/foos/{name}/details": {Get: importedGet}},
				Namespaced: map[string]spec3.PathProps{"/foos/{name}/details": {Get: importedGet}},
			}
			require.NoError(t, mergeExternalVersionRoutes(&version, source))
			assert.Equal(t, "getExternalDetails", version.Kinds[0].Routes["/details"].Get.OperationId)
			assert.Equal(t, "createCueDetails", version.Kinds[0].Routes["/details"].Post.OperationId)
			if scope == "Cluster" {
				assert.NotContains(t, version.Routes.Cluster, "/foos/{name}/details")
				assert.Contains(t, version.Routes.Namespaced, "/foos/{name}/details")
			} else {
				assert.NotContains(t, version.Routes.Namespaced, "/foos/{name}/details")
				assert.Contains(t, version.Routes.Cluster, "/foos/{name}/details")
			}
			// Extraction must not consume the source paths on a subsequent generation.
			assert.Contains(t, source.Cluster, "/foos/{name}/details")
			assert.Contains(t, source.Namespaced, "/foos/{name}/details")
		})
	}
}

func TestMergeExternalVersionPreservesKindSchemas(t *testing.T) {
	for _, name := range []string{"Widget", "spec"} {
		t.Run(name, func(t *testing.T) {
			schema, err := app.VersionSchemaFromMap(map[string]any{
				"Widget": map[string]any{"type": "object", "properties": map[string]any{
					"spec": map[string]any{"$ref": "#/components/schemas/spec"},
				}},
				"spec": map[string]any{"type": "object"},
			}, "Widget")
			require.NoError(t, err)
			version := app.ManifestVersion{Kinds: []app.ManifestVersionKind{{
				Kind: "Widget", Plural: "widgets", Scope: "Cluster", Schema: schema,
			}}}
			err = mergeExternalVersionRoutes(&version, app.ManifestVersionRoutes{
				Cluster: map[string]spec3.PathProps{"/widgets/{name}/details": {Get: &spec3.Operation{}}},
				Schemas: map[string]spec.Schema{name: {SchemaProps: spec.SchemaProps{Type: []string{"string"}}}},
			})
			require.ErrorContains(t, err, "conflicts with schema")
			assert.Same(t, schema, version.Kinds[0].Schema)
		})
	}
}

func TestMergeExternalVersionSharedKindSchemas(t *testing.T) {
	// "components" is a valid schema name; it must not be interpreted as
	// the wrapper of an OpenAPI document when rebuilding the kind schema.
	for _, name := range []string{"Shared", "components"} {
		t.Run(name, func(t *testing.T) {
			schemas := map[string]any{
				"Widget": map[string]any{"type": "object"},
				name:     map[string]any{"type": "string", "description": "Shared schema"},
			}
			kindSchema, err := app.VersionSchemaFromMap(map[string]any{
				"components": map[string]any{"schemas": schemas},
			}, "Widget")
			require.NoError(t, err)
			version := app.ManifestVersion{Kinds: []app.ManifestVersionKind{{
				Kind: "Widget", Plural: "widgets", Scope: "Cluster", Schema: kindSchema,
			}}}
			source := app.ManifestVersionRoutes{
				Cluster: map[string]spec3.PathProps{"/widgets/{name}/details": {Get: &spec3.Operation{}}},
				Schemas: map[string]spec.Schema{
					name:  {SchemaProps: spec.SchemaProps{Type: []string{"string"}, Description: "Shared schema"}},
					"New": {SchemaProps: spec.SchemaProps{Type: []string{"boolean"}}},
				},
			}
			// Repeated generation must also accept an existing schema stored as a struct.
			for range 2 {
				require.NoError(t, mergeExternalVersionRoutes(&version, source))
				assert.Contains(t, version.Kinds[0].Schema.AsOpenAPI3SchemasMap(), "New")
				assert.Equal(t, schemas["Widget"], version.Kinds[0].Schema.AsOpenAPI3SchemasMap()["Widget"])
			}
			assert.NotContains(t, kindSchema.AsOpenAPI3SchemasMap(), "New")
		})
	}
}
