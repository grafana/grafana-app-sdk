package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

func WatcherJenny(projectRepo, codegenPath string, groupByKind bool) codejen.OneToMany[codegen.AppManifest] {
	return &watcherJenny{
		projectRepo: projectRepo,
		codegenPath: codegenPath,
		groupByKind: groupByKind,
	}
}

type watcherJenny struct {
	projectRepo string
	codegenPath string
	groupByKind bool
}

func (*watcherJenny) JennyName() string {
	return "Watcher"
}

func (w *watcherJenny) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0)
	for _, version := range appManifest.Versions() {
		if version.Name() != appManifest.Properties().PreferredVersion {
			continue
		}
		for _, kind := range version.Kinds() {
			if !kind.Codegen.Go.Enabled {
				continue
			}
			props := versionedKindToKindProperties(kind, appManifest)
			b := bytes.Buffer{}
			err := templates.WriteWatcher(templates.WatcherMetadata{
				KindProperties:   props,
				PackageName:      "watchers",
				Repo:             w.projectRepo,
				CodegenPath:      w.codegenPath,
				Version:          version.Name(),
				KindPackage:      GetGeneratedGoTypePath(w.groupByKind, appManifest.Properties().Group, version.Name(), kind.MachineName),
				KindsAreGrouped:  !w.groupByKind,
				KindPackageAlias: fmt.Sprintf("%s%s", kind.MachineName, version.Name()),
			}, &b)
			if err != nil {
				return nil, err
			}
			formatted, err := format.Source(b.Bytes())
			if err != nil {
				return nil, err
			}
			files = append(files, codejen.File{
				RelativePath: fmt.Sprintf("pkg/watchers/watcher_%s.go", props.MachineName),
				Data:         formatted,
				From:         []codejen.NamedJenny{w},
			})
		}
	}

	return files, nil
}

type OperatorKubeConfigJenny struct {
}

func (*OperatorKubeConfigJenny) JennyName() string {
	return "OperatorKubeConfig"
}

func (o *OperatorKubeConfigJenny) Generate(_ codegen.AppManifest) (*codejen.File, error) {
	b := bytes.Buffer{}
	err := templates.WriteOperatorKubeConfig(&b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile("cmd/operator/kubeconfig.go", formatted, o), nil
}

type OperatorConfigJenny struct {
}

func (*OperatorConfigJenny) JennyName() string {
	return "OperatorConfig"
}

func (o *OperatorConfigJenny) Generate(_ codegen.AppManifest) (*codejen.File, error) {
	// TODO: combine this with kubeconfig?
	b := bytes.Buffer{}
	err := templates.WriteOperatorConfig(&b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile("cmd/operator/config.go", formatted, o), nil
}

func OperatorMainJenny(projectRepo, codegenPath string, groupByKind bool) codejen.OneToOne[codegen.AppManifest] {
	parts := strings.Split(projectRepo, "/")
	if len(parts) == 0 {
		parts = []string{""}
	}
	return &operatorMainJenny{
		projectRepo: projectRepo,
		projectName: parts[len(parts)-1],
		codegenPath: codegenPath,
		groupByKind: groupByKind,
	}
}

type operatorMainJenny struct {
	projectRepo string
	projectName string
	codegenPath string
	groupByKind bool
}

func (*operatorMainJenny) JennyName() string {
	return "OperatorMain"
}

func (o *operatorMainJenny) Generate(_ codegen.AppManifest) (*codejen.File, error) {
	tmd := templates.OperatorMainMetadata{
		Repo:            o.projectRepo,
		ProjectName:     o.projectName,
		CodegenPath:     o.codegenPath,
		PackageName:     "main",
		WatcherPackage:  "watchers",
		Resources:       make([]codegen.KindProperties, 0),
		KindsAreGrouped: !o.groupByKind,
	}

	b := bytes.Buffer{}
	err := templates.WriteOperatorMain(tmd, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile("cmd/operator/main.go", formatted, o), nil
}
