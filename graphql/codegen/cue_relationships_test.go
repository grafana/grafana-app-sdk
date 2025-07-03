package codegen

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCUERelationshipParser_ParseSingleRelationship tests parsing a single relationship from CUE
func TestCUERelationshipParser_ParseSingleRelationship(t *testing.T) {
	// Create CUE context and compile test schema
	ctx := cuecontext.New()
	schema := ctx.CompileString(`
#PlaylistKind: {
	kind: "Playlist"
	
	_relationships: {
		"spec.items.dashboard": {
			target: {
				kind: "Dashboard"
				group: "dashboard.grafana.app"
				version: "v1alpha1"
			}
			resolver: {
				sourceField: "value"
				condition: "type == 'dashboard_by_uid'"
				targetQuery: "dashboard"
				targetArgs: {
					namespace: "default"
					name: "{source.value}"
				}
			}
		}
	}
}
`)

	// Create test kind
	testKind := &testKind{
		name: "Playlist",
		props: codegen.KindProperties{
			Kind:    "Playlist",
			Current: "v1alpha1",
		},
		versions: []codegen.KindVersion{
			{
				Version: "v1alpha1",
				Schema:  schema.LookupPath(cue.ParsePath("#PlaylistKind")),
			},
		},
	}

	// Parse relationships
	parser := NewCUERelationshipParser([]codegen.Kind{testKind})
	relationships, err := parser.ParseRelationships()

	// Verify results
	require.NoError(t, err)
	require.Len(t, relationships, 1)

	rel := relationships[0]
	assert.Equal(t, "Playlist", rel.SourceType)
	assert.Equal(t, "dashboard", rel.SourceField)
	assert.Equal(t, "Dashboard", rel.TargetType)
	assert.Equal(t, "dashboard", rel.TargetService)
	assert.Equal(t, "dashboard", rel.TargetQuery)
	assert.Equal(t, "default", rel.TargetArguments["namespace"])
	assert.Equal(t, "{source.value}", rel.TargetArguments["name"])
	assert.NotNil(t, rel.Transform)
}

// TestCUERelationshipParser_ParseMultipleRelationships tests parsing multiple relationships from CUE
func TestCUERelationshipParser_ParseMultipleRelationships(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`
#DashboardKind: {
	kind: "Dashboard"
	
	_relationships: {
		"playlists": {
			target: {
				kind: "Playlist"
				group: "playlist.grafana.app"
				version: "v1alpha1"
			}
			resolver: {
				sourceField: "metadata.uid"
				targetQuery: "playlists"
				targetArgs: {
					namespace: "{source.metadata.namespace}"
				}
				cardinality: "many"
			}
		}
		"investigations": {
			target: {
				kind: "Investigation"
				group: "investigations.grafana.app"
				version: "v1alpha1"
			}
			resolver: {
				sourceField: "metadata.uid"
				condition: "relatedDashboardUID != null"
				targetQuery: "investigations"
				targetArgs: {
					namespace: "{source.metadata.namespace}"
					filter: "spec.relatedDashboardUID == '{source.metadata.uid}'"
				}
				cardinality: "many"
			}
		}
	}
}
`)

	testKind := &testKind{
		name: "Dashboard",
		props: codegen.KindProperties{
			Kind:    "Dashboard",
			Current: "v1alpha1",
		},
		versions: []codegen.KindVersion{
			{
				Version: "v1alpha1",
				Schema:  schema.LookupPath(cue.ParsePath("#DashboardKind")),
			},
		},
	}

	parser := NewCUERelationshipParser([]codegen.Kind{testKind})
	relationships, err := parser.ParseRelationships()

	require.NoError(t, err)
	require.Len(t, relationships, 2)

	// Check first relationship (playlists)
	playlistRel := findRelationshipByField(relationships, "playlists")
	require.NotNil(t, playlistRel)
	assert.Equal(t, "Dashboard", playlistRel.SourceType)
	assert.Equal(t, "playlists", playlistRel.SourceField)
	assert.Equal(t, "Playlist", playlistRel.TargetType)
	assert.Equal(t, "playlist", playlistRel.TargetService)

	// Check second relationship (investigations)
	investigationRel := findRelationshipByField(relationships, "investigations")
	require.NotNil(t, investigationRel)
	assert.Equal(t, "Dashboard", investigationRel.SourceType)
	assert.Equal(t, "investigations", investigationRel.SourceField)
	assert.Equal(t, "Investigation", investigationRel.TargetType)
	assert.Equal(t, "investigations", investigationRel.TargetService)
	assert.NotNil(t, investigationRel.Transform)
}

// TestCUERelationshipParser_MissingRelationshipsField tests handling of missing _relationships field
func TestCUERelationshipParser_MissingRelationshipsField(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`
#SimpleKind: {
	kind: "Simple"
	// No _relationships field
}
`)

	testKind := &testKind{
		name: "Simple",
		props: codegen.KindProperties{
			Kind:    "Simple",
			Current: "v1alpha1",
		},
		versions: []codegen.KindVersion{
			{
				Version: "v1alpha1",
				Schema:  schema.LookupPath(cue.ParsePath("#SimpleKind")),
			},
		},
	}

	parser := NewCUERelationshipParser([]codegen.Kind{testKind})
	relationships, err := parser.ParseRelationships()

	require.NoError(t, err)
	assert.Len(t, relationships, 0)
}

// TestCUERelationshipParser_ConditionalRelationships tests parsing conditional relationships
func TestCUERelationshipParser_ConditionalRelationships(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`
#PlaylistKind: {
	kind: "Playlist"
	
	_relationships: {
		"spec.items.dashboard": {
			target: {
				kind: "Dashboard"
				group: "dashboard.grafana.app"
				version: "v1alpha1"
			}
			resolver: {
				sourceField: "value"
				condition: "type == 'dashboard_by_uid'"
				targetQuery: "dashboard"
				targetArgs: {
					namespace: "default"
					name: "{source.value}"
				}
			}
		}
	}
}
`)

	testKind := &testKind{
		name: "Playlist",
		props: codegen.KindProperties{
			Kind:    "Playlist",
			Current: "v1alpha1",
		},
		versions: []codegen.KindVersion{
			{
				Version: "v1alpha1",
				Schema:  schema.LookupPath(cue.ParsePath("#PlaylistKind")),
			},
		},
	}

	parser := NewCUERelationshipParser([]codegen.Kind{testKind})
	relationships, err := parser.ParseRelationships()

	require.NoError(t, err)
	require.Len(t, relationships, 1)

	rel := relationships[0]
	require.NotNil(t, rel.Transform)

	// Test condition evaluation
	testData := map[string]interface{}{
		"type":  "dashboard_by_uid",
		"value": "test-dashboard-uid",
	}
	result := rel.Transform(testData)
	assert.NotNil(t, result)
	assert.Equal(t, testData, result)

	// Test condition not met
	testDataNotMet := map[string]interface{}{
		"type":  "dashboard_by_tag",
		"value": "test-tag",
	}
	result = rel.Transform(testDataNotMet)
	assert.Nil(t, result)
}

// TestCUERelationshipParser_ValidationError tests error handling for malformed CUE
func TestCUERelationshipParser_ValidationError(t *testing.T) {
	tests := []struct {
		name          string
		schema        string
		expectedError string
	}{
		{
			name: "missing target",
			schema: `
#InvalidKind: {
	kind: "Invalid"
	_relationships: {
		"field": {
			resolver: {
				sourceField: "value"
				targetQuery: "query"
			}
		}
	}
}`,
			expectedError: "relationship missing 'target' field",
		},
		{
			name: "missing target kind",
			schema: `
#InvalidKind: {
	kind: "Invalid"
	_relationships: {
		"field": {
			target: {
				group: "group.grafana.app"
			}
			resolver: {
				sourceField: "value"
				targetQuery: "query"
			}
		}
	}
}`,
			expectedError: "relationship target missing 'kind'",
		},
		{
			name: "missing resolver",
			schema: `
#InvalidKind: {
	kind: "Invalid"
	_relationships: {
		"field": {
			target: {
				kind: "Target"
				group: "group.grafana.app"
			}
		}
	}
}`,
			expectedError: "relationship missing 'resolver' field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := cuecontext.New()
			schema := ctx.CompileString(tt.schema)

			testKind := &testKind{
				name: "Invalid",
				props: codegen.KindProperties{
					Kind:    "Invalid",
					Current: "v1alpha1",
				},
				versions: []codegen.KindVersion{
					{
						Version: "v1alpha1",
						Schema:  schema.LookupPath(cue.ParsePath("#InvalidKind")),
					},
				},
			}

			parser := NewCUERelationshipParser([]codegen.Kind{testKind})
			_, err := parser.ParseRelationships()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

// TestGetRelationshipFromCUE tests the utility function for direct CUE value access
func TestGetRelationshipFromCUE(t *testing.T) {
	ctx := cuecontext.New()
	relationshipDef := ctx.CompileString(`{
	target: {
		kind: "Dashboard"
		group: "dashboard.grafana.app"
		version: "v1alpha1"
	}
	resolver: {
		sourceField: "value"
		targetQuery: "dashboard"
		targetArgs: {
			namespace: "default"
			name: "{source.value}"
		}
	}
}`)

	config, err := GetRelationshipFromCUE(relationshipDef, "spec.items.dashboard")

	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "TestKind", config.SourceType)
	assert.Equal(t, "dashboard", config.SourceField)
	assert.Equal(t, "Dashboard", config.TargetType)
	assert.Equal(t, "dashboard", config.TargetService)
}

// TestValidateRelationshipDefinition tests the validation function
func TestValidateRelationshipDefinition(t *testing.T) {
	tests := []struct {
		name          string
		definition    string
		shouldError   bool
		expectedError string
	}{
		{
			name: "valid definition",
			definition: `{
			target: {
				kind: "Dashboard"
				group: "dashboard.grafana.app"
			}
			resolver: {
				sourceField: "value"
				targetQuery: "dashboard"
			}
		}`,
			shouldError: false,
		},
		{
			name: "missing target",
			definition: `{
			resolver: {
				sourceField: "value"
				targetQuery: "dashboard"
			}
		}`,
			shouldError:   true,
			expectedError: "target field is required",
		},
		{
			name: "missing target kind",
			definition: `{
			target: {
				group: "dashboard.grafana.app"
			}
			resolver: {
				sourceField: "value"
				targetQuery: "dashboard"
			}
		}`,
			shouldError:   true,
			expectedError: "kind is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := cuecontext.New()
			def := ctx.CompileString(tt.definition)

			err := ValidateRelationshipDefinition(def)

			if tt.shouldError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper types and functions for testing

type testKind struct {
	name     string
	props    codegen.KindProperties
	versions []codegen.KindVersion
}

func (t *testKind) Name() string {
	return t.name
}

func (t *testKind) Properties() codegen.KindProperties {
	return t.props
}

func (t *testKind) Versions() []codegen.KindVersion {
	return t.versions
}

func (t *testKind) Version(version string) *codegen.KindVersion {
	for i, v := range t.versions {
		if v.Version == version {
			return &t.versions[i]
		}
	}
	return nil
}

func findRelationshipByField(relationships []MeshRelationshipConfig, field string) *MeshRelationshipConfig {
	for _, rel := range relationships {
		if rel.SourceField == field {
			return &rel
		}
	}
	return nil
}
