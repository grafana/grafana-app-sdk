package jennies

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/codegen"
)

func TestValidateSearchFieldPath(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`{
		spec: {
			email: string
			panelRef?: {
				dashboardUID: string
				panelID: int64
			}
			expressions: [...{ datasourceUID?: string }]
			notificationSettings?: {
				simplifiedRouting?: { receiver: string }
			} | { namedRoutingTree?: { routingTree: string } }
		}
	}`)
	require.NoError(t, schema.Err())

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "plain nested path", path: "spec.email"},
		{name: "optional parent", path: "spec.panelRef.dashboardUID"},
		{name: "int leaf", path: "spec.panelRef.panelID"},
		{name: "array projection", path: "spec.expressions[*].datasourceUID"},
		{name: "missing top field", path: "spec.nope", wantErr: true},
		{name: "missing leaf", path: "spec.panelRef.nope", wantErr: true},
		{name: "projection on non-list", path: "spec.email[*].foo", wantErr: true},
		{name: "empty segment", path: "spec..email", wantErr: true},
		// Path crosses a disjunction into a field that exists in one variant: not
		// statically resolvable to a single variant, so it is allowed.
		{name: "disjunction crossing, field in a variant", path: "spec.notificationSettings.simplifiedRouting.receiver"},
		{name: "disjunction crossing, field in other variant", path: "spec.notificationSettings.namedRoutingTree.routingTree"},
		// Segment after a disjunction matches no variant: a typo, so it errors.
		{name: "disjunction typo", path: "spec.notificationSettings.typo", wantErr: true},
		// Typo nested inside a matching variant: the leading segment matches a
		// variant but the remainder does not, so it still errors.
		{name: "disjunction nested typo", path: "spec.notificationSettings.simplifiedRouting.typo", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := resolveSearchFieldPath(schema, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSearchFieldType(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`
import "time"

spec: {
	name: string
	count: int64
	ratio: float
	enabled: bool
	seen: string & time.Time
	tags: [...string]
}
`)
	require.NoError(t, schema.Err())

	tests := []struct {
		name     string
		path     string
		declared string
		wantErr  bool
		wantWarn bool
	}{
		{name: "string as string", path: "spec.name", declared: "string"},
		{name: "string as date (kept verbatim)", path: "spec.name", declared: "date"},
		{name: "string as boolean", path: "spec.name", declared: "boolean", wantErr: true},
		{name: "int as int64", path: "spec.count", declared: "int64"},
		{name: "int as double (widening)", path: "spec.count", declared: "double"},
		{name: "int as date", path: "spec.count", declared: "date", wantErr: true},
		{name: "int as string", path: "spec.count", declared: "string", wantErr: true},
		{name: "float as double", path: "spec.ratio", declared: "double"},
		{name: "float as int64 (rounded, warns)", path: "spec.ratio", declared: "int64", wantWarn: true},
		{name: "bool as boolean", path: "spec.enabled", declared: "boolean"},
		{name: "bool as string", path: "spec.enabled", declared: "string", wantErr: true},
		{name: "time as date", path: "spec.seen", declared: "date"},
		{name: "time as string (kept verbatim)", path: "spec.seen", declared: "string"},
		{name: "string list element", path: "spec.tags", declared: "string"},
		{name: "string list element as boolean", path: "spec.tags", declared: "boolean", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leaf, resolved, err := resolveSearchFieldPath(schema, tt.path)
			require.NoError(t, err)
			require.True(t, resolved)
			warning, err := searchFieldTypeCompatibility(leaf, tt.declared)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantWarn {
				assert.NotEmpty(t, warning)
			} else {
				assert.Empty(t, warning)
			}
		})
	}
}

func TestValidateSearchFields_PerVersionSchema(t *testing.T) {
	ctx := cuecontext.New()
	// The same field path is valid against v2's schema but absent from v1's.
	// validateSearchFields receives the version's own VersionedKind.Schema, so a
	// path that does not exist in that version must fail for that version only.
	v1Schema := ctx.CompileString(`{ spec: { name: string } }`)
	v2Schema := ctx.CompileString(`{ spec: { name: string, region: string } }`)
	require.NoError(t, v1Schema.Err())
	require.NoError(t, v2Schema.Err())

	searchFields := []codegen.SearchField{
		{Name: "region", Path: "spec.region", Type: "string", Capabilities: []string{"filter"}},
	}

	errV1 := validateSearchFields(codegen.VersionedKind{Kind: "Widget", Schema: v1Schema, SearchFields: searchFields}, "v1")
	assert.Error(t, errV1, "spec.region is absent from v1's schema")

	errV2 := validateSearchFields(codegen.VersionedKind{Kind: "Widget", Schema: v2Schema, SearchFields: searchFields}, "v2")
	assert.NoError(t, errV2, "spec.region exists in v2's schema")
}

func TestValidateSearchFieldCapabilities(t *testing.T) {
	tests := []struct {
		name    string
		field   codegen.SearchField
		wantErr bool
	}{
		{name: "string with text", field: codegen.SearchField{Type: "string", Capabilities: []string{"filter", "text", "sort"}}},
		{name: "int with filter only", field: codegen.SearchField{Type: "int64", Capabilities: []string{"filter", "retrieve"}}},
		{name: "int with sort", field: codegen.SearchField{Type: "int64", Capabilities: []string{"sort"}}},
		{name: "double with sort", field: codegen.SearchField{Type: "double", Capabilities: []string{"sort"}}},
		{name: "boolean with sort", field: codegen.SearchField{Type: "boolean", Capabilities: []string{"sort"}}},
		{name: "date with sort", field: codegen.SearchField{Type: "date", Capabilities: []string{"sort"}}, wantErr: true},
		{name: "boolean with facet", field: codegen.SearchField{Type: "boolean", Capabilities: []string{"facet"}}, wantErr: true},
		{name: "date with text", field: codegen.SearchField{Type: "date", Capabilities: []string{"text"}}, wantErr: true},
		{name: "double with partial", field: codegen.SearchField{Type: "double", Capabilities: []string{"partial"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSearchFieldCapabilities(tt.field)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProcessKindVersion_RejectsInvalidSearchFieldPath(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`{ spec: { email: string } }`)
	require.NoError(t, schema.Err())

	vk := codegen.VersionedKind{
		Kind:   "Widget",
		Scope:  "Namespaced",
		Schema: schema,
		SearchFields: []codegen.SearchField{
			{Name: "email", Path: "spec.nope", Type: "string", Capabilities: []string{"filter"}},
		},
	}
	_, err := processKindVersion(vk, "v1", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `search field "email"`)
}

func TestValidateSearchFields_ErrorMentionsKindAndField(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`{ spec: { email: string } }`)
	require.NoError(t, schema.Err())

	vk := codegen.VersionedKind{
		Kind:   "Widget",
		Schema: schema,
		SearchFields: []codegen.SearchField{
			{Name: "missing", Path: "spec.nope", Type: "string", Capabilities: []string{"filter"}},
		},
	}
	err := validateSearchFields(vk, "v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `kind "Widget"`)
	assert.Contains(t, err.Error(), `version "v1"`)
	assert.Contains(t, err.Error(), `search field "missing"`)
}
