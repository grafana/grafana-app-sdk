# Operators and Event-Based Design

The SDK embraces the event-driven design of kubernetes operators. 
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
* **Informers**, which are given a particular kind and will notify the controller on changes - when resources change, Watchers and Reconcilers will be triggered, performing the according actions;
* **Watchers**, which subscribe to changes for a particular kind and will be notified about any changes from a relevant Informer. Multiple Watchers can watch the same resource kind, and when a change occurs, they will be called in the order they were added to the controller.;
* **Reconcilers**, which subscribe to changes in the state of a particular kind and will be notified about any changes from a relevant Informer, its objective is to ensure that the current state of resources matches the desired state. Multiple Reconcilers can watch the same resource kind, and when a change occurs, they will be called in the order they were added to the controller.

A Watcher has three hooks for reacting to changes: `Add`, `Update`, and `Delete`. 
When the relevant change occurs for the resource they watch, the appropriate hook is called. 
The SDK also offers an _Opinionated_ watcher, called `operator.OpinionatedWatcher` (used by default in `simple.App`). 
This watcher adds some internal finalizer logic to make sure events cannot be missed during operator downtime, 
and adds a fourth hook: `Sync`, which is called when a resource _may_ have been changed during operator downtime, 
but there isn't a way to be sure (with a vanilla Watcher, these events would be called as `Add`).

A Reconciler has its reconciling logic described under the `Reconcile` function.
The `Reconcile` flow allows for explicit failure (returning an error), which uses the normal retry policy of the `operator.InformerController`, or supplying a `RetryAfter` time in response explicitly telling the `operator.InformerController` to try this exact same Reconcile action again after the request interval has passed.
As for the reconciler, the SDK also offers an _Opinionated_ reconciler, called `operator.OpinionatedReconciler`, and adds some internal finalizer logic to make sure events cannot be missed during operator downtime.

Please note that it's enough to specify a Watcher or a Reconciler for a resource. The choice between the two depends on operator needs. 

## Delta Sources

An Informer is the operator's _delta source_: where the add/update/delete events it dispatches come from. By default
that is a kubernetes watch, backed by a cache of every object it has seen. The SDK ships three suppliers, selected per
app with `AppInformerConfig.InformerSupplier` or per kind with `ReconcileOptions.InformerSupplier`:

* `simple.DefaultInformerSupplier` — a watch-backed `operator.KubernetesBasedInformer`;
* `simple.OptimizedInformerSupplier` — a watch-backed `operator.CustomCacheInformer`, for large object counts;
* `simple.BusInformerSupplier` — an `operator.BusInformer`, driven by a `resourcebus.Subscriber` instead of a watch.

### The bus informer

A `BusInformer` takes its deltas from a bus the storage layer publishes change notifications to, rather than from a
watch. It keeps no per-object cache a reader can serve from. Two things follow:

* **A re-list is what makes it correct, not just live.** On a fixed cadence (5 minutes by default, with 10% jitter)
  the informer lists the whole kind and diffs the result against its previous snapshot: a key seen for the first time
  is an add, a key seen before is an update, and a key which has vanished is a delete carrying its last-known state.
  An event which was published while the subscription was down, routed to another replica of a shared group, or never
  announced at all is caught there. The cadence is therefore an upper bound on how stale a replica can be, not a
  tuning nicety.
* **The live stream decides latency, not coverage.** `BusInformerConfig.SharedGroup` joins the app's replicas into a
  group across which each event is delivered once rather than to all of them. That reduces live volume per replica; it
  does not reduce correctness, because the re-list covers what the replica did not see live.

#### Payload

`BusInformerConfig.Payload` decides what a live event delivers:

* `operator.PayloadIdentity` (the default) delivers the kind's zero value with namespace, name and resource version
  set, and nothing else. It costs no API request per event and suits a reconciler which treats the event as a signal
  and reads the object itself. It **cannot** be combined with an opinionated reconciler or watcher — see
  [Writing a Reconciler](writing-a-reconciler.md#identity-payloads-and-opinionated-reconcilers).
* `operator.PayloadHydrated` reads the object from the API before dispatching, so watchers receive a full object as
  they would from a watch, at one request per event. An event whose object cannot be read is dropped and reported to
  the error handler; the delete arrives as its own event or on the next re-list.

#### Listing

`BusInformerConfig.ListerFactory` replaces the list the informer re-lists with. The default is a paginated list of
full objects over the kind's client, which follows continue tokens to the end of the set and fails the whole re-list
on any error: a truncated snapshot does not read as stale, it diffs as a mass deletion.

A server which can return identities alone — namespace, name and resource version, which is all the diff keys off and
all a reconcile needs to re-fetch the object — supplies a `ResourceLister` which does that instead, and the informer
is none the wiser. `informer_bus_relist_projection_total` reports which of the two served each re-list, so a
fallback is visible. With `PayloadHydrated` the lister must return full objects, or re-list deliveries would carry
less than live ones.

#### Resource versions

The bus contract carries resource versions as opaque strings, as the kubernetes API contract requires. The SDK
assumes they are numeric in exactly two places: the snapshot's arbitration between a re-list and the live writes that
raced it, and the delivery latency histograms. A version which does not parse reads as zero, which disables the
arbitration — a live delete racing a re-list then produces a spurious add/delete pair, corrected on the next re-list —
and records no latency. Supply `BusInformerConfig.ResourceVersionTime` to decode the timestamp a version embeds;
without it the histograms stay empty and the counters are unaffected.

### The periodic informer

`operator.PeriodicInformer` has no watch and no subscription, and keeps no snapshot: each pass lists the kind and
delivers every object as an add. It suits idempotent, cadence-driven work which re-evaluates every object on each
pass regardless of whether it changed — age-based cleanup, for instance. For anything which should react to a change
promptly, use one of the informers above.

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
