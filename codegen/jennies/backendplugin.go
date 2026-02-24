package jennies

import (
	"bytes"
	"go/format"
	"strings"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

// BackendPluginMainGenerator returns a many-to-one jenny which generates the `main.go` file needed to run the backend plugin.
func BackendPluginMainGenerator(projectRepo, apiCodegenPath string, groupByKind bool) codejen.OneToOne[codegen.AppManifest] {
	return &backendPluginMainGenerator{
		projectRepo:    projectRepo,
		apiCodegenPath: apiCodegenPath,
		groupByKind:    groupByKind,
	}
}

type backendPluginMainGenerator struct {
	projectRepo    string
	apiCodegenPath string
	groupByKind    bool
}

func (m *backendPluginMainGenerator) Generate(appManifest codegen.AppManifest) (*codejen.File, error) {
	tmd := templates.BackendPluginRouterTemplateMetadata{
		Repo:            m.projectRepo,
		APICodegenPath:  m.apiCodegenPath,
		PluginID:        "REPLACEME",
		Resources:       make([]codegen.KindProperties, 0),
		KindsAreGrouped: !m.groupByKind,
	}

	for _, version := range appManifest.Versions() {
		for _, kind := range version.Kinds() {
			tmd.Resources = append(tmd.Resources, versionedKindToKindProperties(kind, appManifest))
			if appManifest.Properties().FullGroup != "" {
				tmd.PluginID = strings.Split(appManifest.Properties().FullGroup, ".")[0]
			}
		}
	}

	b := bytes.Buffer{}
	err := templates.WriteBackendPluginMain(tmd, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile("../plugin/pkg/main.go", formatted, m), nil
}

func (*backendPluginMainGenerator) JennyName() string {
	return "backendPluginMainGenerator"
}
