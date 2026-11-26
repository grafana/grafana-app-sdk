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
> `WatchOptions`, the watch mechanisms, and the `resourcebus` package are experimental. They may change in a minor
> release. The `resourcebus` package may change or be removed without notice.

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
kind, if one is configured. For example, it can use a periodic LIST, or a message bus that sends change
notifications. This section uses these terms:

| Term | Meaning |
| --- | --- |
| delivery | one event that the app sends to your handlers |
| live channel | deliveries sent when an object changes |
| resync channel | deliveries of every object again, on a fixed schedule |
| watch mechanism | a way to watch a kind, such as the API watch or a bus. It states what it provides |
| bus | a message system, outside the Kubernetes API, that announces changes to objects |
| identity | only the namespace, name, and `resourceVersion` of an object |

If you do not set `WatchNeeds`, nothing changes: the kind is watched as before. If you set `WatchNeeds` but the app
has no cheaper mechanism configured, nothing changes either. The kind still gets the API watch.

### What you declare

Set `WatchNeeds` in `ReconcileOptions` of the kind. It is an `operator.WatchOptions` with one field for each channel:

```go
operator.WatchOptions{
    // What the handlers need from the live channel. Nil means they do not need the live channel.
    Live: &operator.WatchLiveOptions{
        Detail: ..., Distribution: ...,
        Ignore: ..., IgnoreUnchanged: ..., // see "Leaving objects out of a delivery"
    },
    // What the handlers need from the resync channel. Nil means they do not need a resync.
    Resync: &operator.WatchResyncOptions{
        Detail: ..., Distribution: ..., Interval: ..., ListPageSize: ...,
        Ignore: ..., IgnoreUnchanged: ..., // see "Leaving objects out of a delivery"
    },
}
```

Rules for the two channels:

* If you set a channel, your handlers need that channel.
* If you leave a channel nil, your handlers do not need it. A mechanism without that channel can serve the kind.
* If you leave **both** channels nil, the app treats this as a need for the live channel with default values: the
  whole object, sent to every instance. This is what the API watch gives every kind, so a kind that does not set
  `WatchNeeds` keeps working as before. It does not need a resync, but it still gets one if the app sets
  `InformerOptions.CacheResyncInterval`.

This means that if you need a resync **and** live changes, you must set both fields. `WatchOptions{Resync: ...}`
alone means "no live changes needed". A mechanism that only sends every object on a schedule can serve it.

A nil channel is not ignored completely. Your handlers cannot tell a live delivery from a resync delivery. So if a
mechanism also sends deliveries on a channel that you left nil, those deliveries must meet the rules of the channel
that you did set:

* They must contain at least the `Detail` that you declared.
* They must be `Sharded` if you declared `WatchDistributionSharded`.

For example, a kind that declares only a resync with the whole object cannot get a bus whose live events contain
only identities.

Inside a channel, every field has a safe default. If you leave `Detail` or `Distribution` empty, the app uses the
strictest value: the whole object, sent to every instance. So leaving these fields empty never lets the app pick a
mechanism that it would not pick if you set them.

#### `Detail`: how much of the object a delivery must contain

| Value | Use it when your handlers |
| --- | --- |
| `WatchDetailFullObject` (default) | read the object from the event |
| `WatchDetailMetadata` | use only metadata, such as labels, finalizers, or timestamps |
| `WatchDetailIdentity` | need only the identity, because they read the object again before they act |

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
* `Sharded` is a requirement. Only a mechanism that shards by object can serve it. No built-in mechanism does this
  today.

#### `Interval` and `ListPageSize`

`Resync.Interval` is how often the resync should run. `Resync.ListPageSize` is how many objects to read in each
page when the mechanism lists objects from the API. If you leave them at zero, the app-wide settings
`InformerOptions.CacheResyncInterval` and `InformerOptions.WatchListPageSize` are used.

These two values are requests. The app does not use them to pick a mechanism, and a mechanism can ignore them.
For example, the API watch ignores `ListPageSize` for its resync (see below).

#### Leaving objects out of a delivery

> [!NOTE]
> No built-in mechanism applies `Ignore` or `IgnoreUnchanged` yet. Today, every delivery reaches your handlers, even
> if you set these fields. You can set them now. Mechanisms that support them will start to use them.

Many deliveries need no work. For example, a resync delivers every object, even objects that did not change. A
live update can be a write to `status` that your handlers do not care about. Each channel has two optional fields
that let you say which objects it can leave out. They work the same way on both channels:

* `Ignore` is a `WatchIgnoreFunc`. It looks at the current object only. If it returns `true`, the channel can leave
  the object out.
* `IgnoreUnchanged` is a `WatchUnchangedFunc`. It compares the object with the version that was delivered last
  time. If it returns `true` (unchanged), the channel can leave the object out. On the resync channel, only a
  mechanism that implements `operator.PreviousComparer` applies it.

Set them on `Live` (a `WatchLiveOptions`) to filter changes as they happen, on `Resync` (a `WatchResyncOptions`) to
filter the scheduled deliveries, or on both. A function set on one channel does not apply to the other.

These fields are requests. A mechanism that does not apply them delivers the object anyway, and the app does not use
them to pick a mechanism. Some objects are never left out:

* an object that is being deleted (it has a deletion timestamp),
* for `IgnoreUnchanged`: an object with no earlier delivery, or with a missing or different UID (the object was
  deleted and created again).

The `operator` package has ready-made checks:

| Check | Leaves out an object when |
| --- | --- |
| `SameResourceVersion` | its `resourceVersion` did not change, so nobody wrote to it |
| `SameGeneration` | its `generation` did not change, so only status or metadata changed. Without a generation, it compares `resourceVersion` |
| `SameLabels` | its labels did not change |
| `SameAnnotations` | its annotations did not change |
| `AllUnchanged(checks...)` | every check reports it unchanged |
| `AlreadyObserved(accessor)` | the observed generation in its status (read by your accessor function) equals its `generation`. Use it as an `Ignore` function |

For example:

```go
operator.WatchOptions{
    // Live: skip updates that change neither the spec (generation) nor the labels.
    Live: &operator.WatchLiveOptions{
        IgnoreUnchanged: operator.AllUnchanged(operator.SameGeneration, operator.SameLabels),
    },
    // Resync: every hour, skip objects whose status already shows the current generation.
    Resync: &operator.WatchResyncOptions{
        Interval: time.Hour,
        Ignore:   operator.AlreadyObserved(observedGeneration),
    },
}
```

Here, `observedGeneration` is your own function. It reads the observed generation from the status of your kind and
returns it, and `false` when the object has none.

Leave `IgnoreUnchanged` nil on the resync if your handlers use the resync to repair drift, for example to recreate
something outside the cluster that was removed. If unchanged objects are left out, that repair never runs.

### How the app picks a mechanism

Use `operator.NewSelectingInformerSupplier` as the app's `InformerSupplier`. Give it:

1. A fallback supplier. The app uses it for any kind that no mechanism can serve. Usually this is
   `simple.DefaultInformerSupplier` (the API watch).
2. A list of mechanisms, ordered from the cheapest to the most capable.

For each kind, the app uses the **first** mechanism in the list that can serve the kind's `WatchNeeds`. So each kind
gets the cheapest mechanism that works for it. The app also skips:

* a `nil` mechanism. The bus constructors return `nil` when you pass a `nil` subscriber, so you can turn a bus off
  without changing the list.
* a mechanism that cannot handle the kind's query. The bus mechanisms do not support label selectors or field
  selectors, so a kind with selectors never gets a bus.

If no mechanism matches, the app uses the fallback supplier. If you list no mechanisms, every kind uses the fallback
supplier.

### Built-in mechanisms

The `operator` package has these mechanisms:

| Mechanism | Live channel | Resync channel |
| --- | --- | --- |
| API watch (`simple.DefaultInformerSupplier`) | full object, every instance | full object, every instance, only if an interval is set |
| `NewPeriodicList` | none | full object, every instance |
| `NewPeriodicKeysList` | none | identity, every instance |
| `NewBusNotifications` | identity. Every instance, or shared in a named group | none |
| `NewBusRecovery` | identity. Every instance, or shared in a named group | identity, every instance |

You can also write your own. Use `operator.NewInformerMechanism` to wrap an existing `operator.InformerSupplier`
with what it provides, or implement the `operator.InformerMechanism` interface.

All built-in mechanisms use `InformerOptions.EventTimeout` as the time limit for each handler call. A retry gets a
new time limit of the same length. Your handler must stop when its context is canceled.

#### The API watch

The API watch can resync only if a resync interval is set, either in `WatchNeeds.Resync.Interval` or in
`InformerOptions.CacheResyncInterval`. Its resync sends again the objects that are already in its in-memory cache.
It does not list objects from the API. This has two effects:

* It ignores `ListPageSize` for the resync.
* It runs your handlers again for every object, but it cannot find changes that the live channel missed.
  A mechanism that lists objects from the API during its resync can find those changes.

#### Periodic LIST: `NewPeriodicList`

Use a periodic LIST for kinds that declare only a `Resync`, for work that visits every existing object again, such
as age-based cleanup. Use a live channel for work that must react to changes or deletions.

On each run (a "sweep"), the mechanism does one complete paginated LIST. Then it sends every object to your handlers
as an add event, including objects that did not change since the last sweep. Note these details:

* It does not send delete events. An object deleted between two sweeps is simply not in the next sweep.
* The next interval starts after all objects of the sweep have been delivered.
* If the LIST fails, the mechanism discards the objects it already read, reports the error, and tries again after
  `RetryInterval` (default 5 seconds).
* It supports a namespace, label selectors, and field selectors. A finalizer-aware handler can process a pending
  deletion only while the object still matches those selectors.

`PeriodicListOptions` sets the defaults for this mechanism. The kind's settings win over the app's settings, and the
app's settings win over these defaults:

| Setting | Order of precedence |
| --- | --- |
| interval | `WatchNeeds.Resync.Interval`, then `CacheResyncInterval`, then `PeriodicListOptions.ResyncInterval`, then 5 minutes |
| page size | `WatchNeeds.Resync.ListPageSize`, then `WatchListPageSize`, then `PeriodicListOptions.PageSize`, then 500 |

#### Periodic keys-only LIST: `NewPeriodicKeysList`

This works like `NewPeriodicList`, but it delivers only identities. Use it for handlers that read each object again
before they act.

Pass an `operator.KeyListerFactory` to read only keys from the API, which costs less than reading whole objects.
If the factory is `nil`, or if it returns `operator.ErrProjectionUnsupported` for the kind or query, the mechanism
does a normal LIST and keeps only the identities.

#### Bus notifications: `NewBusNotifications`

This mechanism subscribes to a bus (see [Connecting a bus](#connecting-a-bus)) and delivers each notification as an
identity:

* a created or modified object arrives as an add event,
* a deleted object arrives as a delete event.

It does not recover notifications that it misses. The bus delivers each notification at most once. If the app is
not connected, or its queue (1000 notifications) is full, those notifications are lost. Use it only for handlers
that can accept missed changes, or use `NewBusRecovery`.

The `resourcebus.Group` that you pass controls the distribution:

* With an empty group name, every instance receives every notification (`WatchDistributionAll`).
* With a group name, the instances in the group share the notifications. Each notification goes to one instance.
  The mechanism then provides `WatchDistributionShared`. So a kind must declare `WatchDistributionShared` for its
  live channel to get it, because the default distribution (`WatchDistributionAll`) requires every instance.

#### Bus with recovery: `NewBusRecovery`

This mechanism combines bus notifications with LIST requests, so it does not lose changes. It keeps a list of the
identities that it knows about (not whole objects). It works like this:

1. At startup, it subscribes to the bus and does a LIST. The informer is ready after this first LIST.
2. Live: it delivers each notification as an add, update, or delete event. It drops a notification that is older
   than what it already knows about the object.
3. Scheduled resync: it does a LIST at each interval (default 5 minutes). It delivers every listed object, and a
   delete event for each object that is no longer there.
4. Catch-up: it does an extra LIST when the bus reconnects after a gap, when notifications were dropped because its
   queue was full (at most once every 30 seconds), and to retry after a failed LIST. A catch-up LIST delivers only
   the objects that changed.

Requirements and options:

* The LIST and the bus notifications must use the same numeric `resourceVersion` values, and those values must
  increase with every write. If a LIST returns a value that is not a number, the LIST fails.
* Handlers run on up to `InformerOptions.MaxConcurrentWorkers` workers (default 10). Events for the same object
  always go to the same worker, so they stay in order.
* `BusRecoveryDefaults` sets the default `ResyncInterval` and `PageSize`, and a `KeyListerFactory` (used in the same
  way as in `NewPeriodicKeysList`). The kind's settings win over these defaults.
* By default, startup waits until the bus subscription works. Set `AllowDegradedStart` to start with only the LIST
  when the bus is not available. The mechanism keeps trying to subscribe, and does a LIST again when it connects.

With a named group, the instances share the live notifications, but every instance does its own LIST. So the live
channel is shared, and the resync channel goes to every instance.

#### Connecting a bus

The bus mechanisms read from a `resourcebus.Subscriber`. The SDK does not include an adapter for a specific bus
(such as NATS). You write a small adapter that implements this interface:

```go
type Subscriber interface {
    Subscribe(ctx context.Context, gvr schema.GroupVersionResource, namespace string,
        handler func(context.Context, resourcebus.Event), opts ...resourcebus.SubscribeOption) (resourcebus.Subscription, error)
}
```

The adapter must follow these rules:

* `Subscribe` delivers notifications for one resource, in one namespace, or in every namespace when `namespace` is
  empty. It delivers each notification at most once.
* `handler` must not block. It is called on the delivery path.
* A `resourcebus.Event` contains the change (`VerbAdded`, `VerbModified`, or `VerbDeleted`) and the identity of the
  object, but never the object itself. A delete that only sets a deletion timestamp is a `VerbModified`.
* `Subscription.WaitReady` returns when the subscription can receive events. Its context always has a deadline.
  `Subscription.Close` stops delivery.
* Read the options with `resourcebus.NewSubscribeConfig(opts...)`:
  * `Group`: subscribers with the same group name, resource, and namespace share delivery. If the adapter cannot
    provide the requested `Delivery` (`GroupDeliveryDefault` or `GroupDeliveryRoundRobin`), it must return an error.
    It must not use a different delivery mode instead.
  * `OnReconnect`: call it each time the subscription connects again after a gap. `NewBusRecovery` uses it to start a
    catch-up LIST. It must not block.
* For a permanent failure, such as missing permissions, wrap `resourcebus.ErrSubscriptionRejected` in the error. The
  informer then stops and reports the error. For other errors, it tries again about every 5 seconds.

#### Example

In this example, `subscriber` is your `resourcebus.Subscriber`.

```go
group := resourcebus.Group{Name: "my-app"}

cfg := simple.AppConfig{
    InformerConfig: simple.AppInformerConfig{
        InformerSupplier: operator.NewSelectingInformerSupplier(
            simple.DefaultInformerSupplier, // fallback: the API watch
            operator.NewPeriodicList(operator.PeriodicListOptions{}),
            operator.NewBusNotifications(subscriber, group),
            operator.NewBusRecovery(subscriber, group, operator.BusRecoveryDefaults{}),
        ),
    },
    ManagedKinds: []simple.AppManagedKind{{
        Kind:       myCleanupKind.Kind(),
        Reconciler: &myCleanupReconciler,
        ReconcileOptions: simple.BasicReconcileOptions{
            // Needs no live changes, only an hourly resync with the whole object.
            // The first mechanism that can serve this is NewPeriodicList.
            WatchNeeds: operator.WatchOptions{
                Resync: &operator.WatchResyncOptions{Interval: time.Hour, ListPageSize: 5000},
            },
            UsePlain: true,
        },
    }, {
        Kind:       mySyncKind.Kind(),
        Reconciler: &mySyncReconciler,
        ReconcileOptions: simple.BasicReconcileOptions{
            // Reads each object again, so identities are enough. Allows the group to share live changes,
            // and needs a resync to catch missed changes. NewBusNotifications has no resync,
            // so the first mechanism that can serve this is NewBusRecovery.
            WatchNeeds: operator.WatchOptions{
                Live: &operator.WatchLiveOptions{
                    Detail:       operator.WatchDetailIdentity,
                    Distribution: operator.WatchDistributionShared,
                },
                Resync: &operator.WatchResyncOptions{Detail: operator.WatchDetailIdentity},
            },
            UsePlain: true, // identities cannot serve an opinionated reconciler
        },
    }},
}
```

Any other kind in this app keeps the default needs, so it gets the API watch.

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

To use identity-only deliveries, such as from the bus mechanisms or `NewPeriodicKeysList`, use a plain handler: set
`UsePlain: true` for a managed kind, or leave `UseOpinionated` as `false` for an unmanaged kind. Then handle deletion
in your own reconcile code. Because `simple.Operator` always uses opinionated handlers, it cannot use identity-only
deliveries.

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
