package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/cli"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/examples/apiserver/apis"
	"github.com/grafana/grafana-app-sdk/examples/apiserver/apis/example/v1alpha1"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/k8s/apiserver"
	"github.com/grafana/grafana-app-sdk/k8s/apiserver/cmd/server"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"
)

type BasicModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}

func NewApp(config app.Config) (app.App, error) {
	client, err := v1alpha1.NewTestKindClientFromGenerator(k8s.NewClientRegistry(config.KubeConfig, k8s.ClientConfig{
		MetricsConfig: metrics.DefaultConfig(""),
		NegotiatedSerializerProvider: func(kind resource.Kind) runtime.NegotiatedSerializer {
			return &k8s.KindNegotiatedSerializer{
				Kind: kind,
			}
		},
	}))
	if err != nil {
		return nil, err
	}
	return simple.NewApp(simple.AppConfig{
		Name:       apis.LocalManifest().ManifestData.AppName,
		KubeConfig: config.KubeConfig,
		ManagedKinds: []simple.AppManagedKind{{
			Kind: v1alpha1.TestKindKind(),
			Validator: &simple.Validator{
				ValidateFunc: func(_ context.Context, request *app.AdmissionRequest) error {
					if request.Object.GetName() == "notallowed" {
						return fmt.Errorf("not allowed")
					}
					return nil
				},
			},
			Reconciler: &operator.TypedReconciler[*v1alpha1.TestKind]{
				ReconcileFunc: func(ctx context.Context, t operator.TypedReconcileRequest[*v1alpha1.TestKind]) (operator.ReconcileResult, error) {
					fmt.Printf("Reconciled %s\n", t.Object.GetName()) //nolint:revive
					// Example request to the subresource "/foo"
					resp, err := client.GetFoo(ctx, t.Object.GetStaticMetadata().Identifier(), v1alpha1.GetFooRequest{})
					if err != nil {
						return operator.ReconcileResult{}, fmt.Errorf("error calling /foo subresource: %w", err)
					}
					logging.FromContext(ctx).Info("called subresource", "status", resp.Status)
					return operator.ReconcileResult{}, nil
				},
			},
			CustomRoutes: map[simple.AppCustomRoute]simple.AppCustomRouteHandler{
				{
					Method: simple.AppCustomRouteMethodGet,
					Path:   "foo",
				}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
					logging.FromContext(ctx).Info("called foo subresource", "resource", request.ResourceIdentifier.Name, "namespace", request.ResourceIdentifier.Namespace)
					writer.WriteHeader(http.StatusOK)
					return json.NewEncoder(writer).Encode(v1alpha1.GetFoo{Status: "ok"})
				}, {
					Method: simple.AppCustomRouteMethodGet,
					Path:   "bar",
				}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
					logging.FromContext(ctx).Info("called foo subresource", "resource", request.ResourceIdentifier.Name, "namespace", request.ResourceIdentifier.Namespace)
					writer.WriteHeader(http.StatusOK)
					return json.NewEncoder(writer).Encode(v1alpha1.GetMessage{Message: "Hello, world!"})
				},
			},
		}},
	})
}

func main() {
	logging.DefaultLogger = logging.NewSLogLogger(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	provider := simple.NewAppProvider(apis.LocalManifest(), nil, NewApp)
	config := app.Config{
		KubeConfig:     rest.Config{}, // this will be replaced by the apiserver loopback config
		ManifestData:   *apis.LocalManifest().ManifestData,
		SpecificConfig: nil,
	}
	installer, err := apiserver.NewDefaultAppInstaller(provider, config, &apis.GoTypeAssociator{})
	if err != nil {
		panic(err)
	}
	ctx := genericapiserver.SetupSignalContext()
	opts := apiserver.NewOptions([]apiserver.AppInstaller{installer})
	opts.RecommendedOptions.Authentication = nil
	opts.RecommendedOptions.Authorization = nil
	opts.RecommendedOptions.CoreAPI = nil
	opts.RecommendedOptions.EgressSelector = nil
	opts.RecommendedOptions.Admission.Plugins = admission.NewPlugins()
	opts.RecommendedOptions.Admission.RecommendedPluginOrder = []string{}
	opts.RecommendedOptions.Admission.EnablePlugins = []string{}
	opts.RecommendedOptions.Features.EnablePriorityAndFairness = false
	opts.RecommendedOptions.ExtraAdmissionInitializers = func(_ *genericapiserver.RecommendedConfig) ([]admission.PluginInitializer, error) {
		return nil, nil
	}
	cmd := server.NewCommandStartServer(ctx, opts)
	code := cli.Run(cmd)
	os.Exit(code)
}
