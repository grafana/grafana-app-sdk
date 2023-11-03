package jennies

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/codegen"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

func WatcherJenny(projectRepo, codegenPath string, generatedKindsAreVersioned bool) codejen.OneToOne[codegen.Kind] {
	return &watcherJenny{
		projectRepo:                projectRepo,
		codegenPath:                codegenPath,
		generatedKindsAreVersioned: generatedKindsAreVersioned,
	}
}

type watcherJenny struct {
	projectRepo                string
	codegenPath                string
	generatedKindsAreVersioned bool
}

func (*watcherJenny) JennyName() string {
	return "Watcher"
}

func (w *watcherJenny) Generate(kind codegen.Kind) (*codejen.File, error) {
	if kind.Properties().APIResource == nil || !kind.Version(kind.Properties().Current).Codegen.Backend {
		return nil, nil
	}

	ver := kind.Properties().Current
	if !w.generatedKindsAreVersioned {
		ver = ""
	}
	props := kind.Properties()
	b := bytes.Buffer{}
	err := templates.WriteWatcher(templates.WatcherMetadata{
		KindProperties: props,
		PackageName:    "watchers",
		Repo:           w.projectRepo,
		CodegenPath:    w.codegenPath,
		Version:        ver,
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

type OperatorTelemetryJenny struct {
}

func (*OperatorTelemetryJenny) JennyName() string {
	return "OperatorTelemetry"
}

func (o *OperatorTelemetryJenny) Generate(_ ...codegen.Kind) (*codejen.File, error) {
	// TODO: combine this with config or keep separate?
	b := bytes.Buffer{}
	err := templates.WriteOperatorTelemetry(&b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile("cmd/operator/telemetry.go", formatted, o), nil
}

func OperatorMainJenny(projectRepo, codegenPath string, generatedKindsAreVersioned bool) codejen.ManyToOne[codegen.Kind] {
	return &operatorMainJenny{
		projectRepo:                projectRepo,
		codegenPath:                codegenPath,
		generatedKindsAreVersioned: generatedKindsAreVersioned,
	}
}

type operatorMainJenny struct {
	projectRepo                string
	codegenPath                string
	generatedKindsAreVersioned bool
}

func (*operatorMainJenny) JennyName() string {
	return "OperatorMain"
}

func (o *operatorMainJenny) Generate(kinds ...codegen.Kind) (*codejen.File, error) {
	tmd := templates.OperatorMainMetadata{
		Repo:                  o.projectRepo,
		CodegenPath:           o.codegenPath,
		PackageName:           "main",
		WatcherPackage:        "watchers",
		Resources:             make([]codegen.KindProperties, 0),
		ResourcesAreVersioned: o.generatedKindsAreVersioned,
	}

	for _, kind := range kinds {
		if kind.Properties().APIResource == nil {
			continue
		}
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
