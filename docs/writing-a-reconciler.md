# Writing a Reconciler (or Watcher)

A Reconciler is a logical pattern which takes the state of a set of objects in the platform API, and propagates changes to other components based on that state. 
This logic is asynchronous, and works by having a function which is called when a change occurs in the set of resources the reconciler is watching, 
which provides the current state of the object, and the type of change that occurred (Create, Update, Delete). 

## Reconciler vs Watcher

Both reconcilers and watchers are used for the reconciliation process. 
Whether you use one or the other is down to preference, and use-case. 
Both reconcilers and watchers are powered by the same informer design within an `InformerController`, with just slightly different handling logic. 
They both have an `Opinionated` variant that can wrap the interface as well. Note: if using the `Opinionated` variant, it will add a [finalizer](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/) to the resource you are watching. If you are watching a resource not created by your app, please reach out to the owners prior to proceeding with this approach.

The major difference between a `Reconciler` and a `Watcher` is that a `Reconciler` has a single function, `Reconcile`, which is called for every event for a kind, while a `Watcher` has a function for each event type (`Add`, `Update`, `Delete`). There are more minor differences in how these events are handled as well:
* A `Reconciler` can return a response with an explicit "retry after this time period" message, while a `Watcher` will only return success/fail (`nil` or `error`).
* A `Watcher` will give you the previous state of the resource on an update event, while a Reconcile event will not.
* A `Reconciler` can pass state between retries in-memory if storing state in the API is failing, however, this will only persist until the operator is restarted and should not be relied upon excepting situations where the API cannot be reached.

## Considerations When Writing a Reconciler

When writing a reconciler, it's important to take a few things into consideration:
* If you make an update to the object you're doing the reconcile (or watch) event for, this will trigger _another_ reconcile (or watch) event. Generally, favor only updating subresources (specifically `status`) and some metadata in your reconcile (or watch) events, as a `status` update should not trigger the `metadata.generation` value to increase (only `metadata.resourceVersion`), which will allow you to filter events out. Using the `operator.OpinionatedWatcher` will filter these events for you, but you will need to track this yourself in a Reconciler; if you prefer not to use OpinionatedWatcher or want to do your own event filtering, keep in mind how updates within your reconcile loop will be received.
* The reconciler is taking action on _every_ consumed event. Finding ways to escape from a reconcile or watcher event early will help your overall program logic.
* All objects for the kind(s) you are watching are cached to memory by default. This can be customized by using a different informer implementation, such as `operator.CustomCacheInformer`. Custom informers can be used in `simple.App` with `AppConfig.InformerConfig.InformerSupplier`, or by using your own custom `app.App` implementation. See [Declaring What a Reconciler Needs From a Watch](#declaring-what-a-reconciler-needs-from-a-watch-experimental) for having the app choose each kind's watch mechanism from what its handlers need.
* Don't rely on retries to track operator state; use the `status` subresource to track operator success/failure, so that your operator can work out state from a fresh start (a restart will remove all pending retries, which are stored purely in-memory). This also allows a user to track operator status by viewing the `status` subresource.
* If your reconcile process makes requests for other resources, consider caching, as high-traffic objects may cause your application to have to make these requests extremely frequently.
* If your operator has a watcher or reconciler that updates the resource in a deterministic way (such as adding a label based on the spec), consider adding mutation for the kind on your App instead, as it makes that process synchronous and will never leave the object in an intermediate state (and reduces API calls from your operator). Mutation can be added for a kind in `simple.App` with `AppConfig.ManagedKinds[].Mutator`, or by implementing the behavior in `Mutate` if you're using a custom `app.App` implementation (don't forget to add mutation in your manifest as well).
* When you have multiple versions of a kind, your reconciliation should only deal with one of them (typically the latest), as events are always issued for any version as the version requested by the operator's watch (so a user creating a `v1` version of a resource will still produce a `v2` version of that resource in a watch request for the `v2` of the kind).
* The default conversion between versions is roughly equivalent to running `json.Marshal` on the stored version and then `json.Unmarshal` into the requested version. If this is not good enough for your purposes, add conversion to your app (for `simple.App`, use `AppConfig.Converters`, or implement `Convert` if you're implementing `app.App` yourself. Don't forget to add conversion in your manifest as well).

## An Example Reconciler

Let's consider an example reconciler for a kind defined by the CUE:
```cue
{
	kind: "MyKind"
	versions: {
		"v1": {
			schema: {
				spec: {
					someInfo: string
					otherInfo: string
				}
				status: {
					lastAppliedGeneration: int
				}
			}
		}
	}
}
```
We want to build a reconciler that will send `someInfo` and `otherInfo` to some other system, but we only need to do this if `someInfo` or `otherInfo` change. 
Since there are ways a resource can change without the contents of `spec` being altered, we need a way to track if the current `spec` has been applied. 
To do this here, we track `lastAppliedGeneration` in the `status`. `generation` is an object metadata property which increments when the spec changes, 
so we can use it to check if a given request has a different version of the spec than one we've already processed. 
```go
func NewMyKindReconciler(infoClient InfoClient, store *resource.TypedStore[*v1.MyKind]) operator.Reconciler {
	// operator.TypedReconciler implements operator.Reconciler but calls ReconcilerFunc with an operator.TypedReeconcileRequest 
	// rather than an operator.ReconcileRequest, avoiding the need to cast resource.Object into our go type. 
	// We could also have a struct which implements operator.Reconciler, but this is easier for simple things.
	return &operator.TypedReconciler[*v1.MyKind]{ 
		ReconcileFunc: func(ctx context.Context, req operator.TypedReconcileRequest[*v1.MyKind]) (operator.ReconcileResult, error) {
			logging.FromContext(ctx).Info("Reconcile request", "name", req.Object.GetName(), "action", operator.ResourceActionFromReconcile(req.Action), "generation", req.Object.GetGeneration())
			// If we're deleting the object, tell InfoClient
			if req.Action == operator.ReconcileActionDeleted {
				err := infoClient.Delete(req.Object.GetNamespace(), req.Object.GetName())
				return operator.ReconcileResult{}, err
			}
			// If the last applied generation matches the current generation of the resource, we can ignore this reconcile request
			if req.Object.GetGeneration() == req.Object.Status.LastAppliedGeneration {
				return operator.ReconcileResult{}, nil
			}
			// Attempt to apply the state to the third-party service
			err := infoClient.ApplyInfo(req.Object.GetNamespace(), req.Object.GetName(), req.Object.Spec.SomeInfo, req.Object.Spec.OtherInfo)
			if err != nil {
				// Check the error, if it's retryable, tell the controller to try again in a bit
				if IsRetryable(err) {
					return operator.ReconcileResult{
						RequeueAfter: time.Minute,
					}, nil
				}
				// Otherwise, return the error. The controllers RetryPolicy will dictate if it should be retried, and after how long
				return operator.ReconcileResult{}, err
			}
			// Set status.lastAppliedGeneration
			req.Object.Status.LastAppliedGeneration = req.Object.GetGeneration()
			_, err = store.UpdateSubresource(ctx, req.Object.GetStaticMetadata().Identifier(), resource.SubresourceStatus, req.Object)
			if err != nil {
				return operator.ReconcileResult{}, err
			}
			return operator.ReconcileResult{}, nil
		}
	}   
}
```
We could write a similar `Watcher`, with the caveat being that this function becomes split and duplicated amongst the watcher's methods for each action, 
and we can't control requeue behavior from the watcher response (we have to leave it up to the controller's `RetryPolicy`). 
A watcher version of this would look like:
```go
func NewMyKindReconciler(infoClient InfoClient, store *resource.TypedStore[*v1.MyKind]) operator.Reconciler {
	// simple.Watcher implements operator.Watcher and calls the defined functions for each event. 
	// We could also have a struct which implements operator.Watcher, but this is easier for simple things.
	return &simple.Watcher{ 
		AddFunc: func(ctx context.Context, obj resource.Object) error {
			logging.FromContext(ctx).Info("Add event", "name", obj.GetName(), "action", "add", "generation", obj.GetGeneration())
			// Cast the object
			mykind, ok := obj.(*v1.MyKind)
			if !ok {
				return fmt.Errorf("unable to cast object into *v1.MyKind")
			}
			// We still need to check the lastAppliedGeneration, as an add event can be called on operator startup, 
			// as the application doesn't have the state to know if the resource already existed.
			if mykind.GetGeneration() == mykind.Status.LastAppliedGeneration {
				return nil
			}
			// Attempt to apply the state to the third-party service
			err := infoClient.ApplyInfo(mykind.GetNamespace(), mykind.GetName(), mykind.Spec.SomeInfo, mykind.Spec.OtherInfo)
			if err != nil {
				// Return the error. The controller's RetryPolicy will dictate if it should be retried, and after how long
				return err
			}
			// Set status.lastAppliedGeneration
			mykind.Status.LastAppliedGeneration = mykind.GetGeneration()
			_, err = store.UpdateSubresource(ctx, mykind.GetStaticMetadata().Identifier(), resource.SubresourceStatus, mykind)
			if err != nil {
				return err
			}
			return nil
		}
		UpdateFunc: func(ctx context.Context, oldObj resource.Object, newObj resource.Object) error {
			logging.FromContext(ctx).Info("Update event", "name", obj.GetName(), "action", "update", "generation", obj.GetGeneration())
			// Cast the object
			mykind, ok := newObj.(*v1.MyKind)
			if !ok {
				return fmt.Errorf("unable to cast object into *v1.MyKind")
			}
			// If the last applied generation matches the current generation of the resource, we can ignore this update
			if mykind.GetGeneration() == mykind.Status.LastAppliedGeneration {
				return nil
			}
			// Attempt to apply the state to the third-party service
			err := infoClient.ApplyInfo(mykind.GetNamespace(), mykind.GetName(), mykind.Spec.SomeInfo, mykind.Spec.OtherInfo)
			if err != nil {
				// Return the error. The controller's RetryPolicy will dictate if it should be retried, and after how long
				return err
			}
			// Set status.lastAppliedGeneration
			mykind.Status.LastAppliedGeneration = mykind.GetGeneration()
			_, err = store.UpdateSubresource(ctx, mykind.GetStaticMetadata().Identifier(), resource.SubresourceStatus, mykind)
			if err != nil {
				return err
			}
			return nil
		}
		DeleteFunc: func(ctx context.Context, obj resource.Object) error {
			logging.FromContext(ctx).Info("Delete event", "name", obj.GetName(), "action", "delete", "generation", obj.GetGeneration())
			err := infoClient.Delete(req.Object.GetNamespace(), req.Object.GetName())
			return operator.ReconcileResult{}, err
		}
	}   
}
```

## Declaring What a Reconciler Needs From a Watch (experimental)

> [!WARNING]
> `WatchOptions` and everything which reads it are experimental, and may change in a minor release.

By default, every kind is watched by the API server: your handlers are delivered each change as it happens (`Live`),
carrying the whole object (`WatchDetailFullObject`), to every instance of the app (`WatchDistributionAll`). That is
the zero value of every field below, so a kind which declares nothing keeps exactly this.

Some reconcilers use less. One which re-reads the object it acts on does not need it delivered; one which
re-evaluates everything hourly does not need changes at all.

A kind says so in `ReconcileOptions.WatchNeeds`. Cheaper ways of watching are deployment wiring, so declaring less
never changes what a kind gets unless the deployment has something cheaper to give it.

### What you declare

```go
operator.WatchOptions{
    // What the handlers need of deliveries made as changes happen. Nil needs no live channel.
    Live: &operator.WatchLiveOptions{Detail: ..., Distribution: ...},
    // What they need of every object being re-delivered on a schedule. Nil needs no resync.
    Resync: &operator.WatchResyncOptions{Detail: ..., Distribution: ..., Interval: ..., ListPageSize: ...},
}
```

Setting a channel is what says you need it, which makes `Live` and `Resync` the one place leaving a field unset
*widens* what is acceptable: a nil channel is not compared at all. So a kind which wants a resync **as well as**
live changes sets both — `WatchOptions{Resync: ...}` on its own says nothing is needed live, and a mechanism which
only re-delivers everything on a schedule satisfies it. Declaring nothing at all is the exception: a `WatchOptions`
with neither channel needs the live one.

Within a channel, every field's zero value is the strictest thing it could say — whole objects, undivided — so
leaving one of those unset never widens anything.

`Detail` — the least a delivery must carry:

| Value | Declare it when |
| --- | --- |
| `WatchDetailFullObject` (zero) | your handlers read the object off the event |
| `WatchDetailMetadata` | they decide from metadata: labels, finalizers, timestamps |
| `WatchDetailIdentity` | the key alone is enough, because they re-read whatever they act on |

`Distribution` — how far the work may be divided across the app's instances:

| Value | Declare it when |
| --- | --- |
| `WatchDistributionAll` (zero) | every instance must see everything, because your handlers keep state of their own |
| `WatchDistributionShared` | the work is idempotent and done once, so you permit dividing it |
| `WatchDistributionSharded` | you require that each object always goes to the same instance |

`Shared` and `Sharded` read in opposite directions: `Shared` *permits* the work being divided, so anything satisfies
it, while `Sharded` *requires* affinity and only a sharded mechanism provides it.

`Resync.Interval` and `Resync.ListPageSize` say how often the resync should run and how large a page anything it
lists should read. Zero takes the mechanism's own.

### How a mechanism is chosen

The deployment lists what it has, ordered by increasing capability. Each kind gets the first mechanism which
satisfies its declaration, so each takes the cheapest thing which serves it.

```go
simple.AppConfig{
    InformerConfig: simple.AppInformerConfig{
        InformerSupplier: operator.NewSelectingInformerSupplier(
            simple.DefaultInformerSupplier, // what a kind none of them satisfies is watched by
            myReListMechanism,              // no live channel: only re-delivers everything on a schedule
            myNotifyOnlyMechanism,          // live metadata, and no resync
            myEventBusWithResyncMechanism,  // both, at the cost of a subscription and a re-list
        ),
    },
    ManagedKinds: []simple.AppManagedKind{{
        Kind:       myCleanupKind.Kind(),
        Reconciler: &myCleanupReconciler,
        ReconcileOptions: simple.BasicReconcileOptions{
            // Needs nothing as changes happen, so it takes the re-list: the first mechanism which satisfies it.
            WatchNeeds: operator.WatchOptions{
                Resync: &operator.WatchResyncOptions{Interval: time.Hour, ListPageSize: 5000},
            },
            UsePlain: true,
        },
    }},
}
```

Anything no mechanism satisfies is watched, and an app which lists nothing watches everything.

`Interval` and `ListPageSize` take no part in that choice: a mechanism takes a cadence rather than offering one, so
the one a kind gets may ignore them. The API watch ignores the page size, since its resync re-delivers its cache
rather than listing — which is also why that resync re-evaluates every object but discovers nothing the live stream
missed, where a mechanism which lists the API recovers as well.

### What the app refuses

A declaration nothing can serve fails when the app is constructed, naming the way out. The API watch cannot resync
without a cadence and cannot shard, so those are the declarations to be careful with. `WatchDistributionShared` is
not one of them: it permits division rather than requiring it, so a kind declaring it where nothing divides anything
is simply watched. A leader-elected deployment, where one replica is active anyway, needs no declaration at all.

The `Opinionated` variants read the deletion timestamp, the finalizer list and the generation. All three are
metadata, so `WatchDetailMetadata` serves them and `WatchDetailIdentity` does not: declaring identities alongside one
is refused, as is a mechanism carrying them on any channel it delivers on. Set `UsePlain` (or `UseOpinionated: false`
for an unmanaged kind) to be served by identities, and handle deletion in your own reconcile.

Two entries watching the same kind and query share one set of deliveries, so what one entry's informer carries
reaches the other's handlers too. A pair where that would hand a handler less detail than it declared fails at
construction; informers with complementary channels are fine, since each entry still has its own for what it asked
for.

Two things the SDK cannot check for you:

* Your own handler reading `obj.Spec` off a metadata-only delivery. That contract is yours to keep, like a label
  selector.
* A `Watcher` reading the previous object on an update, which a mechanism keeping no object body cannot hand it.
  Prefer a `Reconciler` for a kind declaring less than the whole object.

Finally, a need is about the deliveries, not the informer behind them. Whether it keeps a cache decides its memory,
but no handler reads that cache through `operator.Informer`.

`simple.Operator` takes the same declaration, with `simple.WithWatchNeeds` on `WatchKind`/`ReconcileKind`.

### Serving a Resync With a Re-List (experimental)

`operator.NewPeriodicMechanism` is the re-list: no watch, no subscription, no cache. Each pass lists the kind and
delivers every object as an add, unchanged ones included, which is what re-triggers the work.

```go
simple.AppConfig{
    InformerConfig: simple.AppInformerConfig{
        InformerSupplier: operator.NewSelectingInformerSupplier(
            simple.DefaultInformerSupplier, // the API watch, for anything the mechanisms cannot serve
            // Least capable first, so the re-list takes the kinds which only resync.
            operator.NewPeriodicMechanism(operator.PeriodicMechanismOptions{
                // Fallbacks, applied last: a kind's own figures win, and so does an app-wide
                // InformerOptions.CacheResyncInterval or WatchListPageSize.
                ResyncInterval: 15 * time.Minute,
                PageSize:       1000,
                RetryInterval:  10 * time.Second, // how often a failed or deferred pass is retried
            }),
            myEventBusMechanism,
        ),
    },
    ManagedKinds: []simple.AppManagedKind{{
        Kind:       myCleanupKind.Kind(),
        Reconciler: &myCleanupReconciler,
        ReconcileOptions: simple.BasicReconcileOptions{
            // What to list: the server applies these, and the re-list is the only source, so nothing re-applies them.
            Namespace:      "default",
            LabelFilters:   []string{"app=myapp"},
            FieldSelectors: []string{"spec.state=pending"},

            // What the handlers need: no live channel, so the re-list satisfies this and the bus does not.
            WatchNeeds: operator.WatchOptions{
                Resync: &operator.WatchResyncOptions{
                    Interval:     time.Hour, // this kind re-evaluates hourly, not every 15 minutes
                    ListPageSize: 5000,      // and reads its larger set in larger pages
                },
            },
            UsePlain: true, // no finalizer, so nothing to clean up on delete; see below
        },
    }},
}
```

The kind declares a `Resync` and no `Live`, so the re-list satisfies it and is offered first; a kind needing the
live channel passes over the re-list and takes the bus, or is watched when no mechanism serves it. A kind handed
this mechanism outright while needing a live channel fails when the app is constructed.

`Interval` and `ListPageSize` are requests rather than requirements: they take no part in choosing the mechanism,
and `PeriodicMechanismOptions` answers for whichever a kind leaves unset — so the kind above resyncs hourly in
5000-object pages while a kind declaring only `Resync: &operator.WatchResyncOptions{}` takes the mechanism's 15
minutes and 1000. Unlike the watch's resync this one reads the API, so it recovers as well as re-evaluates, and a
page size means something to it.

Only `Add` is ever called: without a snapshot an update and a delete are indistinguishable from a re-delivery, so
handlers re-evaluate an object from the object and treat one which has gone away as nothing to do.

A pass does not start while the previous one's deliveries are still queued: it is retried every `RetryInterval`
until they drain, so handlers slower than the cadence run less often instead of building a backlog. Retries a
handler schedules itself with `RequeueAfter` are outside that queue, so prefer letting the next pass be the retry.
`InformerOptions.EventTimeout` does not bound handlers here, as it does not under the stock informers while they
run.

**Deletion needs care.** This informer never emits `Delete`; a removed object simply stops appearing in the list, so
nothing derived from events can notice it went. What does work is a finalizer: the `Opinionated` variants' `Add`
handles an object which is pending deletion — running the delete handling and removing the finalizer, the path they
already use for a resync — because such an object is still listed until its finalizers clear. Three consequences:

* Cleanup is **cadence-delayed**. A deleted object sits in `Terminating` until the next pass reads its deletion
  timestamp, so an hourly kind can take an hour to finalize.
* A **selector can strand it**. An object holding the finalizer which then stops matching `LabelFilters` or
  `FieldSelectors` disappears from the list, so nothing removes the finalizer and the deletion never completes.
  Removing it then needs a delivery path which still sees the object — a second informer without the selectors, or a
  sweep of your own.
* **An object created and deleted between two passes is never seen at all**, finalizer or not, since it was gone
  before anything could attach one.

So `UsePlain` is not a way to handle deletion yourself — it opts out of the finalizer, which is the only thing that
makes an external deletion observable here. Use it when the work needs no cleanup on delete, as the example above
does, and reconcile from the listed set rather than from events when it does.

A server which lists some other way is reached by building the informer yourself: `operator.NewPeriodicInformer`
takes any `ResourceLister`, and `operator.NewInformerMechanism` pairs it with what it provides.
