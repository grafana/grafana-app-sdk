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
			name: "inline: discriminator field (case 1)",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {routing: {type: "A", x: string} | {type: "B", y: string} | {type: "C", z: string}}}`),
				SelectableFields: []string{".spec.routing.type"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{
					Field: ".spec.routing.type", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec.routing",
						UnionOptional:  false,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "AType", ConstantValue: "A"},
							{VariantField: "BType", ConstantValue: "B"},
							{VariantField: "CType", ConstantValue: "C"},
						},
					},
				},
			},
		},
		{
			name: "inline: field in only one variant (case 2)",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {routing?: {type: "A", x: string} | {type: "B", y: string} | {type: "C", z: string}}}`),
				SelectableFields: []string{".spec.routing.x"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{
					Field: ".spec.routing.x", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec.routing",
						UnionOptional:  true,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "AType", FieldInVariant: "X"},
						},
					},
				},
			},
		},
		{
			name: "named definitions: field in only one variant (case 2)",
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
				{
					Field: ".spec.routing.x", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec.routing",
						UnionOptional:  true,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "A", FieldInVariant: "X"},
						},
					},
				},
			},
		},
		{
			name: "named definitions: discriminator field (case 1)",
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
				{
					Field: ".spec.routing.type", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec.routing",
						UnionOptional:  true,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "A", ConstantValue: "A"},
							{VariantField: "B", ConstantValue: "B"},
							{VariantField: "C", ConstantValue: "C"},
						},
					},
				},
			},
		},
		{
			name: "inline: field in two of three variants (case 3)",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {routing?: {type: "A", x: string} | {type: "B", x: string} | {type: "C", z: string}}}`),
				SelectableFields: []string{".spec.routing.x"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{
					Field: ".spec.routing.x", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec.routing",
						UnionOptional:  true,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "AType", FieldInVariant: "X"},
							{VariantField: "BType", FieldInVariant: "X"},
						},
					},
				},
			},
		},
		{
			name: "named definitions: field in two of three variants (case 3)",
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
				{
					Field: ".spec.routing.x", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec.routing",
						UnionOptional:  true,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "A", FieldInVariant: "X"},
							{VariantField: "B", FieldInVariant: "X"},
						},
					},
				},
			},
		},
		{
			name: "inline: field path crosses union parent",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {type: "A", spec: {name: string}} | {type: "B", spec: {name: string}}}`),
				SelectableFields: []string{".spec.spec.name"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{
					Field: ".spec.spec.name", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec",
						UnionOptional:  false,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "AType", FieldInVariant: "Spec.Name"},
							{VariantField: "BType", FieldInVariant: "Spec.Name"},
						},
					},
				},
			},
		},
		{
			name: "named definitions: field path crosses union parent",
			kind: codegen.VersionedKind{
				Schema: ctx.CompileString(`
					#A: {kind: "A", spec: {name: string}}
					#B: {kind: "B", spec: {name: string}}
					#Union: #A | #B
					{spec: #Union}
				`),
				SelectableFields: []string{".spec.spec.name"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{
					Field: ".spec.spec.name", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec",
						UnionOptional:  false,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "A", FieldInVariant: "Spec.Name"},
							{VariantField: "B", FieldInVariant: "Spec.Name"},
						},
					},
				},
			},
		},
		{
			name: "inline: disjunction with null variant on optional field",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {routing?: {type: "A", x: string} | {type: "B", y: string} | null}}`),
				SelectableFields: []string{".spec.routing.type"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{
					Field: ".spec.routing.type", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec.routing",
						UnionOptional:  true,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "AType", ConstantValue: "A"},
							{VariantField: "BType", ConstantValue: "B"},
						},
					},
				},
			},
		},
		{
			name: "named definitions: disjunction with null variant on optional field",
			kind: codegen.VersionedKind{
				Schema: ctx.CompileString(`
					#A: {type: "A", x: string}
					#B: {type: "B", y: string}
					#Union: #A | #B | null
					{spec: {routing?: #Union}}
				`),
				SelectableFields: []string{".spec.routing.x"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{
					Field: ".spec.routing.x", Optional: true, Type: "string",
					OptionalFieldsInPath: []string{},
					Union: &templates.UnionFieldAccess{
						UnionFieldPath: "spec.routing",
						UnionOptional:  true,
						Variants: []templates.UnionVariantAccess{
							{VariantField: "A", FieldInVariant: "X"},
						},
					},
				},
			},
		},
		{
			name: "disjunction with one type and null collapses into single value",
			kind: codegen.VersionedKind{
				Schema: ctx.CompileString(`
					#A: {type: "A", x: string}
					{spec: {routing?: #A | null}}
				`),
				SelectableFields: []string{".spec.routing.x"},
			},
			want: []templates.SchemaMetadataSelectableField{
				{
					Field:                ".spec.routing.x",
					Optional:             false,
					Type:                 "string",
					OptionalFieldsInPath: []string{"spec.routing"},
				},
			},
		},
		{
			name: "disjunction with null variant on required field is an error",
			kind: codegen.VersionedKind{
				Schema:           ctx.CompileString(`{spec: {routing: {type: "A", x: string} | {type: "B", y: string} | null}}`),
				SelectableFields: []string{".spec.routing.type"},
			},
			wantErr: true,
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
		{
			name: "union case 1: discriminator returns constant per variant",
			fields: []templates.SchemaMetadataSelectableField{{
				Field: ".spec.routing.type", Optional: true, Type: "string",
				OptionalFieldsInPath: []string{},
				Union: &templates.UnionFieldAccess{
					UnionFieldPath: "spec.routing",
					UnionOptional:  true,
					Variants: []templates.UnionVariantAccess{
						{VariantField: "A", ConstantValue: "A"},
						{VariantField: "B", ConstantValue: "B"},
						{VariantField: "C", ConstantValue: "C"},
					},
				},
			}},
			contains: []string{
				`if cast.Spec.Routing == nil`,
				`if cast.Spec.Routing.A != nil`,
				`return "A", nil`,
				`if cast.Spec.Routing.B != nil`,
				`return "B", nil`,
				`if cast.Spec.Routing.C != nil`,
				`return "C", nil`,
			},
		},
		{
			name: "union case 2: field in single variant — pointer check + access",
			fields: []templates.SchemaMetadataSelectableField{{
				Field: ".spec.routing.x", Optional: true, Type: "string",
				OptionalFieldsInPath: []string{},
				Union: &templates.UnionFieldAccess{
					UnionFieldPath: "spec.routing",
					UnionOptional:  true,
					Variants: []templates.UnionVariantAccess{
						{VariantField: "A", FieldInVariant: "X"},
					},
				},
			}},
			contains: []string{
				`if cast.Spec.Routing == nil`,
				`if cast.Spec.Routing.A != nil`,
				`return string(cast.Spec.Routing.A.X), nil`,
			},
		},
		{
			name: "union case 3: field in multiple variants — try each",
			fields: []templates.SchemaMetadataSelectableField{{
				Field: ".spec.routing.x", Optional: true, Type: "string",
				OptionalFieldsInPath: []string{},
				Union: &templates.UnionFieldAccess{
					UnionFieldPath: "spec.routing",
					UnionOptional:  true,
					Variants: []templates.UnionVariantAccess{
						{VariantField: "A", FieldInVariant: "X"},
						{VariantField: "B", FieldInVariant: "X"},
					},
				},
			}},
			contains: []string{
				`if cast.Spec.Routing == nil`,
				`if cast.Spec.Routing.A != nil`,
				`return string(cast.Spec.Routing.A.X), nil`,
				`if cast.Spec.Routing.B != nil`,
				`return string(cast.Spec.Routing.B.X), nil`,
			},
		},
		{
			name: "union case 2: optional int field in variant uses fmt.Sprintf and deref",
			fields: []templates.SchemaMetadataSelectableField{{
				Field: ".spec.routing.count", Optional: true, Type: "int",
				OptionalFieldsInPath: []string{},
				Union: &templates.UnionFieldAccess{
					UnionFieldPath: "spec.routing",
					UnionOptional:  false,
					Variants: []templates.UnionVariantAccess{
						{VariantField: "A", FieldInVariant: "Count", FieldInVariantOptional: true},
					},
				},
			}},
			contains: []string{
				`if cast.Spec.Routing.A != nil`,
				`if cast.Spec.Routing.A.Count != nil`,
				`fmt.Sprintf("%d", *cast.Spec.Routing.A.Count)`,
			},
		},
		{
			name: "union case 1: required union skips outer nil check",
			fields: []templates.SchemaMetadataSelectableField{{
				Field: ".spec.routing.type", Optional: true, Type: "string",
				OptionalFieldsInPath: []string{},
				Union: &templates.UnionFieldAccess{
					UnionFieldPath: "spec.routing",
					UnionOptional:  false,
					Variants: []templates.UnionVariantAccess{
						{VariantField: "A", ConstantValue: "A"},
					},
				},
			}},
			contains: []string{
				`if cast.Spec.Routing.A != nil`,
				`return "A", nil`,
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
