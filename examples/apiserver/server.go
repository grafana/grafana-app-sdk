package main

import (
	"os"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s/apiserver"
	"github.com/grafana/grafana-app-sdk/k8s/apiserver/cmd/server"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"
	"k8s.io/apiserver/pkg/admission"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/cli"
)

type BasicModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}

// Schema, Kind, and Manifest are typically generated, but can be crafted by hand as seen here.
// For anything more complex than this simple example, it is advised that you use the CLI (grafana-app-sdk generate) to get these values
var (
	schema = resource.NewSimpleSchema("example.grafana.com", "v1", &resource.TypedSpecObject[BasicModel]{}, &resource.TypedList[*resource.TypedSpecObject[BasicModel]]{}, resource.WithKind("BasicCustomResource"))
	kind   = resource.Kind{
		Schema: schema,
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
	manifest = app.NewEmbeddedManifest(app.ManifestData{
		AppName: "example-app",
		Group:   kind.Group(),
		Versions: []app.ManifestVersion{{
			Name: kind.Version(),
			Kinds: []app.ManifestVersionKind{{
				Schema: &app.VersionSchema{},
				Kind:   kind.Kind(),
				Scope:  string(kind.Scope()),
			}},
		}},
	})
)

func managedKindResolver(k, v string) (resource.Kind, error) {
	return kind, nil
}

func NewApp(config app.Config) (app.App, error) {
	return simple.NewApp(simple.AppConfig{
		Name:       "simple-reconciler-app",
		KubeConfig: config.KubeConfig,
		ManagedKinds: []simple.AppManagedKind{{
			Kind: kind,
		}},
	})
}

func main() {
	provider := simple.NewAppProvider(manifest, nil, NewApp)
	config := app.Config{
		KubeConfig:     rest.Config{}, // this will be replaced by the apiserver loopback config
		ManifestData:   *manifest.ManifestData,
		SpecificConfig: nil,
	}
	installer, err := apiserver.NewApIServerInstaller(provider, config, managedKindResolver)
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
