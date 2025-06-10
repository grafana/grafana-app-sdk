package main

import (
	"os"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/cli"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/examples/apiserver/apis"
	"github.com/grafana/grafana-app-sdk/examples/apiserver/apis/example/v1alpha1"
	"github.com/grafana/grafana-app-sdk/k8s/apiserver"
	"github.com/grafana/grafana-app-sdk/k8s/apiserver/cmd/server"
	"github.com/grafana/grafana-app-sdk/simple"
)

type BasicModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}

func NewApp(config app.Config) (app.App, error) {
	return simple.NewApp(simple.AppConfig{
		Name:       apis.LocalManifest().ManifestData.AppName,
		KubeConfig: config.KubeConfig,
		ManagedKinds: []simple.AppManagedKind{{
			Kind: v1alpha1.TestKindKind(),
		}},
	})
}

func main() {
	provider := simple.NewAppProvider(apis.LocalManifest(), nil, NewApp)
	config := app.Config{
		KubeConfig:     rest.Config{}, // this will be replaced by the apiserver loopback config
		ManifestData:   *apis.LocalManifest().ManifestData,
		SpecificConfig: nil,
	}
	installer, err := apiserver.NewApIServerInstaller(provider, config, apiserver.ManagedKindResolver(apis.ManifestGoTypeAssociator), func(gvk schema.GroupVersionKind) (string, bool) {
		return "github.com/grafana/grafana-app-sdk/examples/apiserver/apis/example", true
	})
	if err != nil {
		panic(err)
	}
	ctx := genericapiserver.SetupSignalContext()
	opts := apiserver.NewOptions([]apiserver.APIServerInstaller{installer})
	opts.RecommendedOptions.Authentication = nil
	opts.RecommendedOptions.Authorization = nil
	opts.RecommendedOptions.CoreAPI = nil
	opts.RecommendedOptions.EgressSelector = nil
	opts.RecommendedOptions.Admission.Plugins = admission.NewPlugins()
	opts.RecommendedOptions.Admission.RecommendedPluginOrder = []string{}
	opts.RecommendedOptions.Admission.EnablePlugins = []string{}
	opts.RecommendedOptions.Features.EnablePriorityAndFairness = false
	opts.RecommendedOptions.ExtraAdmissionInitializers = func(c *genericapiserver.RecommendedConfig) ([]admission.PluginInitializer, error) {
		return nil, nil
	}
	cmd := server.NewCommandStartServer(ctx, opts)
	code := cli.Run(cmd)
	os.Exit(code)

}
