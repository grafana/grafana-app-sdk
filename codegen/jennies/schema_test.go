package jennies

import (
	"bytes"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

func TestGetSelectableFields(t *testing.T) {
	ctx := cuecontext.New()

	tests := []struct {
		name    string
		kind    codegen.VersionedKind
		want    []templates.SchemaMetadataSelectableField
		wantErr bool
	}{
		{
			name: "required string field",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {name: string}}`),
				SelectableFields: []string{".spec.name"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.name", Optional: false, Type: "string", OptionalFieldsInPath: []string{}},
			},
		},
		{
			name: "optional string field",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {name?: string}}`),
				SelectableFields: []string{".spec.name"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.name", Optional: true, Type: "string", OptionalFieldsInPath: []string{"spec.name"}},
			},
		},
		{
			// CUE does not support direct field lookup on disjunction values, so the variant
			// fallback is always used — meaning fields through a disjunction are always optional.
			name: "inline: field in all variants",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {routing: {type: "A", x: string} | {type: "B", y: string} | {type: "C", z: string}}}`),
				SelectableFields: []string{".spec.routing.type"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.routing.type", Optional: true, Type: "string", OptionalFieldsInPath: []string{}},
			},
		},
		{
			name: "inline: field in only one variant",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {routing?: {type: "A", x: string} | {type: "B", y: string} | {type: "C", z: string}}}`),
				SelectableFields: []string{".spec.routing.x"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.routing.x", Optional: true, Type: "string", OptionalFieldsInPath: []string{"spec.routing"}},
			},
		},
		{
			// Named CUE definitions (#Foo | #Bar) rather than inline struct literals
			name: "named definitions: field in only some variants",
			kind: codegen.VersionedKind{
				Schema: ctx.CompileString(`
					#A: {type: "A", x: string}
					#B: {type: "B", y: string}
					#C: {type: "C", z: string}
					#Union: #A | #B | #C
					{spec: {routing?: #Union}}
				`),
				SelectableFields: []string{".spec.routing.x"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.routing.x", Optional: true, Type: "string", OptionalFieldsInPath: []string{"spec.routing"}},
			},
		},
		{
			// Named CUE definitions (#Foo | #Bar) rather than inline struct literals
			name: "named definitions: field in all variants",
			kind: codegen.VersionedKind{
				Schema: ctx.CompileString(`
					#A: {type: "A", x: string}
					#B: {type: "B", y: string}
					#C: {type: "C", z: string}
					#Union: #A | #B | #C
					{spec: {routing?: #Union}}
				`),
				SelectableFields: []string{".spec.routing.type"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.routing.type", Optional: true, Type: "string", OptionalFieldsInPath: []string{"spec.routing"}},
			},
		},
		{
			name: "inline: field in two of three variants",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {routing?: {type: "A", x: string} | {type: "B", x: string} | {type: "C", z: string}}}`),
				SelectableFields: []string{".spec.routing.x"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.routing.x", Optional: true, Type: "string", OptionalFieldsInPath: []string{"spec.routing"}},
			},
		},
		{
			name: "named definitions: field in two of three variants",
			kind: codegen.VersionedKind{
				Schema: ctx.CompileString(`
					#A: {type: "A", x: string}
					#B: {type: "B", x: string}
					#C: {type: "C", z: string}
					#Union: #A | #B | #C
					{spec: {routing?: #Union}}
				`),
				SelectableFields: []string{".spec.routing.x"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.routing.x", Optional: true, Type: "string", OptionalFieldsInPath: []string{"spec.routing"}},
			},
		},
		{
			name: "invalid path — field does not exist",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {name: string}}`),
				SelectableFields: []string{".spec.missing"},
			},
			wantErr: true,
		},
		{
			name: "invalid path — too short",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {name: string}}`),
				SelectableFields: []string{".spec"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&SchemaGenerator{}).getSelectableFields(&tt.kind)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetCUEValueKindString(t *testing.T) {
	ctx := cuecontext.New()

	tests := []struct {
		name    string
		cue     string
		want    string
		wantErr bool
	}{
		{
			name: "plain string",
			cue:  `"hello"`,
			want: "string",
		},
		{
			name: "string type",
			cue:  `string`,
			want: "string",
		},
		{
			name: "regex-constrained string",
			cue:  `string & =~"^[a-z]+"`,
			want: "string",
		},
		{
			name: "enum of strings",
			cue:  `"foo" | "bar" | "baz"`,
			want: "string",
		},
		{
			name: "plain bool",
			cue:  `bool`,
			want: "bool",
		},
		{
			name: "bool literal",
			cue:  `true | false`,
			want: "bool",
		},
		{
			name: "plain int",
			cue:  `int`,
			want: "int",
		},
		{
			name: "int with min constraint",
			cue:  `int & >=0`,
			want: "int",
		},
		{
			name: "int64",
			cue:  `int64`,
			want: "int",
		},
		{
			name:    "unsupported float",
			cue:     `float`,
			wantErr: true,
		},
		{
			name:    "unsupported struct",
			cue:     `{field: string}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ctx.CompileString(tt.cue)
			require.NoError(t, v.Err())

			got, err := getCUEValueKindString(v)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestSchemaTemplateSelectableFieldCast verifies that the schema template emits string(cast.Field)
// rather than a bare cast.Field for selectable string fields, so that named wrapper types
// (e.g. type MyType string) compile correctly.
func TestSchemaTemplateSelectableFieldCast(t *testing.T) {
	base := templates.SchemaMetadata{
		Package: "v1",
		Group:   "test.grafana.app",
		Version: "v1",
		Kind:    "MyKind",
		Plural:  "mykinds",
		Scope:   "Namespaced",
	}

	tests := []struct {
		name     string
		fields   []templates.SchemaMetadataSelectableField
		contains []string
	}{
		{
			name: "required string field uses string() cast",
			fields: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.name", Optional: false, Type: "string", OptionalFieldsInPath: []string{}},
			},
			contains: []string{`string(cast.Spec.Name)`},
		},
		{
			name: "optional string field uses string() cast with dereference",
			fields: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.name", Optional: true, Type: "string", OptionalFieldsInPath: []string{}},
			},
			contains: []string{`string(*cast.Spec.Name)`},
		},
		{
			name: "required int field uses fmt.Sprintf",
			fields: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.count", Optional: false, Type: "int", OptionalFieldsInPath: []string{}},
			},
			contains: []string{`fmt.Sprintf("%d", cast.Spec.Count)`},
		},
		{
			name: "optional int field uses fmt.Sprintf with dereference",
			fields: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.count", Optional: true, Type: "int", OptionalFieldsInPath: []string{}},
			},
			contains: []string{`fmt.Sprintf("%d", *cast.Spec.Count)`},
		},
		{
			name: "optional field with nil guard emits nil check",
			fields: []templates.SchemaMetadataSelectableField{
				{Field: ".spec.nested.name", Optional: true, Type: "string", OptionalFieldsInPath: []string{"spec.nested"}},
			},
			contains: []string{
				`cast.Spec.Nested == nil`,
				`string(*cast.Spec.Nested.Name)`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := base
			meta.SelectableFields = tt.fields

			var buf bytes.Buffer
			err := templates.WriteSchema(meta, &buf)
			require.NoError(t, err)

			output := buf.String()
			for _, want := range tt.contains {
				assert.Contains(t, output, want)
			}
		})
	}
}
