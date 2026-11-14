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
* All objects for the kind(s) you are watching are cached to memory by default. This can be customized by using a different informer implementation, such as `operator.CustomCacheInformer`. Custom informers can be used in `simple.App` with `AppConfig.InformerConfig.InformerSupplier`, or by using your own custom `app.App` implementation. See [Declaring What a Reconciler Needs From a Watch](#declaring-what-a-reconciler-needs-from-a-watch-experimental) for letting the app pick a cheaper mechanism for the kinds which do not need the watch.
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
	current: "v1"
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

By default a kind is watched by the API server and your handlers are delivered each change, carrying the whole
object, to every instance of the app. That is more than some reconcilers use, and the cheaper ways of watching — an
event bus, a periodic re-list — are not something an app can assume its deployment has.

So a kind declares the least it needs in `ReconcileOptions.WatchOptions`, and the app gives it the most optimal
mechanism available to it — the deployment lists the mechanisms it has, best first, and a kind takes the first which
satisfies it. Which mechanisms those are, and in which order, is deployment wiring. Nothing you declare can leave a
kind unserved: a need none of them satisfies falls back to the API watch, which satisfies every need. And declaring
nothing keeps exactly what a kind has today, even where a cheaper mechanism is listed first — a bus which carries
identities does not satisfy the default need for whole objects.

```go
simple.AppManagedKind{
    Kind:       myCleanupKind.Kind(),
    Reconciler: &myCleanupReconciler,
    ReconcileOptions: simple.BasicReconcileOptions{
        // Nothing needed as changes happen: this reconciler re-evaluates every object hourly.
        WatchOptions: operator.WatchOptions{
            Sweep: &operator.WatchSweepOptions{Every: time.Hour, PageSize: 5000},
        },
        UsePlain: true,
    },
}
```

`WatchOptions` has a section per channel a watch can deliver on, and setting one is what says you need it:

| Field | | Zero value |
| --- | --- | --- |
| `Live` | what the handlers need of deliveries made as changes happen, or `nil` for none | the live channel, with the defaults below |
| `Sweep` | what they need of every object being re-delivered on a schedule, or `nil` for none | no sweep |
| `*.Detail` | `WatchDetailIdentity` when the handlers re-read the object they act on, so the delivery only has to say which object changed | the full object |
| `*.Distribution` | `WatchDistributionShared` when one instance acting is enough, or `WatchDistributionSharded` when it must also always be the same instance per object | all instances get all of it |
| `Sweep.Every` | how often the handlers need the sweep | the mechanism's own cadence |
| `Sweep.PageSize` | how large a page the sweep should read at a time | the mechanism's own page size |

The API watch has both channels, so a kind needing a sweep gets one even where the deployment runs nothing cheaper:
the app sets the watch's cache resync interval from `Sweep.Every`, where it is otherwise one app-wide setting. Note
what that sweep is, though — a resync re-delivers the informer's cache, so it costs nothing and re-evaluates every
object, but discovers nothing the live stream missed. A mechanism which sweeps by listing the API both re-evaluates
and recovers.

Declare `Sweep` and no `Live` when what makes an object actionable is the passage of time rather than a change —
age-based cleanup — since a live stream never fires for an object nobody touched. Declare both when the handlers act
on changes promptly *and* re-evaluate on a cadence. Declare `Detail: WatchDetailIdentity` when a reconcile re-reads
the object anyway, and a `Distribution` when the work is idempotent and done once, so a deployment with a shared
subscription can divide it. Allowing less is always safe; it only ever widens what can serve the kind.

Two things to know:

* The `Opinionated` variants read the deletion timestamp and the finalizer list off the object on the event, so they
  need whole objects whatever you declare. Set `UsePlain` (or `UseOpinionated: false` for an unmanaged kind) to be
  served by identities, and handle deletion in your own reconcile after reading the object.
* A `Watcher` also reads the previous object on an update, which a mechanism keeping no object body cannot hand it.
  Prefer a `Reconciler` for a kind declaring identities.

A need is about the deliveries, not the informer behind them. Whether that informer keeps a cache of the kind decides
its memory, which is most of the reason a deployment wants a cheaper mechanism, but no handler reads that cache
through `operator.Informer` — a handler which needs objects other than the one its event is about reads them through
the API, whatever serves it.

### Serving a Sweep With a Re-List (experimental)

A deployment lists the mechanisms it has, best first, and `simple.PeriodicInformerMechanism` is the re-list among
them: `operator.PeriodicInformer` has no watch and no subscription, keeps no cache, and on each pass lists the kind
and delivers every listed object as an add — unchanged objects included, which is what re-triggers the work.

```go
simple.AppConfig{
    InformerConfig: simple.AppInformerConfig{
        InformerSupplier: simple.NewSelectingInformerSupplier(nil,
            myEventBusMechanism,
            simple.PeriodicInformerMechanism(simple.PeriodicInformerConfig{}),
        ),
    },
}
```

It states for itself that it provides a sweep carrying whole objects to every instance and no live channel, so the
kinds which declared `Sweep` and no `Live` take it and nothing else does. A kind needing the live channel is passed
over here and falls back to the API watch; one handed this mechanism anyway fails when the app is constructed rather
than reconciling a cadence late.

The config above is empty because each kind sweeps at the cadence and page size it asked for in `Sweep.Every` and
`Sweep.PageSize`; `SweepInterval` and `PageSize` are the defaults for kinds which state neither. Only `Add` is ever
called, because without a snapshot the informer cannot tell an update or a delete from a re-delivery — so the
handlers it serves re-evaluate an object from the object, and treat one which has gone away as nothing to do. The
initial list is retried on its own shorter interval, and each cadence is jittered so that informers which started
together do not list together for the rest of the process's life.

`Namespace`, `LabelFilters` and `FieldSelectors` reach the list request, where the server applies them: the list is
the informer's only source, so nothing else has to honour them. A server which can list the kind some other way —
projecting less than whole objects, say — is reached through `ListerFactory`, which returns an
`operator.ResourceLister` for the kind; the default is a paginated list over the kind's client, following continue
tokens to the end of the set.
