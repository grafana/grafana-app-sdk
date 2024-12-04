//nolint:dupl
package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/resource"
)

type CodecGenerator struct {
	// This flag exists for compatibility with thema codegen, which only generates code for the current/latest version of the kind
	OnlyUseCurrentVersion bool

	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	// When GroupByKind is false, the generated Codec type names are prefixed with the kind name,
	// i.e. FooJSONCodec for kind.Name()="Foo"
	GroupByKind bool
}

func (*CodecGenerator) JennyName() string {
	return "CodecGenerator"
}

// Generate creates one or more codec go files for the provided Kind
// nolint:dupl
func (c *CodecGenerator) Generate(kind codegen.Kind) (codejen.Files, error) {
	meta := kind.Properties()

	if meta.Scope != string(resource.NamespacedScope) && meta.Scope != string(resource.ClusterScope) {
		return nil, fmt.Errorf("scope '%s' is invalid, must be one of: '%s', '%s'",
			meta.Scope, resource.ClusterScope, resource.NamespacedScope)
	}

	prefix := ""
	if !c.GroupByKind {
		prefix = exportField(kind.Name())
	}

	files := make(codejen.Files, 0)
	if c.OnlyUseCurrentVersion {
		b := bytes.Buffer{}
		err := templates.WriteCodec(templates.SchemaMetadata{
			Package:    meta.MachineName,
			Group:      meta.Group,
			Version:    meta.Current,
			Kind:       meta.Kind,
			Plural:     meta.PluralMachineName,
			Scope:      meta.Scope,
			FuncPrefix: prefix,
		}, &b)
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		files = append(files, codejen.File{
			Data:         formatted,
			RelativePath: fmt.Sprintf("%s/%s_codec_gen.go", meta.MachineName, meta.MachineName),
			From:         []codejen.NamedJenny{c},
		})
	} else {
		for _, ver := range kind.Versions() {
			b := bytes.Buffer{}
			err := templates.WriteCodec(templates.SchemaMetadata{
				Package:    ToPackageName(ver.Version),
				Group:      meta.Group,
				Version:    ver.Version,
				Kind:       meta.Kind,
				Plural:     meta.PluralMachineName,
				Scope:      meta.Scope,
				FuncPrefix: prefix,
			}, &b)
			if err != nil {
				return nil, err
			}
			formatted, err := format.Source(b.Bytes())
			if err != nil {
				return nil, err
			}
			files = append(files, codejen.File{
				Data:         formatted,
				RelativePath: filepath.Join(GetGeneratedPath(c.GroupByKind, kind, ver.Version), fmt.Sprintf("%s_codec_gen.go", meta.MachineName)),
				From:         []codejen.NamedJenny{c},
			})
		}
	}

	return files, nil
}
