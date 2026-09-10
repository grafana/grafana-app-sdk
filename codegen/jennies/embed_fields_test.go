package jennies

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/codegen"
)

func TestValidateEmbedFields(t *testing.T) {
	schema := cuecontext.New().CompileString(`spec: {
		title: string
		description?: string | null
		tags: [...string]
		fixedTags: [string, string]
		nullableTags: [...string] | null
		emptyTags: []
		panels: [...{ title: string }]
		count: int
		nullableCount: int | null
		ratio: number
		nullableRatio: number | null
		enabled: bool
		nullableEnabled: bool | null
		counts: [...int]
		fixedCounts: [int, int]
		nullableCounts: [...int] | null
		nullableFlags: [...bool] | null
		nullablePanels?: [...({ title: string, count: int } | null)] | null
		#Panel: { title: string, count: int }
		nullablePanel?: #Panel | null
		nullableListVariants: [...int] | [...bool] | null
		mixedScalar: string | int
		nullableMixedScalar: string | int | null
		mixedArray: [...(string | int)]
		nullableMixedArray: [...(string | int | null)] | null
		fixedMixedArray: [string, string | int]
		nullableStringElements: [...(string | null)] | null
		nullValue: null
		nullElements: [...null]
		nullableNullElements: [...null] | null
		fixedNullElements: [null, null]
		fixedStringAndNull: [string, null]
		stringWithNullTail: [string, ...null]
		stringOrStringArray: string | [...string]
		nullableStringOrStringArray: string | [...(string | null)] | null
		stringOrNullArray: string | [...null]
		stringOrIntegerArray: string | [...int]
		variant: { text: { body: string } } | { numeric: { body: int } }
	}`)
	require.NoError(t, schema.Err())

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "empty path", wantErr: true},
		{name: "string", path: "spec.title"},
		{name: "optional nullable string", path: "spec.description"},
		{name: "string array", path: "spec.tags"},
		{name: "fixed string array", path: "spec.fixedTags"},
		{name: "nullable string array", path: "spec.nullableTags"},
		{name: "empty string array", path: "spec.emptyTags"},
		{name: "string array projection", path: "spec.tags[*]"},
		{name: "nullable string array projection", path: "spec.nullableTags[*]"},
		{name: "object array projection to string", path: "spec.panels[*].title"},
		{name: "nullable object array projection to string", path: "spec.nullablePanels[*].title"},
		{name: "nullable object array projection to integer", path: "spec.nullablePanels[*].count", wantErr: true},
		{name: "nullable non-text list union projection", path: "spec.nullableListVariants[*]", wantErr: true},
		{name: "string under nullable parent", path: "spec.nullablePanel.title"},
		{name: "integer under nullable parent", path: "spec.nullablePanel.count", wantErr: true},
		{name: "integer", path: "spec.count", wantErr: true},
		{name: "nullable integer", path: "spec.nullableCount", wantErr: true},
		{name: "number", path: "spec.ratio", wantErr: true},
		{name: "nullable number", path: "spec.nullableRatio", wantErr: true},
		{name: "boolean", path: "spec.enabled", wantErr: true},
		{name: "nullable boolean", path: "spec.nullableEnabled", wantErr: true},
		{name: "object", path: "spec", wantErr: true},
		{name: "nullable object", path: "spec.nullablePanel", wantErr: true},
		{name: "object array", path: "spec.panels", wantErr: true},
		{name: "nullable object array", path: "spec.nullablePanels", wantErr: true},
		{name: "integer array", path: "spec.counts", wantErr: true},
		{name: "fixed integer array", path: "spec.fixedCounts", wantErr: true},
		{name: "nullable integer array", path: "spec.nullableCounts", wantErr: true},
		{name: "nullable boolean array", path: "spec.nullableFlags", wantErr: true},
		{name: "mixed scalar", path: "spec.mixedScalar", wantErr: true},
		{name: "nullable mixed scalar", path: "spec.nullableMixedScalar", wantErr: true},
		{name: "mixed array", path: "spec.mixedArray", wantErr: true},
		{name: "mixed array projection", path: "spec.mixedArray[*]", wantErr: true},
		{name: "nullable mixed array", path: "spec.nullableMixedArray", wantErr: true},
		{name: "fixed array with mixed element", path: "spec.fixedMixedArray", wantErr: true},
		{name: "nullable string elements", path: "spec.nullableStringElements"},
		{name: "null-only scalar", path: "spec.nullValue", wantErr: true},
		{name: "null-only array", path: "spec.nullElements", wantErr: true},
		{name: "nullable null-only array", path: "spec.nullableNullElements", wantErr: true},
		{name: "fixed null-only array", path: "spec.fixedNullElements", wantErr: true},
		{name: "null-only array projection", path: "spec.nullElements[*]", wantErr: true},
		{name: "fixed string array with null placeholder", path: "spec.fixedStringAndNull"},
		{name: "string array with null-only tail", path: "spec.stringWithNullTail"},
		{name: "string or string array", path: "spec.stringOrStringArray"},
		{name: "nullable string or string array", path: "spec.nullableStringOrStringArray"},
		{name: "string or null-only array", path: "spec.stringOrNullArray"},
		{name: "string or integer array", path: "spec.stringOrIntegerArray", wantErr: true},
		{name: "unresolved string in object variant", path: "spec.variant.text.body", wantErr: true},
		{name: "unresolved integer in object variant", path: "spec.variant.numeric.body", wantErr: true},
		{name: "missing path", path: "spec.missing", wantErr: true},
		{name: "missing projected leaf", path: "spec.panels[*].missing", wantErr: true},
		{name: "missing nullable projected leaf", path: "spec.nullablePanels[*].missing", wantErr: true},
		{name: "missing leaf under nullable parent", path: "spec.nullablePanel.missing", wantErr: true},
		{name: "projection on scalar", path: "spec.title[*]", wantErr: true},
		{name: "projection on nullable scalar", path: "spec.description[*]", wantErr: true},
		{name: "malformed path", path: "spec..title", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmbedFields(codegen.VersionedKind{
				Kind:   "Widget",
				Schema: schema,
				Embed: &codegen.KindEmbed{
					Fields: []codegen.EmbedField{{Name: "body", Path: tt.path}},
				},
			}, "v1")
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), `kind "Widget"`)
			assert.Contains(t, err.Error(), `version "v1"`)
			assert.Contains(t, err.Error(), `embed field "body"`)
		})
	}
}

func TestValidateEmbedFields_Unset(t *testing.T) {
	assert.NoError(t, validateEmbedFields(codegen.VersionedKind{Kind: "Widget"}, "v1"))
}

func TestValidateEmbedFields_EmptyPathWithoutSchema(t *testing.T) {
	err := validateEmbedFields(codegen.VersionedKind{
		Kind: "Widget",
		Embed: &codegen.KindEmbed{
			Fields: []codegen.EmbedField{{Name: "body"}},
		},
	}, "v1")
	require.ErrorContains(t, err, `embed field "body"`)
}

func TestValidateEmbedFields_PerVersionSchema(t *testing.T) {
	ctx := cuecontext.New()
	embed := &codegen.KindEmbed{
		Fields: []codegen.EmbedField{{Name: "summary", Path: "spec.summary"}},
	}
	for _, tt := range []struct {
		version string
		schema  string
		wantErr bool
	}{
		{version: "v1", schema: `spec: { title: string }`, wantErr: true},
		{version: "v2", schema: `spec: { title: string, summary: string }`},
	} {
		t.Run(tt.version, func(t *testing.T) {
			schema := ctx.CompileString(tt.schema)
			require.NoError(t, schema.Err())
			err := validateEmbedFields(codegen.VersionedKind{Kind: "Widget", Schema: schema, Embed: embed}, tt.version)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProcessKindVersion_RejectsInvalidEmbedField(t *testing.T) {
	schema := cuecontext.New().CompileString(`spec: { title: string }`)
	require.NoError(t, schema.Err())
	_, err := processKindVersion(codegen.VersionedKind{
		Kind:   "Widget",
		Scope:  "Namespaced",
		Schema: schema,
		Embed: &codegen.KindEmbed{
			Fields: []codegen.EmbedField{{Name: "body", Path: "spec.missing"}},
		},
	}, "v1", false)
	require.ErrorContains(t, err, `embed field "body"`)
}
