package jennies

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type ModelsFunctionsGenerator struct {
}

func (*ModelsFunctionsGenerator) JennyName() string {
	return "ModelsFunctionsGenerator"
}

func (s *ModelsFunctionsGenerator) Generate(kind codegen.Kind) (*codejen.File, error) {
	meta := kind.Properties()
	typeName := exportField(meta.Kind)
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
