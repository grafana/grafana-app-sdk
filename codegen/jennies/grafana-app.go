package jennies

import (
	"bytes"
	"go/format"
	"path"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type GrafanaAppGenerator struct {
	ProjectRepo string
	ProjectName string
	APIsPath    string
}

func (*GrafanaAppGenerator) JennyName() string {
	return "GrafanaApp"
}

func (a *GrafanaAppGenerator) Generate(kinds ...codegen.Kind) (*codejen.File, error) {
	tmd := templates.AppMetadata{
		Repo:           a.ProjectRepo,
		ProjectName:    a.ProjectName,
		APIsPath:       a.APIsPath,
		PackageName:    "app",
		WatcherPackage: "watchers",
		Resources:      make([]templates.AppMetadataKind, 0),
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
	err := templates.WriteAppGoFile(tmd, &b, templates.TemplateGrafanaApp)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	// TODO: do inside codegen_path/package_name (apps/playlist)
	return codejen.NewFile(path.Join(tmd.CodegenPath, "pkg/app/app.go"), formatted, a), nil
}
