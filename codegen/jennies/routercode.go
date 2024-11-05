package jennies

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

func RouterCodeGenerator(projectRepo string) codejen.ManyToOne[codegen.Kind] {
	return &routerCodeGenerator{
		projectRepo: projectRepo,
	}
}

type routerCodeGenerator struct {
	projectRepo string
	groupByKind bool
}

func (r *routerCodeGenerator) Generate(decls ...codegen.Kind) (*codejen.File, error) {
	tmd := templates.BackendPluginRouterTemplateMetadata{
		Repo:            r.projectRepo,
		Resources:       make([]codegen.KindProperties, 0),
		KindsAreGrouped: !r.groupByKind,
	}

	for _, decl := range decls {
		tmd.Resources = append(tmd.Resources, decl.Properties())
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

func RouterHandlerCodeGenerator(projectRepo, apiCodegenPath string, groupByKind bool) codejen.OneToOne[codegen.Kind] {
	return &routerHandlerCodeGenerator{
		projectRepo:    projectRepo,
		apiCodegenPath: apiCodegenPath,
		groupByKind:    groupByKind,
	}
}

type routerHandlerCodeGenerator struct {
	projectRepo    string
	apiCodegenPath string
	groupByKind    bool
}

func (h *routerHandlerCodeGenerator) Generate(decl codegen.Kind) (*codejen.File, error) {
	meta := decl.Properties()

	ver := ToPackageName(decl.Properties().Current)
	b := bytes.Buffer{}
	err := templates.WriteBackendPluginHandler(templates.BackendPluginHandlerTemplateMetadata{
		KindProperties:  meta,
		Repo:            h.projectRepo,
		APICodegenPath:  h.apiCodegenPath,
		TypeName:        exportField(decl.Properties().Kind),
		IsResource:      true,
		Version:         ver,
		KindPackage:     GetGeneratedPath(h.groupByKind, decl, ver),
		KindsAreGrouped: !h.groupByKind,
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

func StaticManyToOneGenerator[Input any](file codejen.File) codejen.ManyToOne[Input] {
	return &simpleManyToOneGenerator[Input]{
		file: file,
	}
}

type simpleManyToOneGenerator[I any] struct {
	file codejen.File
}

func (s *simpleManyToOneGenerator[I]) Generate(...I) (*codejen.File, error) {
	s.file.From = []codejen.NamedJenny{s}
	return &s.file, nil
}

func (s *simpleManyToOneGenerator[I]) JennyName() string {
	return fmt.Sprintf("%s_generator", s.file.RelativePath)
}
