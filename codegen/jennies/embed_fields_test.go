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
		nullablePanels: [...{ title: string }] | null
		nullablePanel: { title: string } | null
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
		{name: "object array projection to string", path: "spec.panels[*].title"},
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
		{name: "missing path", path: "spec.missing", wantErr: true},
		{name: "missing projected leaf", path: "spec.panels[*].missing", wantErr: true},
		{name: "projection on scalar", path: "spec.title[*]", wantErr: true},
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
