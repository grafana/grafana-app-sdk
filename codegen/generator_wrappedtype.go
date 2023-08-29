package codegen

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/kindsys"
)

type modelsFunctionsGenerator struct {
}

func (*modelsFunctionsGenerator) JennyName() string {
	return "ModelsFunctionsGenerator"
}

func (s *modelsFunctionsGenerator) Generate(decl kindsys.Custom) (*codejen.File, error) {
	meta := decl.Def().Properties
	typeName := typeNameFromKey(decl.Lineage().Name())
	md := templates.WrappedTypeMetadata{
		Package:     meta.MachineName,
		TypeName:    typeName,
		CUEFile:     fmt.Sprintf("%s_lineage.cue", meta.MachineName),
		CUESelector: meta.MachineName,
	}
	b := bytes.Buffer{}
	err := templates.WriteWrappedType(md, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile(fmt.Sprintf("%s/%s_marshal_gen.go", meta.MachineName, meta.MachineName), formatted, s), nil
}
