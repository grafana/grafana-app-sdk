package codegen

import (
	"strings"

	"github.com/grafana/codejen"
	"github.com/grafana/cuetsy"
	"github.com/grafana/grafana-app-sdk/kindsys"
	"github.com/grafana/thema/encoding/typescript"
)

// TSTypesJenny is a [OneToOne] that produces TypeScript types and
// defaults for a Thema schema.
//
// Thema's generic TS jenny will be able to replace this one once
// https://github.com/grafana/thema/issues/89 is complete.
type TSTypesJenny struct{}

var _ codejen.OneToOne[kindsys.Custom] = &TSTypesJenny{}

func (TSTypesJenny) JennyName() string {
	return "TSTypesJenny"
}

func (j TSTypesJenny) Generate(decl kindsys.Custom) (*codejen.File, error) {
	// TODO allow using name instead of machine name in thema generator
	f, err := typescript.GenerateTypes(decl.Lineage().Latest(), &typescript.TypeConfig{
		RootName: decl.Name(),
		Group:    false,
		CuetsyConfig: &cuetsy.Config{
			ImportMapper: cuetsy.IgnoreImportMapper,
			Export:       true,
		},
	})
	if err != nil {
		return nil, err
	}
	// post-process fix on the generated TS
	fixed := strings.ReplaceAll(f.String(), "Array<string>", "string[]")

	return codejen.NewFile(decl.Lineage().Name()+"_types.gen.ts", []byte(fixed), j), nil
}
