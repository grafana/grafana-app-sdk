# Optimizing Watches (experimental)

This page explains how to tell the app what your reconcilers and watchers need, so that it can watch each kind in a
cheaper way.

Read [Writing a Reconciler](./writing-a-reconciler.md) first. This page assumes that you know how reconcilers and
watchers work.

> [!WARNING]
> `WatchOptions`, the watch mechanisms, and the `resourcebus` package are experimental. They may change in a minor
> release. The `resourcebus` package may change or be removed without notice.

## Overview

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
notifications. This page uses these terms:

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

## What you declare

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

### `Detail`: how much of the object a delivery must contain

| Value | Use it when your handlers |
| --- | --- |
| `WatchDetailFullObject` (default) | read the object from the event |
| `WatchDetailMetadata` | use only metadata, such as labels, finalizers, or timestamps |
| `WatchDetailIdentity` | need only the identity, because they read the object again before they act |

A delivery with more detail can always serve a handler that needs less. For example, a mechanism that sends the
full object can serve a handler that needs only metadata.

### `Distribution`: how the work is split across instances of the app

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

### `Interval` and `ListPageSize`

`Resync.Interval` is how often the resync should run. `Resync.ListPageSize` is how many objects to read in each
page when the mechanism lists objects from the API. If you leave them at zero, the app-wide settings
`InformerOptions.CacheResyncInterval` and `InformerOptions.WatchListPageSize` are used.

These two values are requests. The app does not use them to pick a mechanism, and a mechanism can ignore them.
For example, the API watch ignores `ListPageSize` (see below).

### Leaving objects out of a delivery

> [!NOTE]
> No built-in mechanism applies `Ignore` or `IgnoreUnchanged` yet. Today, every delivery reaches your handlers, even
> if you set these fields. You can set them now. Mechanisms that support them will start to use them.

Many deliveries need no work. For example, a resync delivers every object, even objects that did not change. A
live update can be a write to `status` that your handlers do not care about. Each channel has two optional fields
that let you say which objects it can leave out:

* `Ignore` is a `WatchIgnoreFunc`. It looks at the current object only. If it returns `true`, the channel can leave
  the object out.
* `IgnoreUnchanged` is a `WatchUnchangedFunc`. It compares the current object with a previous one. If it returns
  `true` (unchanged), the channel can leave the object out.

The two channels apply them a little differently:

| | `Live` (`WatchLiveOptions`) | `Resync` (`WatchResyncOptions`) |
| --- | --- | --- |
| `Ignore` can leave out | adds and updates | any object in the resync |
| `IgnoreUnchanged` can leave out | updates only | any object in the resync |
| `IgnoreUnchanged` compares with | the previous object that the update carries | the object that was delivered last time. Only a mechanism that implements `operator.PreviousComparer` applies it |
| always delivered | deletes. For `IgnoreUnchanged`, adds too | |

A function set on one channel does not apply to the other.

These fields are requests. A mechanism that does not apply them delivers the object anyway, and the app does not use
them to pick a mechanism. On both channels, some objects are never left out:

* an object that is being deleted (it has a deletion timestamp),
* for `IgnoreUnchanged`: an object with no previous version to compare with, or with a missing or different UID (the
  object was deleted and created again).

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

## How the app picks a mechanism

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

## Built-in mechanisms

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

### The API watch

The API watch can resync only if a resync interval is set, either in `WatchNeeds.Resync.Interval` or in
`InformerOptions.CacheResyncInterval`. Its resync sends again the objects that are already in its in-memory cache.
It does not list objects from the API. This has two effects:

* It ignores `ListPageSize`. (It also ignores `InformerOptions.WatchListPageSize` and `UseWatchList`. Only
  `CustomCacheInformer` uses them.)
* It runs your handlers again for every object, but it cannot find changes that the live channel missed.
  A mechanism that lists objects from the API during its resync can find those changes.

### Periodic LIST: `NewPeriodicList`

Use a periodic LIST for kinds that declare only a `Resync`, for work that visits every existing object again, such
as age-based cleanup. Use a live channel for work that must react to changes or deletions.

On each run (a "sweep"), the mechanism does one complete paginated LIST. Then it sends every object to your handlers
as an add event, including objects that did not change since the last sweep. Note these details:

* Because every object arrives as an add, a plain reconciler sees `ReconcileActionCreated` on every sweep, not
  `ReconcileActionResynced`. Do not use the action to decide whether an object is new.
* It calls your handlers one object at a time. It ignores `InformerOptions.MaxConcurrentWorkers`, so a slow handler
  delays the whole sweep.
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

### Periodic keys-only LIST: `NewPeriodicKeysList`

This works like `NewPeriodicList`, but it delivers only identities. Use it for handlers that read each object again
before they act.

Pass an `operator.KeyListerFactory` to read only keys from the API, which costs less than reading whole objects.
If the factory is `nil`, or if it returns `operator.ErrProjectionUnsupported` for the kind or query, the mechanism
does a normal LIST and keeps only the identities.

### Bus notifications: `NewBusNotifications`

This mechanism subscribes to a bus (see [Connecting a bus](#connecting-a-bus)) and delivers each notification as an
identity:

* a created or modified object arrives as an add event,
* a deleted object arrives as a delete event.

Note these limits:

* It does not deliver objects that already exist when the app starts. It does no LIST. Your handlers see only the
  objects that change after the subscription is ready.
* It does not recover notifications that it misses. The bus delivers each notification at most once. If the app is
  not connected, or its queue (1000 notifications) is full, those notifications are lost without an error.
* It calls your handlers one notification at a time. It ignores `InformerOptions.MaxConcurrentWorkers`.

Use it only for handlers that can accept these limits. Otherwise, use `NewBusRecovery`.

The `resourcebus.Group` that you pass controls the distribution:

* With an empty group name, every instance receives every notification (`WatchDistributionAll`).
* With a group name, the instances in the group share the notifications. Each notification goes to one instance.
  The mechanism then provides `WatchDistributionShared`. So a kind must declare `WatchDistributionShared` for its
  live channel to get it, because the default distribution (`WatchDistributionAll`) requires every instance.

### Bus with recovery: `NewBusRecovery`

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

### Connecting a bus

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

### Example

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

## What the app rejects

The app checks each kind when it is created (in `simple.NewApp`, or in `WatchKind` and `ReconcileKind` for
`simple.Operator`). If the kind's `WatchNeeds` cannot be served, creation fails with an error. The error message
says what to change.

### Needs that the API watch cannot serve

The API watch cannot serve these needs:

* A resync without an interval. Set `WatchNeeds.Resync.Interval` for the kind, or `CacheResyncInterval` for the app.
* `WatchDistributionSharded`. The API watch sends everything to every instance.

`WatchDistributionShared` is not a problem. It allows splitting but does not require it, so the API watch can serve
it. If you use leader election, only one instance is active at a time. In that case you do not need to declare a
distribution.

### Opinionated reconcilers and watchers

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

### Two entries for the same kind

Sometimes two entries watch the same kind. In `simple.App`, this happens when two entries have the same kind. In
`simple.Operator`, it happens when two calls use the same kind and the same `ListWatchOptions`. In both cases, the
informers for that kind send their deliveries to the handlers of **both** entries.

So the app checks each pair of entries. Creation fails if the informer of one entry sends deliveries that the
handlers of the other entry cannot take:

* deliveries with less detail than those handlers declared, or
* deliveries that are not sharded, when those handlers declared `WatchDistributionSharded`.

Informers that provide different channels are fine, because each entry still gets the channels that it asked for
from its own informer.

## What the SDK cannot check

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

## Using `simple.Operator`

`simple.Operator` supports the same declaration. Pass `simple.WithWatchNeeds(...)` to `WatchKind` or `ReconcileKind`,
and set `OperatorConfig.InformerSupplier` to choose the supplier.
