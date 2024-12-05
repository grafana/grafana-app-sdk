package jennies

import (
	"bytes"
	"strings"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

// BackendPluginMainGenerator returns a many-to-one jenny which generates the `main.go` file needed to run the backend plugin.
func BackendPluginMainGenerator(projectRepo, apiCodegenPath string, groupByKind bool) codejen.ManyToOne[codegen.Kind] {
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

func (m *backendPluginMainGenerator) Generate(decls ...codegen.Kind) (*codejen.File, error) {
	tmd := templates.BackendPluginRouterTemplateMetadata{
		Repo:            m.projectRepo,
		APICodegenPath:  m.apiCodegenPath,
		PluginID:        "REPLACEME",
		Resources:       make([]codegen.KindProperties, 0),
		KindsAreGrouped: !m.groupByKind,
	}

	for _, decl := range decls {
		tmd.Resources = append(tmd.Resources, decl.Properties())
		if decl.Properties().Group != "" {
			tmd.PluginID = strings.Split(decl.Properties().Group, ".")[0]
		}
	}

	b := bytes.Buffer{}
	err := templates.WriteBackendPluginMain(tmd, &b)
	if err != nil {
		return nil, err
	}
	return codejen.NewFile("../plugin/pkg/main.go", b.Bytes(), m), nil
}

func (*backendPluginMainGenerator) JennyName() string {
	return "backendPluginMainGenerator"
}
