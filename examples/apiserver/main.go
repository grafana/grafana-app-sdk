package main

import (
	"os"

	cmd "github.com/grafana/grafana-app-sdk/cmd/apiserver"
	corev1 "github.com/grafana/grafana-app-sdk/examples/apiserver/apis/core/v1"
	"github.com/grafana/grafana-app-sdk/simple"
	"k8s.io/component-base/cli"
)

func main() {
	r := simple.APIServerResource{
		Kind:                  corev1.Kind(),
		GetOpenAPIDefinitions: corev1.GetOpenAPIDefinitions,
	}
	g := simple.APIServerGroup{
		Name:     r.Kind.Group(),
		Resource: []simple.APIServerResource{r},
	}

	o := cmd.NewAPIServerOptions([]simple.APIServerGroup{g}, os.Stdout, os.Stderr)
	o.RecommendedOptions.Admission = nil
	o.RecommendedOptions.Authorization = nil
	o.RecommendedOptions.Authentication = nil
	o.RecommendedOptions.CoreAPI = nil

	ch := make(chan struct{})
	cmd := cmd.NewCommandStartAPIServer(o, ch)

	code := cli.Run(cmd)
	os.Exit(code)
}
