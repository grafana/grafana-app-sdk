# Writing an App

Within the context of the `grafana-app-sdk`, an **App** is a collection of behaviors which can be run by a runner, typically as a kubernetes operator. 
An **App** and the way which it is run are decoupled, such that an **App** can be run in any supported runner format. 
Currently, the only out-of-the-box support in the `grafana-app-sdk` is for running an **App** as a standalone kubernetes operator, 
but additional runners are expected to be added in the future.

## Quickstart

To quickly get app code to look at, either consider running through the [tutorial](tutorials/issue-tracker/README.md), 
or run the following commands in a directory you want to build your app in:
```bash
grafana-app-sdk project init test-app
```
```bash
grafana-app-sdk project kind add MyTestKind
```
```bash
grafana-app-sdk generate
```
```bash
grafana-app-sdk project component add operator
```

## Considerations

As mentioned, an **App** is a collection of behaviors, which is defined by the [app.App](../app/app.go#L100) interface:
```go
type App interface {
	// Validate validates the incoming request, and returns an error if validation fails
	Validate(ctx context.Context, request *AdmissionRequest) error
	// Mutate runs mutation on the incoming request, responding with a MutatingResponse on success, or an error on failure
	Mutate(ctx context.Context, request *AdmissionRequest) (*MutatingResponse, error)
	// Convert converts the object based on the ConversionRequest, returning a RawObject which MUST contain
	// the converted bytes and encoding (Raw and Encoding respectively), and MAY contain the Object representation of those bytes.
	// It returns an error if the conversion fails, or if the functionality is not supported by the app.
	Convert(ctx context.Context, req ConversionRequest) (*RawObject, error)
	// CallResourceCustomRoute handles the call to a resource custom route, and returns a response to the request or an error.
	// If the route doesn't exist, the implementer MAY return ErrCustomRouteNotFound to signal to the runner,
	// or may choose to return a response with a not found status code and custom body.
	// It returns an error if the functionality is not supported by the app.
	CallResourceCustomRoute(ctx context.Context, request *ResourceCustomRouteRequest) (*ResourceCustomRouteResponse, error)
	// ManagedKinds returns a slice of Kinds which are managed by this App.
	// If there are multiple versions of a Kind, each one SHOULD be returned by this method,
	// as app runners may depend on having access to all kinds.
	ManagedKinds() []resource.Kind
	// Runner returns a Runnable with an app main loop. Any business logic that is not/can not be exposed
	// via other App interfaces should be contained within this method.
	// Runnable MAY be nil, in which case, the app has no main loop business logic.
	Runner() Runnable
}
```

An **App** doesn't need to support all these behaviors, and if it doesn't, it returns `app.ErrNotImplemented` from that function. 
The **App** indicates which behaviors it supports to the runner via its **Manifest**.

All of these behaviors (except `Runner()` and `ManagedKinds()`) are actions taken on one or more kinds managed by the app. 
For example, when a new resource is created for a kind which the app manages, `Mutate` and `Validate` will both be called if supported. 
`Convert` may be called if the stored version is different from the version used in the create request.

`CallResourceCustomRoute` is currently unsupported in the operator runner (as it is not a supported action for a standalone kubernetes operator), 
but is to be used on custom subroutes for resources (unsupported by kubernetes CRDs). This will be further elaborated on when new runners are released.

`ManagedKinds()` is used by the runner to get the go types for kinds managed by the app (as the manifest is data and does not contain the go types used).

Finally, `Runner()` returns a main loop for the app, for logic which is not explicitly event-based like `Validate`, `Mutate`, and `Convert`. 
However, often this main loop should _also_ be event-based, using the operator pattern to watch resources and react to changes. 
This behavior is the default behavior of `Runner()` in `simple.App`.

## App Manifest

The app manifest is a collection of data about which kinds are managed by the app, what behaviors are supported by the app for each kind, 
and other additional information needed for the app to function (such as additional permissions). 
When you write your CUE manifest and use `grafana-app-sdk generate`, this manifest will be automatically generated for you, 
as both an embeddable go variable, or a JSON/YAML custom resource. Writing a manifest from scratch is not advised, but can be done. 
See [manifest.cue](../app/manifest.cue) for the definition of the custom resource, or [manifest.go](../app/manifest.go) 
to write your `app.ManifestData` in go directly.

The easiest way to get started on this is to use `grafana-app-sdk project init <my-project-module-name>`. 
This will create a starting CUE manifest to use with `grafana-app-sdk generate`. You can add template kinds to this with 
`grafana-app-sdk project kind add <MyKindName>`.

## `simple.App`

The standard way to build an app, without implementing `app.App` yourself, is to use the `simple` package's `App` type. 
`simple.NewApp` creates a new `simple.App`, using the provided `simple.AppConfig`. 
`simple.App` runs an operator as its main `Runner()` code, and the config has you specify watchers or reconcilers for 
the kinds you manage with the app. 

A `simple.App` has two variants for kinds which you attach watchers/reconcilers to: `AppManagedKinds` and `AppUnmanagedKinds`. 
`AppManagedKinds` are kinds which your app owns and manages, which you can attach mutation, validation, and conversion logic to. 
`AppUnmanagedKinds` are kinds which your app wants to watch (such as dashboards), but does not own or manage.

The easiest way to see how to use `simple.App` is to use `grafana-app-sdk project component add operator`, or to 
[follow the tutorial](tutorials/issue-tracker/README.md), which will have you build out a fully-featured `simple.App` 
with a watcher, validation, and mutation.

## App Provider

In order for an app to be run by a runner, you need something which will provide a manifest to the runner, 
and instantiate a new instance of your app once the runner has loaded the configuration necessary for running it. 
In this case, this is defined by [app.Provider](../app/app.go#L71):
```go
// Provider represents a type which can provide an app manifest, and create a new App when given a configuration.
// It should be used by runners to determine an app's capabilities and create an instance of the app to run.
type Provider interface {
	// Manifest returns a Manifest, which may contain ManifestData or may point to a location where ManifestData can be fetched from.
	// The runner should use the ManifestData to determine app capabilities.
	Manifest() Manifest
	// SpecificConfig is any app-specific config that cannot be loaded by the runner that should be provided in NewApp
	SpecificConfig() SpecificConfig
	// NewApp creates a new App instance using the provided config, or returns an error if an App cannot be instantiated.
	NewApp(Config) (App, error)
}
```

Luckily, the `simple` package has a simple implementation of this, with `simple.AppProvider`. 
If you use `grafana-app-sdk project component add operator`, this is automatically generated for you.

## App Runner

Finally, we need to run our app. As mentioned previously, the only currently supported runner in the `grafana-app-sdk` 
is a standalone kubernetes operator. However, because the `app.App` and `app.Provider` interfaces are what are used to run an app, 
app runners can be build outside of the `grafana-app-sdk` which will run any valid app. 

In this case, though, to run as a standalone kubernetes operator, we use the `operator.Runner` type 
(instantiated with `operator.NewRunner`). This runner takes configuration necessary for running as a standalone operator
(kubeconfig, webhook configuration, etc.), and then runs any apps given to its `Run` method via their `AppProvider`.

## An Example

As an example, let's build a quick, simple app that runs as a standalone operator, assuming we have a kind already generated. 
You can quickly generate a kind and manifest to follow this code with:
```bash
mkdir test-app && cd test-app && grafana-app-sdk project init test-app && grafana-app-sdk project kind add MyKind && grafana-app-sdk generate
```
Now, for simplicity, we'll put everything in a `main.go` file:

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/simple"
	"k8s.io/client-go/tools/clientcmd"

	"test-app/pkg/generated"
	mykind "test-app/pkg/generated/mykind/v1"
)

func main() {
	// Configure the default logger to use slog
	logging.DefaultLogger = logging.NewSLogLogger(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Load the kube config from a file
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", "local.kubeconfig")
	if err != nil {
		panic(err)
	}
	// Build the runner
	runner, err := operator.NewRunner(operator.RunnerConfig{
		KubeConfig: *kubeConfig,
	})
	if err != nil {
		panic(err)
	}

	// Run the app
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	err = runner.Run(ctx, simple.NewAppProvider(generated.LocalManifest(), nil, NewApp))
}

func NewApp(cfg app.Config) (app.App, error) {
	return simple.NewApp(simple.AppConfig{
		Name:       "my-test-app",
		KubeConfig: cfg.KubeConfig,
		ManagedKinds: []simple.AppManagedKind{{
			Kind: mykind.Kind(),
			Reconciler: &operator.TypedReconciler[*mykind.MyKind]{
				ReconcileFunc: func(ctx context.Context, req operator.TypedReconcileRequest[*mykind.MyKind]) (operator.ReconcileResult, error) {
					logging.FromContext(ctx).Info("Reconcile request", "name", req.Object.GetName(), "action", operator.ResourceActionFromReconcileAction(req.Action))
					return operator.ReconcileResult{}, nil
				},
			},
		}},
	})
}
```
You can connect this to a kubernetes API server by changing `"local.kubeconfig"` (or populating that file), 
and making sure the `MyKind` CRD exists by applying the generated `definitions/mykind.testapp.ext.grafana.com.json` file.

You can expand on this to add Validation or Mutation (or conversion), but keep in mind you'll need to add a WebhookConfig 
to the `operator.RunnerConfig`, and set up the webhook configurations in your kubernetes API server. 
Unless you're already familiar with how to set up webhooks, the easier way to do this is to 
[follow the tutorial](tutorials/issue-tracker/README.md) which [sets up validation and mutation in part 8](tutorials/issue-tracker/08-adding-admission-control.md).
