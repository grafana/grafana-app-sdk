package jennies

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

func TestAlignAliasComments(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "documented union alias",
			source: `package test
// Supported elements.
// +k8s:openapi-gen=true
type Element = PanelOrLibrary
// +k8s:openapi-gen=true
type PanelOrLibrary struct{}
`,
			want: "Supported elements.\n+k8s:openapi-gen=true\n",
		},
		{
			name: "alias chain and distinct descriptions",
			source: `package test
// First description.
// +k8s:openapi-gen=true
type First = Second
// Second description.
// +k8s:openapi-gen=true
type Second = Target
// +k8s:openapi-gen=true
type Target struct{}
`,
			want: "First description.\n\nSecond description.\n+k8s:openapi-gen=true\n",
		},
		{
			name: "target before undocumented alias",
			source: `package test
// Description with a repeated line:
// repeat
// repeat
type Target struct{}
type Alias = Target
`,
			want: "Description with a repeated line:\nrepeat\nrepeat\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := alignAliasComments([]byte(tc.source))
			require.NoError(t, err)
			got, err = format.Source(got)
			require.NoError(t, err)
			file, err := parser.ParseFile(token.NewFileSet(), "", got, parser.ParseComments)
			require.NoError(t, err)
			for _, decl := range file.Decls {
				require.Equal(t, tc.want, decl.(*ast.GenDecl).Doc.Text())
			}
			again, err := alignAliasComments(got)
			require.NoError(t, err)
			again, err = format.Source(again)
			require.NoError(t, err)
			require.Equal(t, string(got), string(again), "normalization must be idempotent")
		})
	}
}

func TestAlignAliasCommentsLeavesUnrelatedTypesUnchanged(t *testing.T) {
	source := []byte(`package test
import "time"
// Duration documentation.
type Duration = time.Duration
// String documentation.
type String = string
// Defined type documentation.
type Defined Target
// Target documentation.
type Target struct{}
// +k8s:openapi-gen=false
type Disabled = Enabled
// +k8s:openapi-gen=true
type Enabled struct{}
`)
	got, err := alignAliasComments(source)
	require.NoError(t, err)
	require.Equal(t, source, got)
}

func TestGoTypesFromCUEAliasComments(t *testing.T) {
	value := cuecontext.New().CompileString(`
Spec: {elements: [...Element]}
// Supported elements.
Element: Panel | LibraryPanel
Panel: {kind: "panel", title: string}
LibraryPanel: {kind: "library", uid: string}
`).LookupPath(cue.ParsePath("Spec"))
	require.NoError(t, value.Err())
	for _, kubernetes := range []bool{false, true} {
		data, err := GoTypesFromCUE(value, CUEGoConfig{
			PackageName: "test", Name: "Spec", AddKubernetesOpenAPIGenComment: kubernetes,
		}, 0, func(name string) string { return "test." + name })
		require.NoError(t, err)
		file, err := parser.ParseFile(token.NewFileSet(), "", data, parser.ParseComments)
		require.NoError(t, err)
		docs := make(map[string]string)
		aliases := make(map[string]string)
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			spec := gen.Specs[0].(*ast.TypeSpec)
			docs[spec.Name.Name] = gen.Doc.Text()
			if spec.Assign.IsValid() {
				if ref, ok := spec.Type.(*ast.Ident); ok {
					aliases[spec.Name.Name] = ref.Name
				}
			}
		}
		require.NotEmpty(t, aliases)
		for alias, target := range aliases {
			if kubernetes {
				require.Equal(t, docs[alias], docs[target])
				require.Contains(t, docs[target], "Supported elements.")
				require.Contains(t, docs[target], "+k8s:openapi-gen=true")
			} else {
				require.NotEqual(t, docs[alias], docs[target])
			}
		}
	}
}
