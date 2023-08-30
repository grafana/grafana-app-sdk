package codegen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/kindsys"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type lineageGenerator struct {
}

func (*lineageGenerator) JennyName() string {
	return "CustomKindParser"
}

func (s *lineageGenerator) Generate(decl kindsys.Custom) (*codejen.File, error) {
	meta := decl.Def().Properties
	typeName := typeNameFromKey(decl.Lineage().Name())
	md := templates.LineageMetadata{
		Package:      meta.MachineName,
		TypeName:     typeName,
		CUEFile:      fmt.Sprintf("%s_lineage.cue", meta.MachineName),
		CUESelector:  meta.MachineName,
		Subresources: make([]templates.SubresourceMetadata, 0),
	}
	for _, sr := range getSubresources(decl.Lineage().Latest()) {
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

type cueGenerator struct {
}

func (*cueGenerator) JennyName() string {
	return "CUEGenerator"
}

func (c *cueGenerator) Generate(decl kindsys.Custom) (codejen.Files, error) {
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
