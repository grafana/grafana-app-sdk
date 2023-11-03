package jennies

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/codegen"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/resource"
)

type SchemaGenerator struct {
	// This flag exists for compatibility with thema codegen, which only generates code for the current/latest version of the kind
	OnlyUseCurrentVersion bool
}

func (*SchemaGenerator) JennyName() string {
	return "SchemaGenerator"
}

func (s *SchemaGenerator) Generate(kind codegen.Kind) (codejen.Files, error) {
	meta := kind.Properties()

	if meta.APIResource.Scope != string(resource.NamespacedScope) && meta.APIResource.Scope != string(resource.ClusterScope) {
		return nil, fmt.Errorf("scope '%s' is invalid, must be one of: '%s', '%s'",
			meta.APIResource.Scope, resource.ClusterScope, resource.NamespacedScope)
	}

	files := make(codejen.Files, 0)
	if s.OnlyUseCurrentVersion {
		b := bytes.Buffer{}
		err := templates.WriteSchema(templates.SchemaMetadata{
			Package: meta.MachineName,
			Group:   meta.APIResource.Group,
			Version: meta.Current,
			Kind:    meta.Kind,
			Plural:  meta.PluralMachineName,
			Scope:   meta.APIResource.Scope,
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
			RelativePath: fmt.Sprintf("%s/%s_schema_gen.go", meta.MachineName, meta.MachineName),
			From:         []codejen.NamedJenny{s},
		})
	} else {
		for _, ver := range kind.Versions() {
			b := bytes.Buffer{}
			err := templates.WriteSchema(templates.SchemaMetadata{
				Package: ToPackageName(ver.Version),
				Group:   meta.APIResource.Group,
				Version: ver.Version,
				Kind:    meta.Kind,
				Plural:  meta.PluralMachineName,
				Scope:   meta.APIResource.Scope,
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
				RelativePath: fmt.Sprintf("%s/%s/%s_schema_gen.go", meta.MachineName, ToPackageName(ver.Version), meta.MachineName),
				From:         []codejen.NamedJenny{s},
			})
		}
	}

	return files, nil
}
