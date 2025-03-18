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

func WatcherJenny(projectRepo, codegenPath string, groupByKind bool) codejen.OneToOne[codegen.Kind] {
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

func (w *watcherJenny) Generate(kind codegen.Kind) (*codejen.File, error) {
	if !kind.Version(kind.Properties().Current).Codegen.Go.Enabled {
		return nil, nil
	}

	ver := kind.Properties().Current
	props := kind.Properties()
	b := bytes.Buffer{}
	err := templates.WriteWatcher(templates.WatcherMetadata{
		KindProperties:   props,
		PackageName:      "watchers",
		Repo:             w.projectRepo,
		CodegenPath:      w.codegenPath,
		Version:          ver,
		KindPackage:      GetGeneratedPath(w.groupByKind, kind, ver),
		KindsAreGrouped:  !w.groupByKind,
		KindPackageAlias: fmt.Sprintf("%s%s", kind.Properties().MachineName, kind.Properties().Current),
	}, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile(fmt.Sprintf("pkg/watchers/watcher_%s.go", props.MachineName), formatted, w), nil
}

type OperatorKubeConfigJenny struct {
}

func (*OperatorKubeConfigJenny) JennyName() string {
	return "OperatorKubeConfig"
}

func (o *OperatorKubeConfigJenny) Generate(_ ...codegen.Kind) (*codejen.File, error) {
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

func (o *OperatorConfigJenny) Generate(_ ...codegen.Kind) (*codejen.File, error) {
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

func OperatorMainJenny(projectRepo, codegenPath string, groupByKind bool) codejen.ManyToOne[codegen.Kind] {
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

func (o *operatorMainJenny) Generate(kinds ...codegen.Kind) (*codejen.File, error) {
	tmd := templates.OperatorMainMetadata{
		Repo:            o.projectRepo,
		ProjectName:     o.projectName,
		CodegenPath:     o.codegenPath,
		PackageName:     "main",
		WatcherPackage:  "watchers",
		Resources:       make([]codegen.KindProperties, 0),
		KindsAreGrouped: !o.groupByKind,
	}

	for _, kind := range kinds {
		tmd.Resources = append(tmd.Resources, kind.Properties())
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
