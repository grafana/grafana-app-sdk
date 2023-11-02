package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/kindsys"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type LineageGenerator struct {
}

func (*LineageGenerator) JennyName() string {
	return "CustomKindParser"
}

func (s *LineageGenerator) Generate(decl kindsys.Custom) (*codejen.File, error) {
	meta := decl.Def().Properties
	typeName := exportField(decl.Lineage().Name())
	md := templates.LineageMetadata{
		Package:        meta.MachineName,
		TypeName:       typeName,
		ObjectTypeName: "Object",
		CUEFile:        fmt.Sprintf("%s_lineage.cue", meta.MachineName),
		CUESelector:    meta.MachineName,
		Subresources:   make([]templates.SubresourceMetadata, 0),
	}
	for _, sr := range getSubresources(decl.Lineage().Latest().Underlying().LookupPath(cue.MakePath(cue.Hid("_#schema", "github.com/grafana/thema"))).Eval()) {
		md.Subresources = append(md.Subresources, templates.SubresourceMetadata{
			TypeName: sr.TypeName,
			JSONName: sr.FieldName,
		})
	}
	b := bytes.Buffer{}
	err := templates.WriteLineageGo(md, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile(fmt.Sprintf("%s/%s_lineage_gen.go", meta.MachineName, meta.MachineName), formatted, s), nil
}

type CUEGenerator struct {
}

func (*CUEGenerator) JennyName() string {
	return "CUEGenerator"
}

func (c *CUEGenerator) Generate(decl kindsys.Custom) (codejen.Files, error) {
	meta := decl.Def().Properties
	st := cueFmtState{}
	decl.Lineage().Underlying().Format(&st, 'v')
	contents := fmt.Sprintf("package %s\n\n%s", meta.MachineName, st.Bytes())
	// Can't control the name of the definition, (always _#def), so we replace it as a string.
	// TODO: maybe manip this in AST instead?
	contents = strings.Replace(contents, "_#def\n_#def:", meta.MachineName+":", 1)
	files := make(codejen.Files, 0)
	files = append(files, codejen.File{
		RelativePath: fmt.Sprintf("%s/%s_lineage.cue", meta.MachineName, meta.MachineName),
		Data:         []byte(contents),
		From:         []codejen.NamedJenny{c},
	})
	files = append(files, codejen.File{
		RelativePath: fmt.Sprintf("%s/cue.mod/module.cue", meta.MachineName),
		Data:         []byte(fmt.Sprintf("module: \"example.com/%s\"\n", meta.MachineName)),
		From:         []codejen.NamedJenny{c},
	})
	return files, nil
}

// cueFmtState wraps a bytes.Buffer with the extra methods required to implement fmt.State.
// it will return false when queried about any flags.
type cueFmtState struct {
	bytes.Buffer
}

// Width returns the value of the width option and whether it has been set. It will always return 0, false.
func (*cueFmtState) Width() (wid int, ok bool) {
	return 0, false
}

// Precision returns the value of the precision option and whether it has been set. It will always return 0, false.
func (*cueFmtState) Precision() (prec int, ok bool) {
	return 0, false
}

// Flag returns whether the specified flag has been set. It will always return false.
func (*cueFmtState) Flag(flag int) bool {
	return flag == '#' || flag == '+'
}

// exportField makes a field name exported
func exportField(field string) string {
	if len(field) > 0 {
		return strings.ToUpper(field[:1]) + field[1:]
	}
	return strings.ToUpper(field)
}

type subresourceInfo struct {
	TypeName  string
	FieldName string
	Comment   string
}

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

func getSubresources(schema cue.Value) []subresourceInfo {
	// TODO: do we really need this future-proofing for arbitrary extra subresources?
	subs := append(make([]subresourceInfo, 0), customKindSchemaSubresources...)
	i, err := schema.Fields()
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
			TypeName:  exportField(str),
		})
	}
	return subs
}
