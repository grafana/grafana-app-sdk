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

// TypeScriptTypes is a one-to-many jenny that generates one or more TypeScript types for a kind.
// Each type is a specific version of the kind where codegen.frontend is true.
// If GenerateOnlyCurrent is true, then all other versions of the kind will be ignored and only
// the kind.Propertoes().Current version will be used for TypeScript type generation
// (this will impact the generated file path).
type TypeScriptTypes struct {
	// GenerateOnlyCurrent should be set to true if you only want to generate code for the kind.Properties().Current version.
	// This will affect the package and path(s) of the generated file(s).
	GenerateOnlyCurrent bool
}

var _ codejen.OneToMany[codegen.Kind] = &TypeScriptTypes{}

func (TypeScriptTypes) JennyName() string {
	return "TypeScriptTypes"
}

func (j TypeScriptTypes) Generate(kind codegen.Kind) (codejen.Files, error) {
	cfg := cuetsy.Config{
		ImportMapper: cuetsy.IgnoreImportMapper,
		Export:       true,
	}

	if j.GenerateOnlyCurrent {
		ver := kind.Version(kind.Properties().Current)
		if ver == nil {
			return nil, fmt.Errorf("version '%s' of kind '%s' does not exist", kind.Properties().Current, kind.Name())
		}
		if !ver.Codegen.Frontend {
			return nil, nil
		}

		bytes, err := generateTypescriptBytes(ver, kind.Name(), cfg)
		if err != nil {
			return nil, fmt.Errorf("error generating TypeScript for kind '%s', version '%s': %w", kind.Name(), ver.Version, err)
		}
		return codejen.Files{codejen.File{
			RelativePath: fmt.Sprintf("%s_types.gen.ts", kind.Properties().MachineName),
			Data:         bytes,
			From:         []codejen.NamedJenny{j},
		}}, nil
	}

	files := make(codejen.Files, 0)
	// For each version, check if we need to codegen
	for _, v := range kind.Versions() {
		if !v.Codegen.Frontend {
			continue
		}

		bytes, err := generateTypescriptBytes(&v, kind.Name(), cfg)
		if err != nil {
			return nil, fmt.Errorf("error generating TypeScript for kind '%s', version '%s': %w", kind.Name(), v.Version, err)
		}
		files = append(files, codejen.File{
			RelativePath: fmt.Sprintf("%s/%s/types.gen.ts", kind.Properties().MachineName, v.Version),
			Data:         bytes,
			From:         []codejen.NamedJenny{j},
		})
	}
	return files, nil
}

func generateTypescriptBytes(v *codegen.KindVersion, name string, cfg cuetsy.Config) ([]byte, error) {
	tf, err := cuetsy.GenerateAST(v.Schema, cfg)
	if err != nil {
		return nil, err
	}
	as := cuetsy.TypeInterface
	top, err := cuetsy.GenerateSingleAST(name, v.Schema.Eval(), as)
	if err != nil {
		return nil, fmt.Errorf("generating TS for schema root failed: %w", err)
	}
	tf.Nodes = append(tf.Nodes, top.T)
	if top.D != nil {
		tf.Nodes = append(tf.Nodes, top.D)
	}
	// post-process fix on the generated TS
	fixed := strings.ReplaceAll(tf.String(), "Array<string>", "string[]")
	return []byte(fixed), nil
}

func (j TypeScriptTypes) Generate1(decl kindsys.Custom) (*codejen.File, error) {
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
