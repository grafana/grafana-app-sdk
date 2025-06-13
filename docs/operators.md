# Operators and Event-Based Design

The SDK embraces the event-driven design of kubernetes operators, without explicit reliance on kubernetes as machinery. 
Simply put, an operator is an application or process that watches for changes to some resource or set of resources. 
When changes occur, it may take some action based on those changes.

## In the SDK

The operator patterns are implemented in the SDK's `operator` package. 
However, they are easiest to use as part of `simple.App`, 
which wraps the more complex logic of an operator and requires only that you write your reconciler(s) or watcher(s), 
which react to change events in the resources your app cares about.

Within the `operator` package, an operator consists, broadly, of collections of runnable controllers, one type of which is defined in the SDK, 
but a user can easily extend this by having a new controller which implements the `operator.Controller` interface.

The controller offered by the SDK is the `operator.InformerController`, which is a controller that is composed of three sets of objects:
* **Informers**, which are given a particular CRD and will notify the controller on changes - when resources change, Watchers and Reconcilers will be triggered, performing the according actions;
* **Watchers**, which subscribe to changes for a particular CRD kind and will be notified about any changes from a relevant Informer. Multiple Watchers can watch the same resource kind, and when a change occurs, they will be called in the order they were added to the controller.;
* **Reconcilers**, which subscribe to changes in the state of a particular CRD kind and will be notified about any changes from a relevant Informer, its objective is to ensure that the current state of resources matches the desired state. Multiple Reconcilers can watch the same resource kind, and when a change occurs, they will be called in the order they were added to the controller.

A Watcher has three hooks for reacting to changes: `Add`, `Update`, and `Delete`. 
When the relevant change occurs for the resource they watch, the appropriate hook is called. 
The SDK also offers an _Opinionated_ watcher, designed for kubernetes-like storage layers, called `operator.OpinionatedWatcher`. 
This watcher adds some internal finalizer logic to make sure events cannot be missed during operator downtime, 
and adds a fourth hook: `Sync`, which is called when a resource _may_ have been changed during operator downtime, 
but there isn't a way to be sure (with a vanilla Watcher in a kubernetes-like environment, these events would be called as `Add`).

A Reconciler has its reconciling logic described under the `Reconcile` function.
The `Reconcile` flow allows for explicit failure (returning an error), which uses the normal retry policy of the `operator.InformerController`, or supplying a `RetryAfter` time in response explicitly telling the `operator.InformerController` to try this exact same Reconcile action again after the request interval has passed.
As for the reconciler, the SDK also offers an _Opinionated_ reconciler, designed for kubernetes-like storage layers, called `operator.OpinionatedReconciler`, and adds some internal finalizer logic to make sure events cannot be missed during operator downtime.

Please note that it's enough to specify a Watcher or a Reconciler for a resource. The choice between the two depends on operator needs. 

## Event-Based Design

What this all means is that development using the SDK is geared toward an event-based design. 
Resources get added/updated/deleted by the API, and then your operator can pick up that change and take on any complex business logic related to it. 
This means moving complex business logic out of the API calls into a more asynchronous pattern, 
where a call to the API will kick off a workflow, rather than start the workflow, wait for completion, then return.

## A Simple Operator

Let's walk through the creation of a simple operator, using a Reconciler:

```golang
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/simple"
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/runtime"
)

func main() {
	// Obtain kube config (not real code)
	kubeConfig := getKubeConfig()
	// Get the app manifest (function from generated code)
	manifest := generated.LocalManifest()

	// Create the operator runner
	runner, err := operator.NewRunner(operator.RunnerConfig{
		KubeConfig: kubeConfig,
		MetricsConfig: operator.RunnerMetricsConfig{
			Enabled: true,
        },
    })
	
	// Set up tracing
	simple.SetTraceProvider(simple.OpenTelemetryConfig{
		Host:        cfg.OTelConfig.Host,
		Port:        cfg.OTelConfig.Port,
		ConnType:    simple.OTelConnType(cfg.OTelConfig.ConnType),
		ServiceName: cfg.OTelConfig.ServiceName,
	})

	// Create a reconciler which prints some lines when the resource changes
	reconciler := simple.Reconciler{
		ReconcileFunc: func(ctx context.Context, req operator.ReconcileRequest) (operator.ReconcileResult, error) {
			fmt.Printf("Hey, resource state changed! action: %s")

			return operator.ReconcileResult{}, nil
		},
	}
	
	// Create the AppProvider && NewApp function
	provider := simple.NewAppProvider(manifest, nil, func(cfg app.Config) (app.App, error) {
        return simple.NewApp(simple.AppConfig{
			KubeConfig: cfg.KubeConfig,
			ManagedKinds: []simple.AppManagedKind{{
				Kind: mymind.Kind(),
				Reconciler: &reconciler,
            }},
        })
	})
	
	// Run the app as an operator using our operator runner
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	runner.Run(ctx, provider)
}
```

For more details, see [Writing an App](writing-an-app.md), which goes into more details on writing an app, or [Writing a Reconciler](writing-a-reconciler.md) for details on how to write a reconciler. 
There are also the [Operator Examples](../examples/operator), which contain two examples, a [reconciler-based operator](../examples/operator/simple/reconciler/) and a [watcher-based one](../examples/operator/simple/watcher/).
