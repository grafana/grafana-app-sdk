package jennies

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/codegen"
)

func TestGoTypeOverride(t *testing.T) {
	ctx := cuecontext.New()
	tests := []struct {
		name           string
		field          string
		wantImportPath string
		wantType       string
		wantOK         bool
	}{
		{
			name:           "quoted goType",
			field:          `status: {bar: string} @grafana_app_sdk(goType="github.com/org/repo/apis/common.Status")`,
			wantImportPath: "github.com/org/repo/apis/common",
			wantType:       "Status",
			wantOK:         true,
		},
		{
			name:           "import path containing dots",
			field:          `status: {bar: string} @grafana_app_sdk(goType="gopkg.in/foo.v2/common.Status")`,
			wantImportPath: "gopkg.in/foo.v2/common",
			wantType:       "Status",
			wantOK:         true,
		},
		{
			name:           "goType alongside other keys",
			field:          `status: {bar: string} @grafana_app_sdk(prefix=Foo,goType="example.com/pkg.MyStatus")`,
			wantImportPath: "example.com/pkg",
			wantType:       "MyStatus",
			wantOK:         true,
		},
		{
			name:           "import path without slashes",
			field:          `status: {bar: string} @grafana_app_sdk(goType="time.Time")`,
			wantImportPath: "time",
			wantType:       "Time",
			wantOK:         true,
		},
		{
			name:   "no attribute",
			field:  `status: {bar: string}`,
			wantOK: false,
		},
		{
			name:   "attribute without goType key",
			field:  `status: {bar: string} @grafana_app_sdk(prefix=Foo)`,
			wantOK: false,
		},
		{
			name:   "goType without a type name",
			field:  `status: {bar: string} @grafana_app_sdk(goType="github.com/org/repo/apis/common")`,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ctx.CompileString("{" + tt.field + "}").LookupPath(cue.MakePath(cue.Str("status")))
			require.NoError(t, v.Err())
			importPath, typeName, ok := goTypeOverride(v)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantImportPath, importPath)
				assert.Equal(t, tt.wantType, typeName)
			}
		})
	}
}

func TestRenderGoTypeAlias(t *testing.T) {
	got, err := renderGoTypeAlias("v1", "Status", "github.com/org/repo/apis/common", "Status")
	require.NoError(t, err)
	want := `// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1

import common "github.com/org/repo/apis/common"

// Status is a type alias to an externally-defined, shared type.
type Status = common.Status

// NewStatus creates a new Status object.
func NewStatus() *Status {
	return &Status{}
}
`
	assert.Equal(t, want, string(got))
}

// TestGoTypes_GoTypeAliasGeneration exercises the full generateFilesAtDepth path (including real CUE attribute
// reading) to confirm that a status field carrying a goType attribute produces an alias file rather than a
// cog-generated type, while a sibling spec field is still generated normally.
func TestGoTypes_GoTypeAliasGeneration(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`{
		spec: {foo: string}
		status: {bar: string} @grafana_app_sdk(goType="github.com/org/repo/apis/common.Status")
	}`)
	require.NoError(t, schema.Err())

	g := &GoTypes{Depth: 1, OpenAPINamer: func(info OpenAPINamerInfo) string { return info.TypeName }}
	files, err := g.generateFilesAtDepth(schema, schema.Path(), 0, goTypesGenerateFilesConfig{
		PackageName: "v1",
		KindName:    "Foo",
		MachineName: "foo",
	})
	require.NoError(t, err)

	byName := map[string]string{}
	for _, f := range files {
		byName[f.RelativePath] = string(f.Data)
	}

	// The status file should be an alias to the external type, with a matching constructor.
	status, ok := byName["foo_status_gen.go"]
	require.True(t, ok, "expected foo_status_gen.go to be generated, got files: %v", keys(byName))
	assert.Contains(t, status, `import common "github.com/org/repo/apis/common"`)
	assert.Contains(t, status, "type Status = common.Status")
	assert.Contains(t, status, "func NewStatus() *Status {")
	// It should NOT contain a generated struct for the status.
	assert.NotContains(t, status, "type Status struct")

	// The spec file should still be generated as a normal go type.
	spec, ok := byName["foo_spec_gen.go"]
	require.True(t, ok, "expected foo_spec_gen.go to be generated")
	assert.Contains(t, spec, "type Spec struct")
}

// TestGoTypes_GoTypeAliasGeneration_Prefixed confirms the alias type name honors the configured NamePrefix
// (used when kinds are grouped by kind, so subresource types are prefixed with the kind name).
func TestGoTypes_GoTypeAliasGeneration_Prefixed(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`{
		status: {bar: string} @grafana_app_sdk(goType="github.com/org/repo/apis/common.Status")
	}`)
	require.NoError(t, schema.Err())

	g := &GoTypes{Depth: 1}
	files, err := g.generateFilesAtDepth(schema, schema.Path(), 0, goTypesGenerateFilesConfig{
		PackageName: "v1",
		KindName:    "Foo",
		MachineName: "foo",
		NamePrefix:  "Foo",
	})
	require.NoError(t, err)
	require.Len(t, files, 1)
	data := string(files[0].Data)
	assert.Contains(t, data, "type FooStatus = common.Status")
	assert.Contains(t, data, "func NewFooStatus() *FooStatus {")
}

// TestResourceObject_AliasedSubresourceHasNoMethods confirms the resource-object jenny does not emit
// DeepCopy/DeepCopyInto methods on a subresource whose type is a goType alias (Go forbids methods on an
// alias to a non-local type), while still emitting them for a normally-generated subresource, and still
// delegating to the aliased type's DeepCopyInto from the object's own DeepCopyInto.
func TestResourceObject_AliasedSubresourceHasNoMethods(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(`{
		spec: {foo: string}
		status: {bar: string} @grafana_app_sdk(goType="github.com/org/repo/apis/common.Status")
		other: {baz: string}
	}`)
	require.NoError(t, schema.Err())

	r := &ResourceObjectGenerator{}
	data, err := r.generateObjectFile(codegen.VersionedKind{Kind: "Widget", Schema: schema}, "v1", "")
	require.NoError(t, err)
	out := string(data)

	// No methods defined on the aliased Status type.
	assert.NotContains(t, out, "func (s *Status) DeepCopy()")
	assert.NotContains(t, out, "func(s *Status) DeepCopy()")
	assert.NotContains(t, out, "func (s *Status) DeepCopyInto(")
	assert.NotContains(t, out, "func(s *Status) DeepCopyInto(")
	// But the non-aliased subresource and spec still get their methods.
	assert.Contains(t, out, "func (s *Other) DeepCopyInto(dst *Other)")
	assert.Contains(t, out, "func (s *Spec) DeepCopyInto(dst *Spec)")
	// And the object's own DeepCopyInto still delegates to the aliased type's method.
	assert.Contains(t, out, "o.Status.DeepCopyInto(&dst.Status)")
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
