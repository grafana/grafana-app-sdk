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
kind with `ReconcileOptions.InformerSupplier` (the per-kind one wins). The SDK ships three:

| Supplier | Change source | Memory |
| --- | --- | --- |
| `simple.DefaultInformerSupplier` | kubernetes watch | every object of the kind |
| `simple.OptimizedInformerSupplier` | kubernetes watch | every object, in a cache tuned for large counts |
| `simple.BusInformerSupplier` | a `resourcebus.Subscriber` | identity per object |

`DefaultInformerSupplier` is the one to use unless you have a reason not to. Reach for
`OptimizedInformerSupplier` when a kind has more than ~10k objects, you watch cluster-scoped resources, or you are
memory constrained; it respects `UseWatchList` and `WatchListPageSize`, which the default supplier ignores. Reach for
`BusInformerSupplier` when your reconcilers read the object they act on anyway, so the cache earns nothing — see
below.

> [!CAUTION]
> The bus informer, the periodic informer and the `resourcebus` contract are **experimental**. Their Go APIs may
> change or be removed in any release. The watch-backed suppliers are not affected.

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

* `operator.PayloadIdentity` (the default) delivers the kind's zero value with namespace, name and resource version
  set, and nothing else. It costs no API request per event and suits a reconciler which treats the event as a signal
  and reads the object itself. It **cannot** be combined with an opinionated reconciler or watcher — see
  [Writing a Reconciler](writing-a-reconciler.md#identity-payloads-and-opinionated-reconcilers).
* `operator.PayloadHydrated` reads the object from the API before dispatching, so handlers receive a full object as
  they would from a watch, at one request per event. An event whose object cannot be read is dropped and reported to
  the error handler; the delete arrives as its own event or on the next re-list.

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
* `PayloadHydrated` needs full objects, or re-list deliveries would carry less than live ones.

`informer_bus_relist_projection_total{group,version,kind,projection}` reports which projection served each re-list,
and `BusInformerConfig.ListerFactory` replaces the lister outright for a list which fits neither shape.

### Starting without the bus

By default an informer does not list, and so does not report synced, until its subscription opens: re-listing a kind
it cannot watch would start your reconcilers against a snapshot with no live updates until the next re-list. Set
`BusInformerConfig.AllowDegradedStart` to list and start reconciling anyway, with the subscription retrying in the
background and a re-list covering the gap once it opens. Opt in when progress must not stall with the bus.

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

`operator.PeriodicInformer` has no watch and no subscription, and keeps no snapshot: each pass lists the kind and
delivers every object as an add. It suits idempotent, cadence-driven work which re-evaluates every object on each
pass regardless of whether it changed — age-based cleanup, for instance. For anything which should react to a change
promptly, use one of the informers above.

## Writing your own

An informer is anything implementing `operator.Informer`: `Run`, `WaitForSync` and `AddEventHandler`. Supply it
through an `InformerSupplier` and the `InformerController` treats it like any other. If it implements
`metrics.Provider` or `health.Check`, its collectors and health check are picked up with the rest of the app's.
