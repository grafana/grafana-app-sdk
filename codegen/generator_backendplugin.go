package codegen

import (
	"bytes"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/kindsys"
)

type backendPluginMainGenerator struct {
	projectRepo    string
	apiCodegenPath string
}

func (m *backendPluginMainGenerator) Generate(decls ...kindsys.Custom) (*codejen.File, error) {
	tmd := templates.BackendPluginRouterTemplateMetadata{
		Repo:           m.projectRepo,
		APICodegenPath: m.apiCodegenPath,
		PluginID:       "REPLACEME",
		Resources:      make([]kindsys.CustomProperties, 0),
	}

	for _, decl := range decls {
		tmd.Resources = append(tmd.Resources, decl.Def().Properties)
		if decl.Def().Properties.Group != "" {
			tmd.PluginID = decl.Def().Properties.Group
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
