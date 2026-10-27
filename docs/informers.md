# Informers

An Informer is where an operator's add/update/delete events come from. It is the piece between the API and your
reconciler or watcher: it learns that something changed, works out what kind of change it was, and hands the object
to each registered handler. The `operator.InformerController` coordinates informers and handlers; see
[Operators and Event-Based Design](operators.md) for how the pieces fit together.

An informer answers two questions, and the implementations differ in how they answer them:

* **How does it learn about a change?** A kubernetes watch, or a bus the storage layer publishes change notifications
  to, or nothing at all — just a list on a timer.
* **What does it hold in memory?** Every object it has seen, so handlers can read from the cache, or only enough to
  tell one change from another.

## Choosing an informer

Informers are created by an `InformerSupplier`, selected per app with `AppInformerConfig.InformerSupplier` or per
kind with `ReconcileOptions.InformerSupplier` (the per-kind one wins). Or let the kinds choose — see
[Declaring what a kind needs](#declaring-what-a-kind-needs). The SDK ships four:

| Supplier | Change source | Memory |
| --- | --- | --- |
| `simple.DefaultInformerSupplier` | kubernetes watch | every object of the kind |
| `simple.OptimizedInformerSupplier` | kubernetes watch | every object, in a cache tuned for large counts |
| `simple.BusInformerSupplier` | a `resourcebus.Subscriber` | identity per object, in identity mode |
| `simple.PeriodicInformerSupplier` | a re-list on a cadence, nothing live | nothing retained between passes |

`DefaultInformerSupplier` is the one to use unless you have a reason not to. Reach for
`OptimizedInformerSupplier` when a kind has more than ~10k objects, you watch cluster-scoped resources, or you are
memory constrained; it respects `UseWatchList` and `WatchListPageSize`, which the default supplier ignores. Reach for
`BusInformerSupplier` when your reconcilers read the object they act on anyway, so the cache earns nothing — see
below. Its snapshot holds namespace, name and resource version per object, and no object bodies, whatever the lister
returns: with `PayloadIdentity` the informer projects listed objects onto their identities before retaining them, so
a full-object list and an identity list cost the same to hold. `PayloadHydrated` retains whole objects by
definition, since that is what it delivers.

> [!CAUTION]
> The bus informer, the periodic informer and the `resourcebus` contract are **experimental**. Their Go APIs may
> change or be removed in any release. The watch-backed suppliers are not affected.

## Declaring what a kind needs

Which informers a deployment can offer is not something an app ships knowing: a change bus may or may not be running.
So a kind declares in its CUE what its handlers need from the change stream, and the runtime resolves that to the
cheapest informer which satisfies it:

```cue
watch: {
    needs: "signal"
    metadata: {
        finalizers:        ["myapp.grafana.app/finalizer"]
        generation:        true
        deletionTimestamp: true
    }
}
```

| `needs` | Meaning | Resolves to |
| --- | --- | --- |
| `objects` (the default) | handlers read objects from the informer's cache | the watch |
| `signal` | handlers re-read the object they act on, so knowing which one changed is enough | the bus where one is configured, the watch otherwise |
| `cadence` | handlers re-evaluate everything on a schedule; individual changes need not arrive | the periodic informer |

Pass the manifest to `simple.InformerSupplierFromManifest` and it does the resolving:

```go
InformerSupplier: simple.InformerSupplierFromManifest(manifest, simple.ManifestInformerConfig{
    Subscriber: subscriber, // nil where the deployment runs no bus
})
```

**Every fallback is upward in capability, so it is always safe.** A kind which needs a signal, served by a watch
because no bus is configured, is given strictly more than it asked for — whole objects and a cache — at the cost of
more memory. That is why the declaration names a requirement and not an informer: an app asking for "the bus" on a
deployment without one has said something that cannot be honoured, whereas an app asking for a signal can always be
served somehow.

`AnnouncedMetadata` defaults to what the same `watch.metadata` declares, so an app states that once too.

## The bus informer

A `BusInformer` takes its changes from a bus the storage layer publishes notifications to, rather than from a watch.
It keeps no per-object cache a handler can read from. Two things follow.

**A re-list is what makes it correct, not just live.** On a fixed cadence (5 minutes by default, with 10% jitter) the
informer lists the whole kind and diffs the result against its previous snapshot: a key seen for the first time is an
add, a key seen before is an update, and a key which has vanished is a delete carrying its last-known state. An event
published while the subscription was down, delivered to another member of a group, or never announced at all is
caught there. The cadence is an upper bound on how stale a replica can be, not a tuning nicety.

**The live stream decides latency, not coverage.** Because the re-list covers what a replica did not see live, a
replica does not have to see every event.

The two overlap, because the subscription stays open while the list runs. A change can land after the list read its
snapshot but before the diff, and taking the list as authoritative would then undo it:

* An object created at version 200, when the list was read at 150, is missing from the list. The live add already
  delivered it, so the diff sees a key in the snapshot and not in the list — a delete that never happened.
* An object deleted at version 200, when the list was read at 150, is still in the list. The live delete already
  evicted it, so the diff sees a key in the list and not in the snapshot — an add that resurrects it.

The informer settles this by comparing versions. A stored object newer than the list is carried forward, because the
list's silence about it is stale rather than authoritative; a listed object that a newer delete already evicted is
dropped.

### Groups

`BusInformerConfig.Group` joins the app's replicas into a group across which the bus shares delivery, so each event
reaches one replica rather than all of them. That cuts live volume per replica without cutting correctness. A zero
`Group` — the default — delivers every event to every replica.

`Group.Delivery` says how the bus should share events among the members. Leaving it empty takes whatever the bus
does; `resourcebus.GroupDeliveryRoundRobin` asks for each event to go to one member in turn, with no affinity between
an object and the member handling its events. A `Subscriber` which cannot provide the delivery asked for fails the
subscription rather than substituting another, because the difference decides whether an event reaches one member or
all of them.

### Payload

`BusInformerConfig.Payload` decides what a live event delivers:

* `operator.PayloadIdentity` (the default) delivers the kind's zero value carrying namespace, name, resource version
  and whatever metadata the bus announced with the event. It costs no API request per event and suits a reconciler
  which treats the event as a signal and reads the object itself. Pairing it with an opinionated reconciler or
  watcher needs `AnnouncedMetadata` — see
  [Writing a Reconciler](writing-a-reconciler.md#identity-payloads-and-opinionated-reconcilers).
* `operator.PayloadHydrated` reads the object from the API before dispatching, so handlers receive a full object as
  they would from a watch, at one request per event. An event whose object cannot be read is dropped and reported to
  the error handler; the delete arrives as its own event or on the next re-list.

### Announced metadata

A bus may announce part of an object's metadata with the change, so a consumer can decide what to do without reading
the object. Everything announced is opt-in, and a kind asks for it in the `metadata` half of its `watch` section:

```cue
watch: metadata: {
    labels:            ["app"]
    annotations:       ["myapp.grafana.app/paused"]
    finalizers:        ["myapp.grafana.app/finalizer"]
    generation:        true
    deletionTimestamp: true
}
```

Labels, annotations and finalizers are named individually, because announcing all of them would put most of the
object in every notification. The platform registers from the manifest, so this is the only place it is stated.

Whatever arrives is copied onto the delivered object, on live events and re-list deliveries alike. **An absent field
means unknown, not empty**: an empty finalizer list does not say the object has no finalizers, only that none were
announced — a handler which must tell those apart reads the object.

Pass `simple.AnnouncedMetadataFromManifest(manifest)` to `BusInformerConfig.AnnouncedMetadata` so the informer
expects exactly what the kinds asked for. Declaring `finalizers`, `generation` and `deletionTimestamp` is what lets
an identity payload be paired with an opinionated reconciler or watcher, which read those three off the delivered
object. The informer's configuration is app-wide while it serves one kind at a time, so the weakest declaring kind
governs; a kind which declares nothing is ignored rather than treated as a refusal.

`BusInformerConfig.AnnouncedMetadata` can also be set directly, for a bus registered outside the manifest.

Labels and annotations need no such flag, because nothing in the SDK branches on them — your handler reads them off
the delivered object, and skips the API read when they answer the question:

```go
func (w *watcher) Add(ctx context.Context, obj resource.Object) error {
    if obj.GetLabels()["app"] != "mine" {
        return nil // not ours; no read needed
    }
    if obj.GetAnnotations()["myapp.grafana.app/paused"] == "true" {
        return nil
    }
    full, err := w.client.Get(ctx, resource.Identifier{Namespace: obj.GetNamespace(), Name: obj.GetName()})
    ...
}
```

Both are empty when the bus announced none, which is indistinguishable from the object genuinely having none — so a
handler which must tell those apart reads the object rather than trusting the absence.

### Listing

By default the re-list is a paginated list of full objects over the kind's client. It follows continue tokens to the
end of the set and fails on any error, because a truncated snapshot diffs as a mass deletion.

A server which can return identities alone — namespace, name and resource version, which is all the diff needs —
lists through those instead: implement `operator.ResourceKeyLister` and set `BusInformerConfig.KeyListerFactory`. The
SDK builds the identity objects, records the projection, and falls back to the full-object list for a re-list the
projection cannot serve. Three things follow:

* Your key lister must honor the namespace and filters it is given, and report
  `operator.ErrProjectionUnsupported` when it cannot — an identity carries no labels or fields, so nothing
  downstream can apply a filter it did not.
* Only `operator.ErrProjectionUnsupported` falls back. Any other error fails the re-list, so a transient outage is
  retried next tick rather than served by the expensive path forever.
* `PayloadHydrated` needs full objects, so configuring `KeyListerFactory` with it is rejected: re-lists would carry
  less than live events.

`informer_bus_relist_projection_total{group,version,kind,projection}` reports which projection served each re-list,
and `BusInformerConfig.ListerFactory` replaces the lister outright for a list which fits neither shape.

### What a watcher can observe

A `ResourceWatcher` sees `Add` the same way whether the event came from the live stream or from a re-list, which is
usually what you want. Instrumentation that attributes an event to its cause is the exception, so the informer puts
the distinction on the dispatch context:

```go
func (w *watcher) Add(ctx context.Context, obj resource.Object) error {
    if operator.DeliveryFromContext(ctx) == operator.DeliveryLive {
        // The change source announced this, at the resource version the object carries.
    }
}
```

`DeliveryRelist` covers the initial list, a re-delivery of an unchanged object, and a change the live stream did not
deliver here — all of them carrying a version the lister read rather than one an announcement was made at. A
watch-backed informer reports `DeliveryUnknown`, because a resync replays its cache rather than re-listing.

`BusInformer.Snapshot()` returns the identities the informer last reconciled with storage: everything its most recent
re-list listed, plus the live adds since, minus the live deletes. It is **not** a cache for serving reads — it lags by
up to one re-list, holds only what this informer was delivered, and is the informer's own diffing state. It exists for
a staleness-tolerant count, such as a quota check that would otherwise cost an API list per reconcile. Anything a
reconcile's correctness depends on must be read from the API.

### Read freshness

A change source can announce a write to this replica before the read path can serve it, and a reconcile which reads
a version older than the one it was woken for is acting on state the system already knows is stale. That is what
`operator.FreshnessMetrics` measures, and it is the one thing the informer's own metrics cannot see — the informer
knows what it delivered, not what the handler's read came back with.

```go
freshness := operator.NewFreshnessMetrics(operator.FreshnessMetricsConfig{
    MetricsConfig:       metrics.DefaultConfig("myapp"),
    ResourceVersionTime: decodeVersionTimestamp,
})
tracker := operator.NewAnnouncementTracker(operator.AnnouncementTrackerOptions{})
// Registered before the watchers which act on the events, since informers dispatch in order.
_ = informer.AddEventHandler(tracker.Watcher())

// Around your own read, with the announcement sampled first:
announced := tracker.Announced(id)
obj, err := client.Get(ctx, id)
freshness.Recorder(kind).Observe(ctx, id, announced, obj, err)
```

Two outcomes are excluded by construction, so a non-zero `behind` cannot be dismissed as ordinary concurrency: the
announcement is sampled **before** the read is issued, because a read already in flight when a newer write commits
may legitimately return the earlier state; and a `NotFound` is its own outcome and never `behind`, because an object
deleted after its announcement is correctly absent.

A handler which reads inside the dispatch needs no tracker — `operator.AnnouncedVersionFromContext(ctx)` has the
version. The tracker is for a work queue, where the read happens long after the event that enqueued the key.

### Limits

* **No selectors.** The bus delivers by namespace, so `LabelFilters` and `FieldSelectors` are rejected when the
  informer is built rather than applied to the re-list alone — otherwise live events would keep arriving for excluded
  objects, land in the snapshot, and be diffed as deletes by the next filtered re-list.
* **Events are queued, not handled inline.** The subscriber callback only enqueues; the informer applies and
  dispatches on its own goroutine, so a slow API read or handler cannot stall the bus. `EventQueueSize` bounds the
  queue, and a full queue drops the event and forces a re-list to recover it — counted by
  `informer_bus_dropped_events_total`, which climbing means the informer is not keeping up. Those recovery re-lists
  are rate-limited to `OverflowRelistInterval`, since one list recovers every drop before it, and the informer
  applies up to `EventDrainBatch` queued events per pass so a backlog cannot keep scheduling lists instead of being
  worked off.
* **One order for live and re-list deliveries.** Both are applied on that same goroutine, so a live delete cannot
  land between a re-list computing its diff and delivering it.
* **A completed re-list supersedes older queued events.** A list read after an event was published already accounts
  for it, so applying it afterwards would roll the snapshot back — a queued delete would evict a re-creation the list
  saw, a queued add would resurrect a key it confirmed absent. Such events are dropped and counted by
  `informer_bus_superseded_events_total`. Opaque resource versions disable the comparison, as they do the snapshot's.

### Starting without the bus

By default an informer does not list, and so does not report synced, until its subscription is open **and ready to
receive**: re-listing a kind it cannot watch would start your reconcilers against a snapshot with no live updates
until the next re-list. A subscription which is accepted but does not become ready within
`SubscriptionReadyTimeout` is closed and retried, since accepting is not delivering.

Set `BusInformerConfig.AllowDegradedStart` to list and start reconciling anyway, with the subscription retrying in
the background and a re-list covering the gap once it opens. Opt in when progress must not stall with the bus.

### Resource versions

The contract carries resource versions as opaque strings, because that is all the Kubernetes API guarantees. But
settling a re-list against the live events that raced it means comparing them, and comparing them needs numbers. The
SDK parses them in two places only:

* **That comparison.** A version which does not parse reads as zero, which switches the comparison off: the informer
  does a plain wholesale swap instead, taking each list as authoritative. A live add the list is too old to include
  is then diffed as a delete, and a live delete the list still lists is diffed as an add — in both cases the next
  re-list corrects it. Nothing else about the informer changes, so an opaque version costs a redundant event per
  racing change, not correctness.
* **The delivery latency histograms**, which date a change from the timestamp its version embeds. There is no
  portable way to decode that, so supply `BusInformerConfig.ResourceVersionTime`. Without it the histograms stay
  empty; the event counters are unaffected either way.

## The periodic informer

`simple.PeriodicInformerSupplier` creates an `operator.PeriodicInformer`: no watch, no subscription and no snapshot,
just a re-list on a cadence delivering every listed object as an add. It suits idempotent, cadence-driven work which
re-evaluates every object on each pass regardless of whether it changed — age-based cleanup, for instance. For
anything which should react to a change promptly, use one of the informers above.

It accepts `Namespace`, `LabelFilters` and `FieldSelectors` and passes them to the list request, where the server
applies them. The bus informer rejects selectors because its live stream delivers by namespace alone and cannot
honor them; here the list is the only source, so there is nothing for a filtered list to disagree with.

## Writing your own

An informer is anything implementing `operator.Informer`: `Run`, `WaitForSync` and `AddEventHandler`. Supply it
through an `InformerSupplier` and the `InformerController` treats it like any other. If it implements
`metrics.Provider` or `health.Check`, its collectors and health check are picked up with the rest of the app's.
