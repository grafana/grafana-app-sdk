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
* All objects for the kind(s) you are watching are cached to memory by default. This can be customized by using a different informer implementation, such as `operator.CustomCacheInformer`. Custom informers can be used in `simple.App` with `AppConfig.InformerConfig.InformerSupplier`, or by using your own custom `app.App` implementation. You can also declare what your handlers need from a watch, and let the app choose a watch mechanism for each kind. See [Declaring What a Reconciler Needs From a Watch](#declaring-what-a-reconciler-needs-from-a-watch-experimental).
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
> `WatchOptions` and the code that reads it are experimental. They may change in a minor release.

### Overview

By default, the app watches each kind through the Kubernetes API (the "API watch"). With the API watch, your
handlers (your `Reconciler` or `Watcher`) get:

* every change as soon as it happens,
* the whole object with every change, and
* every change on every instance (replica) of the app.

Some handlers need less than this. For example:

* A reconciler that always reads the object again from the API does not need the object in the event. It only
  needs to know which object changed.
* A reconciler that checks every object once an hour does not need to see changes as they happen.

You can tell the app what your handlers need with `WatchNeeds`. The app uses this to pick a cheaper way to watch the
kind, if one is configured. This section uses these terms:

| Term | Meaning |
| --- | --- |
| delivery | one event that the app sends to your handlers |
| live channel | deliveries sent when an object changes |
| resync channel | deliveries of every object again, on a fixed schedule |
| watch mechanism | a way to watch a kind, such as the API watch or an event bus. It states what it provides |

If you do not set `WatchNeeds`, nothing changes: the kind is watched as before. If you set `WatchNeeds` but the app
has no cheaper mechanism configured, nothing changes either. The kind still gets the API watch.

### What you declare

Set `WatchNeeds` in `ReconcileOptions` of the kind. It is an `operator.WatchOptions` with one field for each channel:

```go
operator.WatchOptions{
    // What the handlers need from the live channel. Nil means they do not need the live channel.
    Live: &operator.WatchLiveOptions{Detail: ..., Distribution: ...},
    // What the handlers need from the resync channel. Nil means they do not need a resync.
    Resync: &operator.WatchResyncOptions{Detail: ..., Distribution: ..., Interval: ..., ListPageSize: ...},
}
```

Rules for the two channels:

* If you set a channel, your handlers need that channel.
* If you leave a channel nil, your handlers do not need it. A mechanism without that channel can serve the kind.
* If you leave **both** channels nil, the app treats this as a need for the live channel with default values.
  This is the default behavior.

This means that if you need a resync **and** live changes, you must set both fields. `WatchOptions{Resync: ...}`
alone means "no live changes needed". A mechanism that only sends every object on a schedule can serve it.

A nil channel is not ignored completely. Your handlers cannot tell a live delivery from a resync delivery. So if a
mechanism also sends deliveries on a channel that you left nil, those deliveries must meet the rules of the channel
that you did set:

* They must contain at least the `Detail` that you declared.
* They must be `Sharded` if you declared `WatchDistributionSharded`.

For example, a kind that declares only a resync with the whole object cannot get an event bus whose live events
contain only the object's name.

Inside a channel, every field has a safe default. If you leave `Detail` or `Distribution` empty, the app uses the
strictest value: the whole object, sent to every instance. So leaving these fields empty never lets the app pick a
mechanism that it would not pick if you set them.

#### `Detail`: how much of the object a delivery must contain

| Value | Use it when your handlers |
| --- | --- |
| `WatchDetailFullObject` (default) | read the object from the event |
| `WatchDetailMetadata` | use only metadata, such as labels, finalizers, or timestamps |
| `WatchDetailIdentity` | need only the namespace and name, because they read the object again before they act |

A delivery with more detail can always serve a handler that needs less. For example, a mechanism that sends the
full object can serve a handler that needs only metadata.

#### `Distribution`: how the work is split across instances of the app

| Value | Use it when |
| --- | --- |
| `WatchDistributionAll` (default) | every instance must see every delivery, for example because your handlers keep their own state in memory |
| `WatchDistributionShared` | the work can be done once by any instance. You **allow** the app to split it, but do not require it |
| `WatchDistributionSharded` | each object must always go to the same instance. You **require** this |

Note the difference between `Shared` and `Sharded`:

* `Shared` is a permission. A mechanism with any distribution can serve it, including one that sends everything to
  every instance.
* `Sharded` is a requirement. Only a mechanism that shards by object can serve it.

#### `Interval` and `ListPageSize`

`Resync.Interval` is how often the resync should run. `Resync.ListPageSize` is how many objects to read in each
page when the mechanism lists objects from the API. If you leave them at zero, the app-wide settings
`InformerOptions.CacheResyncInterval` and `InformerOptions.WatchListPageSize` are used.

These two values are requests. The app does not use them to pick a mechanism, and a mechanism can ignore them.
For example, the API watch ignores `ListPageSize` for its resync (see below).

### How the app picks a mechanism

Use `operator.NewSelectingInformerSupplier` as the app's `InformerSupplier`. Give it:

1. A fallback supplier. The app uses it for any kind that no mechanism can serve.
2. A list of mechanisms, ordered from the cheapest to the most capable.

For each kind, the app uses the **first** mechanism in the list that can serve the kind's `WatchNeeds`. So each kind
gets the cheapest mechanism that works for it. If no mechanism matches, the app uses the fallback supplier. If you
list no mechanisms, every kind uses the fallback supplier.

You can create a mechanism from an existing supplier with `operator.NewInformerMechanism`. You pass what the
mechanism provides, and the supplier that builds its informers. You can also implement the
`operator.InformerMechanism` interface yourself.

In the example below, `myReListSupplier`, `myNotifySupplier`, and `myEventBusSupplier` are your own
`operator.InformerSupplier` functions.

```go
// Sends every object on a schedule. Has no live channel.
reList := operator.NewInformerMechanism(operator.WatchOptions{
    Resync: &operator.WatchResyncOptions{Detail: operator.WatchDetailFullObject},
}, myReListSupplier)

// Sends metadata as changes happen. Has no resync.
notifyOnly := operator.NewInformerMechanism(operator.WatchOptions{
    Live: &operator.WatchLiveOptions{Detail: operator.WatchDetailMetadata},
}, myNotifySupplier)

// Sends the name of each changed object, and every full object on a schedule.
// It costs the most: a subscription and a periodic list.
eventBus := operator.NewInformerMechanism(operator.WatchOptions{
    Live:   &operator.WatchLiveOptions{Detail: operator.WatchDetailIdentity},
    Resync: &operator.WatchResyncOptions{Detail: operator.WatchDetailFullObject},
}, myEventBusSupplier)

cfg := simple.AppConfig{
    InformerConfig: simple.AppInformerConfig{
        InformerSupplier: operator.NewSelectingInformerSupplier(
            simple.DefaultInformerSupplier, // fallback: the API watch
            reList,
            notifyOnly,
            eventBus,
        ),
    },
    ManagedKinds: []simple.AppManagedKind{{
        Kind:       myCleanupKind.Kind(),
        Reconciler: &myCleanupReconciler,
        ReconcileOptions: simple.BasicReconcileOptions{
            // This reconciler does not need live changes. It needs only an hourly resync.
            // The first mechanism that can serve this is reList, so the app uses it.
            // eventBus could not serve it: its live events contain only names, which is less
            // than the full object that this kind declared for its resync.
            WatchNeeds: operator.WatchOptions{
                Resync: &operator.WatchResyncOptions{Interval: time.Hour, ListPageSize: 5000},
            },
            UsePlain: true,
        },
    }},
}
```

#### The resync of the API watch

The API watch can resync only if a resync interval is set, either in `WatchNeeds.Resync.Interval` or in
`InformerOptions.CacheResyncInterval`. Its resync sends again the objects that are already in its in-memory cache.
It does not list objects from the API. This has two effects:

* It ignores `ListPageSize` for the resync.
* It runs your handlers again for every object, but it cannot find changes that the live channel missed.
  A mechanism that lists objects from the API during its resync can find those changes.

### What the app rejects

The app checks each kind when it is created (in `simple.NewApp`, or in `WatchKind` and `ReconcileKind` for
`simple.Operator`). If the kind's `WatchNeeds` cannot be served, creation fails with an error. The error message
says what to change.

#### Needs that the API watch cannot serve

The API watch cannot serve these needs:

* A resync without an interval. Set `WatchNeeds.Resync.Interval` for the kind, or `CacheResyncInterval` for the app.
* `WatchDistributionSharded`. The API watch sends everything to every instance.

`WatchDistributionShared` is not a problem. It allows splitting but does not require it, so the API watch can serve
it. If you use leader election, only one instance is active at a time. In that case you do not need to declare a
distribution.

#### Opinionated reconcilers and watchers

The opinionated reconciler and watcher (the default in `simple.App`, and always used by `simple.Operator`) read three
fields from the object in each event: the deletion timestamp, the list of finalizers, and the generation. All three
are metadata. So:

* `WatchDetailMetadata` and `WatchDetailFullObject` work with them.
* `WatchDetailIdentity` does not work with them. The app rejects a kind that declares `WatchDetailIdentity` and
  uses an opinionated handler.
* The app also rejects a mechanism that sends identity-only deliveries on **any** channel to an opinionated handler,
  even on a channel that the kind did not ask for.

To use identity-only deliveries, use a plain handler: set `UsePlain: true` for a managed kind, or leave
`UseOpinionated` as `false` for an unmanaged kind. Then handle deletion in your own reconcile code.

An opinionated **watcher** cannot declare a `Resync`. It skips every update in which the generation did not change,
and a resync delivery is such an update. So it would never act on a resync, and the app rejects the declaration.
This applies to a `Watcher` in `simple.App` that does not set `UsePlain` (or sets `UseOpinionated` for an unmanaged
kind), and to `simple.Operator.WatchKind`. If you need a resync, use a `Reconciler` instead (the opinionated
reconciler passes resyncs to your code), or use a plain `Watcher`.

#### Two entries for the same kind

Sometimes two entries watch the same kind. In `simple.App`, this happens when two entries have the same kind. In
`simple.Operator`, it happens when two calls use the same kind and the same `ListWatchOptions`. In both cases, the
informers for that kind send their deliveries to the handlers of **both** entries.

So the app checks each pair of entries. Creation fails if the informer of one entry sends deliveries that the
handlers of the other entry cannot take:

* deliveries with less detail than those handlers declared, or
* deliveries that are not sharded, when those handlers declared `WatchDistributionSharded`.

Informers that provide different channels are fine, because each entry still gets the channels that it asked for
from its own informer.

### What the SDK cannot check

The SDK cannot check these cases. You must take care of them yourself:

* Your handler reads fields that the delivery does not contain. For example, it reads `obj.Spec` from a
  metadata-only delivery. Make sure your code matches what you declared, in the same way that it must match your
  label selectors.
* A `Watcher` reads the previous version of the object in an update event. A mechanism that does not keep the whole
  object cannot provide the previous version. If you declare less than `WatchDetailFullObject`, use a `Reconciler`
  instead of a `Watcher`.
* The opinionated watcher itself needs the previous object. It compares the old and new objects in each update, and
  it returns an error for an update without a previous object, even if your own watcher never reads it. If you
  write your own informer, it must send the previous object with each update, with the same detail as the new
  object.

`WatchNeeds` is only about deliveries to your handlers. It does not control whether the informer keeps a cache.
A cache affects memory use, but your handlers do not read the cache through `operator.Informer`.

### Using `simple.Operator`

`simple.Operator` supports the same declaration. Pass `simple.WithWatchNeeds(...)` to `WatchKind` or `ReconcileKind`,
and set `OperatorConfig.InformerSupplier` to choose the supplier.
