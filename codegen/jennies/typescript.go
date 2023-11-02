package jennies

import (
	"fmt"
	"strings"

	"github.com/grafana/codejen"
	"github.com/grafana/cuetsy"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/thema/encoding/typescript"

	"github.com/grafana/grafana-app-sdk/kindsys"
)

// TSTypesJenny is a [OneToOne] that produces TypeScript types and
// defaults for a Thema schema.
//
// Thema's generic TS jenny will be able to replace this one once
// https://github.com/grafana/thema/issues/89 is complete.
type TSTypesJenny struct{}

var _ codejen.OneToMany[codegen.Kind] = &TSTypesJenny{}

func (TSTypesJenny) JennyName() string {
	return "TSTypesJenny"
}

func (j TSTypesJenny) Generate(kind codegen.Kind) (codejen.Files, error) {
	cfg := cuetsy.Config{
		ImportMapper: cuetsy.IgnoreImportMapper,
		Export:       true,
	}
	files := make(codejen.Files, 0)

	// For each version, check if we need to codegen
	for _, v := range kind.Versions() {
		if !v.Codegen.Frontend {
			continue
		}

		tf, err := cuetsy.GenerateAST(v.Schema, cfg)
		if err != nil {
			return nil, err
		}
		as := cuetsy.TypeInterface
		top, err := cuetsy.GenerateSingleAST(kind.Properties().Kind, v.Schema, as)
		if err != nil {
			return nil, fmt.Errorf("generating TS for schema root failed: %w", err)
		}
		tf.Nodes = append(tf.Nodes, top.T)
		if top.D != nil {
			tf.Nodes = append(tf.Nodes, top.D)
		}
		// post-process fix on the generated TS
		fixed := strings.ReplaceAll(tf.String(), "Array<string>", "string[]")
		files = append(files, codejen.File{
			RelativePath: fmt.Sprintf("%s/%s/types.gen.ts", kind.Properties().Kind, v.Version),
			Data:         []byte(fixed),
			From:         []codejen.NamedJenny{j},
		})
	}
	return files, nil
}

func (j TSTypesJenny) Generate1(decl kindsys.Custom) (*codejen.File, error) {
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
