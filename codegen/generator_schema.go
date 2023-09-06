package codegen

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/kindsys"
	"github.com/grafana/grafana-app-sdk/resource"
)

type schemaGenerator struct{}

func (*schemaGenerator) JennyName() string {
	return "SchemaGenerator"
}

func (s *schemaGenerator) Generate(decl kindsys.Custom) (*codejen.File, error) {
	meta := decl.Def().Properties

	if decl.Def().Properties.CRD.Scope != string(resource.NamespacedScope) && decl.Def().Properties.CRD.Scope != string(resource.ClusterScope) {
		return nil, fmt.Errorf("scope '%s' is invalid, must be one of: '%s', '%s'",
			decl.Def().Properties.CRD.Scope, resource.ClusterScope, resource.NamespacedScope)
	}

	b := bytes.Buffer{}
	err := templates.WriteSchema(templates.SchemaMetadata{
		Package: meta.MachineName,
		Group:   decl.Def().Properties.CRD.Group,
		Version: versionString(meta.CurrentVersion),
		Kind:    meta.Name,
		Plural:  meta.PluralMachineName,
		Scope:   decl.Def().Properties.CRD.Scope,
	}, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile(fmt.Sprintf("%s/%s_schema_gen.go", meta.MachineName, meta.MachineName), formatted, s), nil
}
