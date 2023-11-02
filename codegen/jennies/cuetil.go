package jennies

import (
	"fmt"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/openapi"
	cueyaml "cuelang.org/go/pkg/encoding/yaml"
)

type CUEOpenAPIConfig struct {
	Name             string
	Version          string
	ExpandReferences bool
	NameFunc         func(cue.Value, cue.Path) string
}

func CUEValueToOAPIYAML(val cue.Value, cfg CUEOpenAPIConfig) ([]byte, error) {
	f, err := CUEValueToOpenAPI(val, cfg)
	if err != nil {
		return nil, err
	}
	str, err := cueyaml.Marshal(val.Context().BuildFile(f))
	if err != nil {
		return nil, err
	}
	return []byte(str), nil
}

func CUEValueToOpenAPI(val cue.Value, cfg CUEOpenAPIConfig) (*ast.File, error) {
	defpath := cue.MakePath(cue.Def(cfg.Name))
	v := val.Context().CompileString(fmt.Sprintf("#%s: _", cfg.Name))
	defsch := v.FillPath(defpath, val)
	f, err := openapi.Generate(defsch.Eval(), &openapi.Config{
		ExpandReferences: cfg.ExpandReferences,
		NameFunc: func(val cue.Value, path cue.Path) string {
			tpath := TrimPathPrefix(path, defpath)
			if path.String() == "" || tpath.String() == defpath.String() {
				return cfg.Name
			}
			switch val {
			case defsch:
				return cfg.Name
			}
			if cfg.NameFunc != nil {
				return cfg.NameFunc(val, tpath)
			}
			return strings.Trim(tpath.String(), "?#")
		}})
	if err != nil {
		return nil, err
	}
	decls := getSchemas(f)

	return &ast.File{
		Decls: []ast.Decl{
			ast.NewStruct(
				"openapi", ast.NewString("3.0.0"),
				"info", ast.NewStruct(
					"title", ast.NewString(cfg.Name),
					"version", ast.NewString(cfg.Version),
				),
				"paths", ast.NewStruct(),
				"components", ast.NewStruct(
					"schemas", &ast.StructLit{Elts: decls},
				),
			),
		},
	}, nil
}

func getSchemas(f *ast.File) []ast.Decl {
	compos := orp(getASTFieldByLabel(f, "components"))
	schemas := orp(getASTFieldByLabel(compos.Value, "schemas"))
	return schemas.Value.(*ast.StructLit).Elts
}

func orp[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

func getASTFieldByLabel(n ast.Node, label string) (*ast.Field, error) {
	var d []ast.Decl
	switch x := n.(type) {
	case *ast.File:
		d = x.Decls
	case *ast.StructLit:
		d = x.Elts
	default:
		return nil, fmt.Errorf("not an *ast.File or *ast.StructLit")
	}

	for _, el := range d {
		if isFieldWithLabel(el, label) {
			return el.(*ast.Field), nil
		}
	}

	return nil, fmt.Errorf("no field with label %q", label)
}

func isFieldWithLabel(n ast.Node, label string) bool {
	if x, is := n.(*ast.Field); is {
		if l, is := x.Label.(*ast.BasicLit); is {
			return strEq(l, label)
		}
		if l, is := x.Label.(*ast.Ident); is {
			return identStrEq(l, label)
		}
	}
	return false
}

func strEq(lit *ast.BasicLit, str string) bool {
	if lit.Kind != token.STRING {
		return false
	}
	ls, _ := strconv.Unquote(lit.Value)
	return str == ls || str == lit.Value
}

func identStrEq(id *ast.Ident, str string) bool {
	if str == id.Name {
		return true
	}
	ls, _ := strconv.Unquote(id.Name)
	return str == ls
}

func TrimPathPrefix(path, prefix cue.Path) cue.Path {
	sels, psels := path.Selectors(), prefix.Selectors()
	if len(sels) == 1 {
		return path
	}
	var i int
	for ; i < len(psels) && i < len(sels); i++ {
		if !SelEq(psels[i], sels[i]) {
			break
		}
	}
	return cue.MakePath(sels[i:]...)
}

// SelEq indicates whether two selectors are equivalent. Selectors are equivalent if
// they are either exactly equal, or if they are equal ignoring path optionality.
func SelEq(s1, s2 cue.Selector) bool {
	return s1 == s2 || s1.Optional() == s2.Optional()
}
