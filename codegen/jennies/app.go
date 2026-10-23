package jennies

import (
	"bytes"
	"go/format"
	"slices"
	"strings"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type AppGenerator struct {
	GroupByKind         bool
	ProjectRepo         string
	ProjectName         string
	CodegenPath         string
	ManifestPackagePath string
}

func (*AppGenerator) JennyName() string {
	return "App"
}

func (a *AppGenerator) Generate(appManifest codegen.AppManifest) (*codejen.File, error) {
	tmd := templates.AppMetadata{
		Repo:                a.ProjectRepo,
		ProjectName:         a.ProjectName,
		CodegenPath:         a.CodegenPath,
		PackageName:         "app",
		WatcherPackage:      "watchers",
		Resources:           make([]templates.AppMetadataKind, 0),
		KindsAreGrouped:     !a.GroupByKind,
		ManifestPackagePath: a.ManifestPackagePath,
	}

	appMetadataByKind := make(map[string]templates.AppMetadataKind)

	for version, kind := range codegen.VersionedKinds(appManifest) {
		meta, ok := appMetadataByKind[kind.Kind]
		if !ok {
			meta = templates.AppMetadataKind{
				KindProperties: versionedKindToKindProperties(kind, appManifest),
				Versions:       make([]string, 0),
			}
		}
		meta.Versions = append(meta.Versions, version.Name())
		if version.Name() == appManifest.Properties().PreferredVersion {
			meta.KindProperties = versionedKindToKindProperties(kind, appManifest)
		}
		appMetadataByKind[kind.Kind] = meta
	}

	for _, meta := range appMetadataByKind {
		tmd.Resources = append(tmd.Resources, meta)
	}
	// Sort for deterministic output
	slices.SortFunc(tmd.Resources, func(a, b templates.AppMetadataKind) int {
		return strings.Compare(a.Kind, b.Kind)
	})

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
