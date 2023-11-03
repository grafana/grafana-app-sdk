package jennies

import (
	"bytes"

	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/codegen"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

// BackendPluginMainGenerator returns a many-to-one jenny which generates the `main.go` file needed to run the backend plugin.
func BackendPluginMainGenerator(projectRepo, apiCodegenPath string, generatedKindsAreVersioned bool) codejen.ManyToOne[codegen.Kind] {
	return &backendPluginMainGenerator{
		projectRepo:                projectRepo,
		apiCodegenPath:             apiCodegenPath,
		generatedKindsAreVersioned: generatedKindsAreVersioned,
	}
}

type backendPluginMainGenerator struct {
	projectRepo                string
	apiCodegenPath             string
	generatedKindsAreVersioned bool
}

func (m *backendPluginMainGenerator) Generate(decls ...codegen.Kind) (*codejen.File, error) {
	tmd := templates.BackendPluginRouterTemplateMetadata{
		Repo:                  m.projectRepo,
		APICodegenPath:        m.apiCodegenPath,
		PluginID:              "REPLACEME",
		Resources:             make([]codegen.KindProperties, 0),
		ResourcesAreVersioned: m.generatedKindsAreVersioned,
	}

	for _, decl := range decls {
		tmd.Resources = append(tmd.Resources, decl.Properties())
		if decl.Properties().Group != "" {
			tmd.PluginID = decl.Properties().Group
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
