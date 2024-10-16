package jennies

import (
	"bytes"
	"go/format"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type AppGenerator struct {
	GroupByKind bool
	ProjectRepo string
	ProjectName string
	CodegenPath string
}

func (*AppGenerator) JennyName() string {
	return "App"
}

func (a *AppGenerator) Generate(kinds ...codegen.Kind) (*codejen.File, error) {
	tmd := templates.AppMetadata{
		Repo:            a.ProjectRepo,
		ProjectName:     a.ProjectName,
		CodegenPath:     a.CodegenPath,
		PackageName:     "app",
		WatcherPackage:  "watchers",
		Resources:       make([]templates.AppMetadataKind, 0),
		KindsAreGrouped: !a.GroupByKind,
	}

	for _, kind := range kinds {
		if kind.Properties().APIResource == nil {
			continue
		}
		vers := make([]string, len(kind.Versions()))
		for i, ver := range kind.Versions() {
			vers[i] = ver.Version
		}
		tmd.Resources = append(tmd.Resources, templates.AppMetadataKind{
			KindProperties: kind.Properties(),
			Versions:       vers,
		})
	}

	b := bytes.Buffer{}
	err := templates.WriteAppGoFile(tmd, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile("pkg/app/app.go", formatted, a), nil
}
