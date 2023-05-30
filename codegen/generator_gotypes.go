package codegen

import (
	"fmt"
	"go/format"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"github.com/dave/dst"
	"github.com/dave/dst/dstutil"
	"github.com/grafana/codejen"
	"github.com/grafana/kindsys"
	"github.com/grafana/thema"
	"github.com/grafana/thema/encoding/gocode"
	"github.com/grafana/thema/encoding/openapi"
)

// customKindSchemaSubresources is a list of the valid top-level fields in a Custom Kind's schema
var customKindSchemaSubresources = []subresourceInfo{
	{
		TypeName:  "Spec",
		FieldName: "spec",
		Comment:   "Spec contains the spec contents for the schema",
	},
	{
		TypeName:  "Status",
		FieldName: "status",
		Comment:   "Status contains status subresource information",
	}, {
		TypeName:  "Metadata",
		FieldName: "metadata",
		Comment:   "Metadata contains all the common and kind-specific metadata for the object",
	},
}

type modelGoTypesGenerator struct{}

func (*modelGoTypesGenerator) JennyName() string {
	return "GoTypesJenny"
}

func (ag *modelGoTypesGenerator) Generate(decl kindsys.Custom) (*codejen.File, error) {
	meta := decl.Def().Properties
	sch := decl.Lineage().Latest()

	b, err := gocode.GenerateTypesOpenAPI(sch, &gocode.TypeConfigOpenAPI{
		// TODO will need to account for sanitizing e.g. dashes here at some point
		Config: &openapi.Config{
			Group:    false, // TODO: better
			RootName: typeNameFromKey(sch.Lineage().Name()),
		},
		PackageName: strings.ToLower(meta.MachineName),
		ApplyFuncs:  []dstutil.ApplyFunc{PrefixDropper(meta.Name)},
	})
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b)
	if err != nil {
		return nil, err
	}

	return codejen.NewFile(fmt.Sprintf("%s/%s_type_gen.go", meta.MachineName, meta.MachineName), formatted, ag), nil
}

type resourceGoTypesGenerator struct {
}

func (*resourceGoTypesGenerator) JennyName() string {
	return "GoResourceTypes"
}

func (g *resourceGoTypesGenerator) Generate(decl kindsys.Custom) (codejen.Files, error) {
	meta := decl.Def().Properties
	sch := decl.Lineage().Latest()

	files := make(codejen.Files, 0)
	// Make objects for each of "spec", "status", and "metadata"
	for _, sr := range getSubresources(decl.Lineage().Latest()) {
		b, err := gocode.GenerateTypesOpenAPI(sch, &gocode.TypeConfigOpenAPI{
			// TODO will need to account for sanitizing e.g. dashes here at some point
			Config: &openapi.Config{
				Group:    false, // TODO: better
				RootName: sr.TypeName,
				Subpath:  cue.MakePath(cue.Str(sr.FieldName)),
			},
			PackageName: strings.ToLower(meta.MachineName),
			ApplyFuncs:  []dstutil.ApplyFunc{PrefixDropper(meta.Name)},
		})
		if err != nil {
			return nil, err
		}

		formatted, err := format.Source(b)
		if err != nil {
			return nil, err
		}
		files = append(files, codejen.File{
			RelativePath: fmt.Sprintf("%s/%s_%s_gen.go", meta.MachineName, meta.MachineName, strings.ToLower(sr.FieldName)),
			Data:         formatted,
			From:         []codejen.NamedJenny{g},
		})
	}

	return files, nil
}

func typeNameFromKey(key string) string {
	if len(key) > 0 {
		return strings.ToUpper(key[:1]) + key[1:]
	}
	return strings.ToUpper(key)
}

type subresourceInfo struct {
	TypeName  string
	FieldName string
	Comment   string
}

func getSubresources(sch thema.Schema) []subresourceInfo {
	// TODO: do we really need this future-proofing for arbitrary extra subresources?
	subs := append(make([]subresourceInfo, 0), customKindSchemaSubresources...)
	i, err := sch.Underlying().LookupPath(cue.MakePath(cue.Str("schema"))).Fields()
	if err != nil {
		return nil
	}
	for i.Next() {
		str, _ := i.Value().Label()
		// TODO: grab docs from the field for comments
		exists := false
		for _, v := range subs {
			if v.FieldName == str {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		subs = append(subs, subresourceInfo{
			FieldName: str,
			TypeName:  typeNameFromKey(str),
		})
	}
	return subs
}

type prefixmod struct {
	prefix  string
	replace string
	rxp     *regexp.Regexp
	rxpsuff *regexp.Regexp
}

// PrefixDropper returns a dstutil.ApplyFunc that removes the provided prefix
// string when it appears as a leading sequence in type names, var names, and
// comments in a generated Go file.
func PrefixDropper(prefix string) dstutil.ApplyFunc {
	return (&prefixmod{
		prefix:  prefix,
		rxpsuff: regexp.MustCompile(fmt.Sprintf(`%s([a-zA-Z_]+)`, prefix)),
		rxp:     regexp.MustCompile(fmt.Sprintf(`%s([\s.,;-])`, prefix)),
	}).applyfunc
}

// PrefixReplacer returns a dstutil.ApplyFunc that removes the provided prefix
// string when it appears as a leading sequence in type names, var names, and
// comments in a generated Go file.
//
// When an exact match for prefix is found, the provided replace string
// is substituted.
func PrefixReplacer(prefix, replace string) dstutil.ApplyFunc {
	return (&prefixmod{
		prefix:  prefix,
		replace: replace,
		rxpsuff: regexp.MustCompile(fmt.Sprintf(`%s([a-zA-Z_]+)`, prefix)),
		rxp:     regexp.MustCompile(fmt.Sprintf(`%s([\s.,;-])`, prefix)),
	}).applyfunc
}

func depoint(e dst.Expr) dst.Expr {
	if star, is := e.(*dst.StarExpr); is {
		return star.X
	}
	return e
}

func (d prefixmod) applyfunc(c *dstutil.Cursor) bool {
	n := c.Node()

	switch x := n.(type) {
	case *dst.ValueSpec:
		d.handleExpr(x.Type)
		for _, id := range x.Names {
			d.do(id)
		}
	case *dst.TypeSpec:
		// Always do typespecs
		d.do(x.Name)
	case *dst.Field:
		// Don't rename struct fields. We just want to rename type declarations, and
		// field value specifications that reference those types.
		d.handleExpr(x.Type)
	case *dst.File:
		for _, def := range x.Decls {
			comments := def.Decorations().Start.All()
			def.Decorations().Start.Clear()
			// For any reason, sometimes it retrieves the comment duplicated 🤷
			commentMap := make(map[string]bool)
			for _, c := range comments {
				if _, ok := commentMap[c]; !ok {
					commentMap[c] = true
					def.Decorations().Start.Append(d.rxpsuff.ReplaceAllString(c, "$1"))
					if d.replace != "" {
						def.Decorations().Start.Append(d.rxp.ReplaceAllString(c, d.replace+"$1"))
					}
				}
			}
		}
	}
	return true
}

func (d prefixmod) handleExpr(e dst.Expr) {
	// Deref a StarExpr, if there is one
	expr := depoint(e)
	switch x := expr.(type) {
	case *dst.Ident:
		d.do(x)
	case *dst.ArrayType:
		if id, is := depoint(x.Elt).(*dst.Ident); is {
			d.do(id)
		}
	case *dst.MapType:
		if id, is := depoint(x.Key).(*dst.Ident); is {
			d.do(id)
		}
		if id, is := depoint(x.Value).(*dst.Ident); is {
			d.do(id)
		}
	}
}

func (d prefixmod) do(n *dst.Ident) {
	if n.Name != d.prefix {
		n.Name = strings.TrimPrefix(n.Name, d.prefix)
	} else if d.replace != "" {
		n.Name = d.replace
	}
}
