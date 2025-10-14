package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/cli"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/examples/apiserver/apis"
	"github.com/grafana/grafana-app-sdk/examples/apiserver/apis/example/v0alpha1"
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

//nolint:funlen
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
					logging.FromContext(ctx).Info("called TestKind /foo subresource", "resource", request.ResourceIdentifier.Name, "namespace", request.ResourceIdentifier.Namespace)
					writer.WriteHeader(http.StatusOK)
					return json.NewEncoder(writer).Encode(v1alpha1.GetFoo{
						TypeMeta: metav1.TypeMeta{
							Kind:       "TestKind.Foo",
							APIVersion: config.ManifestData.Group + "/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      request.ResourceIdentifier.Name,
							Namespace: request.ResourceIdentifier.Namespace,
						},
						GetFooBody: v1alpha1.GetFooBody{Status: "ok"},
					})
				}, {
					Method: simple.AppCustomRouteMethodGet,
					Path:   "bar",
				}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
					logging.FromContext(ctx).Info("called TestKind /bar subresource", "resource", request.ResourceIdentifier.Name, "namespace", request.ResourceIdentifier.Namespace)
					writer.WriteHeader(http.StatusOK)
					return json.NewEncoder(writer).Encode(v1alpha1.GetMessage{
						TypeMeta: metav1.TypeMeta{
							Kind:       "TestKind.Message",
							APIVersion: config.ManifestData.Group + "/v1alpha1",
						},
						GetMessageBody: v1alpha1.GetMessageBody{
							Message: "Hello, world!",
						},
					})
				},
				{
					Method: simple.AppCustomRouteMethodGet,
					Path:   "recurse",
				}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
					logging.FromContext(ctx).Info("called TestKind /recurse subresource", "resource", request.ResourceIdentifier.Name, "namespace", request.ResourceIdentifier.Namespace)
					writer.WriteHeader(http.StatusOK)
					return json.NewEncoder(writer).Encode(v1alpha1.GetRecursiveResponse{
						TypeMeta: metav1.TypeMeta{
							Kind:       "TestKind.Message",
							APIVersion: config.ManifestData.Group + "/v1alpha1",
						},
						GetRecursiveResponseBody: v1alpha1.GetRecursiveResponseBody{
							Message: "Hello, world!",
							Next: &v1alpha1.VersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType{
								Message: "Hello again!",
								Next: &v1alpha1.VersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType{
									Message: "Hello once more!",
								},
							},
						},
					})
				},
			},
		}},
		Converters: map[schema.GroupKind]simple.Converter{
			{
				Group: config.ManifestData.Group,
				Kind:  v1alpha1.TestKindKind().Kind(),
			}: NewTestKindConverter(),
		},
		VersionedCustomRoutes: map[string]simple.AppVersionRouteHandlers{
			"v1alpha1": {
				{
					Namespaced: true,
					Path:       "foobar",
					Method:     "GET",
				}: func(_ context.Context, writer app.CustomRouteResponseWriter, _ *app.CustomRouteRequest) error {
					return json.NewEncoder(writer).Encode(v1alpha1.GetFoobar{
						TypeMeta: metav1.TypeMeta{
							Kind:       "NamespacedFoobar",
							APIVersion: config.ManifestData.Group + "/v1alpha1",
						},
						GetFoobarBody: v1alpha1.GetFoobarBody{
							Foo: "hello, world!",
						},
					})
				},
				{
					Namespaced: false,
					Path:       "foobar",
					Method:     "GET",
				}: func(_ context.Context, writer app.CustomRouteResponseWriter, _ *app.CustomRouteRequest) error {
					return json.NewEncoder(writer).Encode(v1alpha1.GetClusterFoobar{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ClusterFoobar",
							APIVersion: config.ManifestData.Group + "/v1alpha1",
						},
						GetClusterFoobarBody: v1alpha1.GetClusterFoobarBody{
							Bar: "hello, world!",
						},
					})
				},
			},
		},
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

var _ simple.Converter = NewTestKindConverter()

type TestKindConverter struct{}

func NewTestKindConverter() *TestKindConverter {
	return &TestKindConverter{}
}

//nolint:funlen
func (*TestKindConverter) Convert(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
	srcGVK := schema.FromAPIVersionAndKind(obj.APIVersion, obj.Kind)
	dstGVK := schema.FromAPIVersionAndKind(targetAPIVersion, v1alpha1.TestKindKind().Kind())
	if srcGVK.Group != v1alpha1.APIGroup {
		// This should never happen, but check just in case
		return nil, fmt.Errorf("wrong group to convert example.grafana.app, got %s", srcGVK.Group)
	}
	if srcGVK.Kind != v1alpha1.TestKindKind().Kind() {
		// This should also never happen, but check just in case
		return nil, fmt.Errorf("wrong kind to convert Example, got %s", srcGVK.Kind)
	}
	if srcGVK == dstGVK {
		// This should never happen, but if it does no conversion is necessary, we can return the input
		return obj.Raw, nil
	}

	// Check source version
	switch srcGVK.Version {
	case v0alpha1.APIVersion:
		srcKind := v0alpha1.TestKindKind()
		uncastSrcObj, err := srcKind.Read(bytes.NewReader(obj.Raw), resource.KindEncodingJSON)
		if err != nil {
			return nil, fmt.Errorf("unable to parse JSON bytes into %s: %w", srcGVK.String(), err)
		}
		srcObj, ok := uncastSrcObj.(*v0alpha1.TestKind)
		if !ok {
			return nil, errors.New("read object was not of type *v0alpha1.Example")
		}
		switch dstGVK.Version {
		case v1alpha1.APIVersion:
			dstObj := &v1alpha1.TestKind{}
			// Set Type metadata
			dstObj.SetGroupVersionKind(dstGVK)
			// Copy Object metadata
			srcObj.ObjectMeta.DeepCopyInto(&dstObj.ObjectMeta)
			// Copy spec and status
			dstObj.Spec.TestField = strconv.Itoa(int(srcObj.Spec.TestField))
			dstObj.Status.AdditionalFields = srcObj.Status.AdditionalFields
			if srcObj.Status.OperatorStates != nil {
				dstObj.Status.OperatorStates = make(map[string]v1alpha1.TestKindstatusOperatorState)
				for k, v := range srcObj.Status.OperatorStates {
					dstObj.Status.OperatorStates[k] = v1alpha1.TestKindstatusOperatorState{
						LastEvaluation:   v.LastEvaluation,
						State:            v1alpha1.TestKindStatusOperatorStateState(v.State),
						DescriptiveState: v.DescriptiveState,
						Details:          v.Details,
					}
				}
			}
			dstKind := v1alpha1.TestKindKind()
			buf := &bytes.Buffer{}
			err := dstKind.Write(dstObj, buf, resource.KindEncodingJSON)
			return buf.Bytes(), err
		default:
			return nil, fmt.Errorf("unknown target version %s", dstGVK.Version)
		}
	case v1alpha1.APIVersion:
		srcKind := v1alpha1.TestKindKind()
		uncastSrcObj, err := srcKind.Read(bytes.NewReader(obj.Raw), resource.KindEncodingJSON)
		if err != nil {
			return nil, fmt.Errorf("unable to parse JSON bytes into %s: %w", srcGVK.String(), err)
		}
		srcObj, ok := uncastSrcObj.(*v1alpha1.TestKind)
		if !ok {
			return nil, errors.New("read object was not of type *v1alpha1.Example")
		}
		switch dstGVK.Version {
		case v0alpha1.APIVersion:
			dstObj := &v0alpha1.TestKind{}
			// Set Type metadata
			dstObj.SetGroupVersionKind(dstGVK)
			// Copy Object metadata
			srcObj.ObjectMeta.DeepCopyInto(&dstObj.ObjectMeta)
			// Copy spec and status
			castInt, _ := strconv.Atoi(srcObj.Spec.TestField) // Lossy backwards conversion
			dstObj.Spec.TestField = int64(castInt)
			dstObj.Status.AdditionalFields = srcObj.Status.AdditionalFields
			if srcObj.Status.OperatorStates != nil {
				dstObj.Status.OperatorStates = make(map[string]v0alpha1.TestKindstatusOperatorState)
				for k, v := range srcObj.Status.OperatorStates {
					dstObj.Status.OperatorStates[k] = v0alpha1.TestKindstatusOperatorState{
						LastEvaluation:   v.LastEvaluation,
						State:            v0alpha1.TestKindStatusOperatorStateState(v.State),
						DescriptiveState: v.DescriptiveState,
						Details:          v.Details,
					}
				}
			}
			dstKind := v0alpha1.TestKindKind()
			buf := &bytes.Buffer{}
			err := dstKind.Write(dstObj, buf, resource.KindEncodingJSON)
			return buf.Bytes(), err
		default:
			return nil, fmt.Errorf("unknown target version %s", dstGVK.Version)
		}
	}
	return nil, fmt.Errorf("unknown source version %s", srcGVK.Version)
}
