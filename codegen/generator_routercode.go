package codegen

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/kindsys"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type routerCodeGenerator struct {
	projectRepo string
}

func (r *routerCodeGenerator) Generate(decls ...kindsys.Custom) (*codejen.File, error) {
	tmd := templates.BackendPluginRouterTemplateMetadata{
		Repo:      r.projectRepo,
		Resources: make([]kindsys.CustomProperties, 0),
	}

	for _, decl := range decls {
		tmd.Resources = append(tmd.Resources, decl.Def().Properties)
	}

	b := bytes.Buffer{}
	err := templates.WriteBackendPluginRouter(tmd, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile("plugin/plugin.go", formatted, r), nil
}

func (*routerCodeGenerator) JennyName() string {
	return "routerCodeGenerator"
}

type routerHandlerCodeGenerator struct {
	projectRepo    string
	apiCodegenPath string
}

func (h *routerHandlerCodeGenerator) Generate(decl kindsys.Custom) (*codejen.File, error) {
	meta := decl.Def().Properties

	b := bytes.Buffer{}
	err := templates.WriteBackendPluginHandler(templates.BackendPluginHandlerTemplateMetadata{
		CustomProperties: meta,
		Repo:             h.projectRepo,
		APICodegenPath:   h.apiCodegenPath,
		TypeName:         typeNameFromKey(decl.Lineage().Name()),
		IsResource:       decl.Def().Properties.IsCRD,
	}, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile(fmt.Sprintf("plugin/handler_%s.go", meta.MachineName), formatted, h), nil
}

func (*routerHandlerCodeGenerator) JennyName() string {
	return "routerHandlerCodeGenerator"
}

type simpleFileReturnGenerator struct {
	file codejen.File
}

func (s *simpleFileReturnGenerator) Generate(...kindsys.Custom) (*codejen.File, error) {
	s.file.From = []codejen.NamedJenny{s}
	return &s.file, nil
}

func (s *simpleFileReturnGenerator) JennyName() string {
	return fmt.Sprintf("%s_generator", s.file.RelativePath)
}
