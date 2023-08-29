package codegen

import (
	"bytes"
	"fmt"
	"go/format"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/kindsys"
)

type watcherJenny struct {
	projectRepo string
	codegenPath string
}

func (*watcherJenny) JennyName() string {
	return "Watcher"
}

func (w *watcherJenny) Generate(decl kindsys.Custom) (*codejen.File, error) {
	if !decl.Def().Properties.IsCRD || !decl.Def().Properties.Codegen.Backend {
		return nil, nil
	}

	props := decl.Def().Properties
	b := bytes.Buffer{}
	err := templates.WriteWatcher(templates.WatcherMetadata{
		CustomProperties: props,
		PackageName:      "watchers",
		Repo:             w.projectRepo,
		CodegenPath:      w.codegenPath,
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

type operatorKubeConfigJenny struct {
}

func (*operatorKubeConfigJenny) JennyName() string {
	return "OperatorKubeConfig"
}

func (o *operatorKubeConfigJenny) Generate(_ ...kindsys.Custom) (*codejen.File, error) {
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

type operatorConfigJenny struct {
}

func (*operatorConfigJenny) JennyName() string {
	return "OperatorConfig"
}

func (o *operatorConfigJenny) Generate(_ ...kindsys.Custom) (*codejen.File, error) {
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

type operatorTelemetryJenny struct {
}

func (*operatorTelemetryJenny) JennyName() string {
	return "OperatorTelemetry"
}

func (o *operatorTelemetryJenny) Generate(_ ...kindsys.Custom) (*codejen.File, error) {
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

type operatorMainJenny struct {
	projectRepo string
	codegenPath string
}

func (*operatorMainJenny) JennyName() string {
	return "OperatorMain"
}

func (o *operatorMainJenny) Generate(decls ...kindsys.Custom) (*codejen.File, error) {
	tmd := templates.OperatorMainMetadata{
		Repo:           o.projectRepo,
		CodegenPath:    o.codegenPath,
		PackageName:    "main",
		WatcherPackage: "watchers",
		Resources:      make([]kindsys.CustomProperties, 0),
	}

	for _, decl := range decls {
		if !decl.Def().Properties.IsCRD {
			continue
		}
		tmd.Resources = append(tmd.Resources, decl.Def().Properties)
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
