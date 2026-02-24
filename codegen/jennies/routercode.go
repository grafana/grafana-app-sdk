package jennies

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

func RouterCodeGenerator(projectRepo string) codejen.OneToOne[codegen.AppManifest] {
	return &routerCodeGenerator{
		projectRepo: projectRepo,
	}
}

type routerCodeGenerator struct {
	projectRepo string
	groupByKind bool
}

func (r *routerCodeGenerator) Generate(appManifest codegen.AppManifest) (*codejen.File, error) {
	tmd := templates.BackendPluginRouterTemplateMetadata{
		Repo:            r.projectRepo,
		Resources:       make([]codegen.KindProperties, 0),
		KindsAreGrouped: !r.groupByKind,
	}

	for _, version := range appManifest.Versions() {
		if version.Name() != appManifest.Properties().PreferredVersion {
			continue
		}
		for _, kind := range version.Kinds() {
			tmd.Resources = append(tmd.Resources, versionedKindToKindProperties(kind, appManifest))
		}
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

func RouterHandlerCodeGenerator(projectRepo, apiCodegenPath string, groupByKind bool) codejen.OneToMany[codegen.AppManifest] {
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

func (h *routerHandlerCodeGenerator) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0)
	for _, version := range appManifest.Versions() {
		if version.Name() != appManifest.Properties().PreferredVersion {
			continue
		}
		for _, kind := range version.Kinds() {
			b := bytes.Buffer{}
			err := templates.WriteBackendPluginHandler(templates.BackendPluginHandlerTemplateMetadata{
				KindProperties:  versionedKindToKindProperties(kind, appManifest),
				Repo:            h.projectRepo,
				APICodegenPath:  h.apiCodegenPath,
				TypeName:        exportField(kind.Kind),
				IsResource:      true,
				Version:         version.Name(),
				KindPackage:     GetGeneratedGoTypePath(h.groupByKind, appManifest.Properties().Group, version.Name(), kind.MachineName),
				KindsAreGrouped: !h.groupByKind,
			}, &b)
			if err != nil {
				return nil, err
			}
			formatted, err := format.Source(b.Bytes())
			if err != nil {
				return nil, err
			}
			files = append(files, codejen.File{
				RelativePath: fmt.Sprintf("plugin/handler_%s.go", kind.MachineName),
				Data:         formatted,
				From:         []codejen.NamedJenny{h},
			})
		}
	}
	return files, nil
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
