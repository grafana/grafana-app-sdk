# Operators and Event-Based Design

The SDK embraces the event-driven design of kubernetes operators, without explicit reliance on kubernetes as machinery. 
Simply put, an operator is an application or process that watches for changes to some resource or set of resources. 
When changes occur, it may take some action based on those changes.

## In the SDK

The SDK offers a simple way to create the operator pattern with the `operator` package. 
While a lot of code in this package has a rather explicit kubernetes reliance (ingesting a kube config or client), 
this will eventually be removed in favor of an abstraction that will produce the appropriate client(s) to talk to the underlying storage layer. 

An operator consists, broadly, of collections of runnable controllers, one type of which is defined in the SDK, 
but a user can easily extend this by having a new controller which implements the `operator.Controller` interface.

The controller offered by the SDK is the `operator.InformerController`, which is a controller that is composed of two sets of objects:
* **Informers**, which are given a particular CRD and will notify the controller on changes;
* **Watchers**, which subscribe to changes for a particular CRD kind and will be notified about any changes from a relevant Informer. Multiple Watchers can watch the same resource kind, and when a change occurs, they will be called in the order they were added to the controller.;
* **Reconcilers**, which subscribe to changes in the state of a particular CRD kind and will be noticied about any changes from a relevant Informer, its objective is to ensure that the current state of resources matches the desired state. Multiple Reconcilers can watch the same resource kind, and when a change occurs, they will be called in the order they were added to the controller.

A Watcher has three hooks for reacting to changes: `Add`, `Update`, and `Delete`. 
When the relevant change occurs for the resource they watch, the appropriate hook is called. 
The SDK also offers an _Opinionated_ watcher, designed for kubernetes-like storage layers, called `operator.OpinionatedWatcher`. 
This watcher adds some internal finalizer logic to make sure events cannot be missed during operator downtime, 
and adds a fourth hook: `Sync`, which is called when a resource _may_ have been changed during operator downtime, 
but there isn't a way to be sure (with a vanilla Watcher in a kubernetes-like environment, these events would be called as `Add`).

A Reconciler has its reconciling logic described under the `Reconcile` function.
The `Reconcile` flow allows for explicit failure (returning an error), which uses the normal retry policy of the `operator.InformerController`, or supplying a `RetryAfter` time in response explicitly telling the `operator.InformerController` to try this exact same Reconcile action again after the request interval has passed.
As for the watcher, the SDK also offers an _Opinionated_ reconciler, designed for kubernetes-like storage layers, called `operator.OpinionatedReconciler`, and adds some internal finalizer logic to make sure events cannot be missed during operator downtime.

## Event-Based Design

What this all means is that development using the SDK is geared toward an event-based design. 
Resources get added/updated/deleted by the API, and then your operator can pick up that change and take on any complex business logic related to it. 
This means moving complex business logic out of the API calls into a more asynchronous pattern, 
where a call to the API will kick off a workflow, rather than start the workflow, wait for completion, then return.

While the SDK is geared toward this type of design, you can still put all of your business logic in your API endpoints 
if you either need to have calls be completely synchronous, or the business logic is very simple.

**Forthcoming:** the SDK codegen utility offers the ability to create a simple CRUDL backend plugin API 
and an operator template given only one or more Schemas defined in CUE (see [Schema Management](schema_management.md)). 

## A Simple Operator

Let's walk through the creation of a simple operator:

```golang
package main

import (
	"context"
	"fmt"
	
	"github.com/grafana/grafana-app-sdk/simple"
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/runtime"
)

func main() {
	// Obtain kube config (not real code)
	kubeConfig := getKubeConfig()
	
	// Create a new operator
	op, err := simple.NewOperator(simple.OperatorConfig{
		Name:       "issue-operator",
		KubeConfig: kubeConfig.RestConfig,
		Metrics: simple.MetricsConfig{
			Enabled: true,
		},
		Tracing: simple.TracingConfig{
			Enabled: true,
			OpenTelemetryConfig: simple.OpenTelemetryConfig{
				Host:        cfg.OTelConfig.Host,
				Port:        cfg.OTelConfig.Port,
				ConnType:    simple.OTelConnType(cfg.OTelConfig.ConnType),
				ServiceName: cfg.OTelConfig.ServiceName,
			},
		},
		ErrorHandler: func(ctx context.Context, err error) {
			logging.FromContext(ctx).Error(err.Error())
		},
	})

	// Create a watcher which prints some lines when a resource is added
	watcher := simple.Watcher{
		AddFunc: func(ctx context.Context, object resource.Object) error {
			fmt.Println("Hey, a resource got added!")
			return nil
    	},
    }

	// Let the operator use given watcher for the 'MyResource' kind
	op.WatchKind(MyResource.Schema(), watcher, simple.ListWatchOptions{
		Namespace: "default",
	})

	stopCh := make(chan struct{}, 1) // Close this channel to stop the operator
	op.Run(stopCh)
}
```

Note that this is not the only way to run an operator. In fact, operators, being just a call to `Run()` on the operator object, 
can be run as part of a back-end plugin alongside your API instead of as standalone applications.

For more details, see the [Operator Examples](../examples/operator) or the [Operator Package README](../operator/README.md).
