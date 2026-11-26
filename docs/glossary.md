# App Platform Glossary (draft for discussion)

> **Status: proposal.** This page lists the terms the SDK uses for background work and event handling, what each one means today, and what it means in Kubernetes and controller-runtime. Every **Proposal** below is a suggestion for team discussion. None of them is an agreed decision yet, and no code has been renamed.

## Why this page exists

Recent work made our vocabulary visible:

- [#1717](https://github.com/grafana/grafana-app-sdk/pull/1717) adds *named runnables* and `GF_APP_TARGET`. The only built-in name is `informers` (`plugin/run.go`, `RunnerNameInformers`), but that runnable also runs every reconciler and watcher.
- [#1708](https://github.com/grafana/grafana-app-sdk/pull/1708) rewrites the "watch needs" section of `docs/writing-a-reconciler.md` and adds a small glossary of its own (*delivery*, *live channel*, *resync channel*, *watch mechanism*).

Points raised in the discussion:

- The SDK uses "informer" for the part that *delivers notifications*, not for the part that *handles* them.
- We have watchers, reconcilers, operators, controllers, informers, mechanisms and suppliers, and the names overlap.
- "Operator" has too many meanings.
- All of these are **background tasks that start at startup**. They may or may not use Kubernetes tooling. "Runner" is a good umbrella word.

## Quick reference (proposed meanings)

| Term | Proposed one-line meaning |
|---|---|
| Runnable | Anything with `Run(ctx) error` that the app starts at startup. A background task. |
| Runner | Something that runs one or more Runnables (lifecycle, errors, shutdown). |
| Named runnable / target | A Runnable with a stable name, so a deployment can choose to run it or not. |
| Informer | Produces notifications (deliveries) about a kind's objects. It does not handle them. |
| Source | Proposed new umbrella word for "anything that produces deliveries" (informer, event bus, periodic lister). |
| Mechanism | A source that also says what it provides (`InformerMechanism`). |
| Supplier | A factory function that builds an informer for a kind (`InformerSupplier`). |
| Handler | Code that receives deliveries. Watchers and reconcilers are both handlers. |
| Watcher | A handler with separate `Add` / `Update` / `Delete` methods. |
| Reconciler | A handler with one `Reconcile` method that moves an object toward its desired state. |
| Controller | Something that connects sources to handlers and adds retries. Avoid as a public word (see below). |
| Operator | A deployment shape: a standalone process that runs an app against a Kubernetes API. Avoid as a type name. |
| Delivery | One notification passed to a handler. |
| Resync | A periodic re-delivery of every object, even if nothing changed. |
| Redelivery / retry | Delivering the same event again because the handler failed or asked for it. |

---

## Background tasks

### Runnable

**Current usage**

- `app.Runnable` (`app/app.go`): `Run(context.Context) error`. It blocks until an error, until the context ends, or until the work is done.
- Embedded in `operator.Informer` (`operator/informer_controller.go`) and `operator.Controller` (`operator/operator.go`).
- `simple.App.AddRunnable` and, from #1717, `AddNamedRunnable` (`simple/app.go`).
- Private `k8sRunnable` wrappers in `simple/app.go` and `operator/runner.go` adapt controller-runtime runnables.

**Kubernetes / controller-runtime**: `manager.Runnable` has `Start(ctx) error`, and the manager starts every runnable it has. This is the same idea as ours. dskit (Grafana/Mimir) calls the same thing a **service**, with explicit states (starting, running, stopping). #1717 plans to wrap runnables as dskit services later.

**Options**: Runnable, Service, Task / Background task, Worker.

**Debate**

- *Runnable* is already public, matches controller-runtime, and says only "it can be run". That is all the interface promises.
- *Service* (dskit) adds lifecycle states. That is useful, but it conflicts with Kubernetes `Service` and with "service" in Grafana's backend.
- *Background task* is the clearest English, and Ryan used it. But it suggests something short-lived, while most of ours run forever.

**Proposal**: Keep **Runnable** as the type name. In prose, define it as "a background task the app starts at startup". Use "service" only for the dskit wrapper.

### Runner

**Current usage** (the same word means three different things):

- `app.Provider` / `App.Runner()` (`app/app.go`) returns the app's single top-level Runnable.
- `app.MultiRunner`, `SingletonRunner`, `DynamicMultiRunner` (`app/runner.go`) run several Runnables together.
- `operator.Runner` (`operator/runner.go`) runs a whole `app.App` as a standalone operator, with webhooks and metrics.
- `app.NamedRunnerProvider.NamedRunners()` (#1717) returns `map[string]Runnable`. The method says "runner", but the values are Runnables.

**Options**: Runner, Manager (controller-runtime), Supervisor, Host.

**Debate**

- *Runner* reads well and Ryan likes it. However, `App.Runner()` returns something that is *run*, not something that *runs*. That confuses the two words.
- *Manager* is what controller-runtime uses for "runs everything and shares caches". It is precise, but it pulls in controller-runtime expectations (leader election, a shared cache).
- *Host* matches how `plugin.Run` and `operator.Runner` work: they host an app inside a process.

**Proposal**: **Runnable** = the thing being run. **Runner** = the thing that runs Runnables (the `MultiRunner` family). Over time, rename the method `App.Runner()` to something like `App.Runnable()`. For process-level wrappers (`operator.Runner`, `plugin.Run`), consider the word **host** in docs. Rename `NamedRunners()` to `NamedRunnables()` before #1717 ships a stable API.

### Named runnable / target

**Current usage**: #1717 adds `NamedRunnerProvider`, `WithTarget(...)` and `GF_APP_TARGET` (`plugin/run.go`). `all` and `none` are reserved. `informers` is built in.

**Kubernetes**: there is no direct equivalent. Mimir and Loki use `-target` for the same purpose (one binary, several deployable roles).

**Debate**: The name `informers` describes the *source* side, but it actually runs the handlers too (reconcilers and watchers). An operator who sets `GF_APP_TARGET=informers` may think they are only turning on caches. Other options: `reconcilers`, `handlers`, `background`, `controllers`.

**Proposal**: Keep **target** (it matches Mimir/Loki). Rename the built-in target to something about *handling*, for example `handlers`, or keep `informers` as an alias. Decide this before #1717 is used widely.

---

## Producing events

### Informer

**Current usage**

- `operator.Informer` (`operator/informer_controller.go`): a Runnable with `WaitForSync` and `AddEventHandler(ResourceWatcher)`.
- Implementations: `NewKubernetesBasedInformer` (`operator/informer_kubernetes.go`), `NewCustomCacheInformer` / `NewMemcachedInformer` (`operator/informer_customcache.go`), `NewConcurrentInformer` (`operator/informer_concurrent.go`, a wrapper that adds concurrency), `NewPeriodicInformer` (`operator/informer_periodic.go`, which re-lists and has no watch).
- `operator.InformerController` and `simple.AppInformerConfig` (`simple/app.go`).

**Kubernetes / client-go**: a `SharedInformer` = reflector (list + watch) + local cache (indexer) + event handler fan-out. It always keeps a cache. Handlers are registered on it, but they are not part of it.

**Debate**

- This matches Roberto's point: the informer *notifies*, it does not *handle*. That is consistent with client-go.
- But our informers are not always client-go informers. `PeriodicInformer` has no watch. Some may have no cache. An event-bus informer may only send names. So "informer" promises more than the interface does.
- The name is everywhere in the public API, so renaming it is expensive.

**Proposal**: Keep **Informer** as the type name. Define it as "a source of deliveries for one kind; it may or may not keep a cache or use a Kubernetes watch". Never use "informer" to mean the handlers or the whole background process.

### Source (proposed new term)

**Current usage**: The SDK has no `Source` type today. The word only appears as `SourceGVK` in conversion code (`simple/app.go`), which is unrelated.

**controller-runtime**: `source.Source` (`source.Kind`, `source.Channel`) is "where events come from". A controller is built from sources + handlers + a reconciler.

**Debate**

- *Source* is neutral. It covers a Kubernetes watch, an event bus, a periodic list and a timer. It avoids promising a cache, which "informer" does.
- Adding a word adds vocabulary. If "informer" is redefined broadly (see above), we may not need "source".
- It conflicts with `SourceGVK` in conversion, but only in a small way.

**Proposal**: Use **source** in *prose* as the umbrella for "anything that produces deliveries". Informers are one kind of source. Do not add a Go type unless we add non-informer sources.

### Mechanism (watch mechanism)

**Current usage**: `operator.InformerMechanism` and `NewInformerMechanism` (`operator/informer_mechanism.go`). #1708 defines a *watch mechanism* as a way of watching a kind that says what it provides (`WatchOptions`), so the app can choose between several.

**Debate**: "Mechanism" is accurate but abstract. It is really "a source plus a description of what it provides". If we adopt *source*, "described source" or "source with capabilities" is an alternative, but "mechanism" is already in an experimental API and is short.

**Proposal**: Keep **mechanism** while the API is experimental, defined as "a source that advertises what it delivers". Revisit when it stops being experimental.

### Supplier

**Current usage**: `operator.InformerSupplier` (`operator/informer_mechanism.go`) is a function `(kind, clients, options) -> (Informer, error)`. Also `simple.DefaultInformerSupplier` / `OptimizedInformerSupplier`, `OperatorConfig.InformerSupplier` (`simple/operator.go`), and `operator.FinalizerSupplier` (`operator/opinionatedwatcher.go`).

**Kubernetes**: client-go uses **factory** (`SharedInformerFactory`). "Supplier" is a Java term (`java.util.function.Supplier`) and is not used in Kubernetes Go code.

**Debate**: *Factory* is the Go and Kubernetes word and is immediately clear. *Supplier* is already public in several places. Note that `FinalizerSupplier` does not build anything, it returns a string, so "supplier" already means two things.

**Proposal**: In prose, describe suppliers as **factories**. Keep the type names for now. If an API is renamed, prefer `InformerFactory`.

---

## Handling events

### Handler (proposed umbrella term)

**Current usage**: The docs use "handlers" informally. #1708 uses "the kind's handlers" for watchers and reconcilers together. `Informer.AddEventHandler` takes a `ResourceWatcher`.

**client-go**: `ResourceEventHandler` (`OnAdd` / `OnUpdate` / `OnDelete`). controller-runtime: `handler.EventHandler` maps events to reconcile requests.

**Proposal**: Use **handler** as the umbrella for "code that receives deliveries". Watchers and reconcilers are both handlers.

### Watcher

**Current usage**

- `operator.ResourceWatcher` (`operator/informer_controller.go`): `Add`, `Update(src, tgt)`, `Delete`.
- `operator.OpinionatedWatcher` (`operator/opinionatedwatcher.go`) adds finalizers and `Sync`. `simple.Watcher` (`simple/watcher.go`), `simple.SyncWatcher` (`simple/operator.go`), and the private `concurrentWatcher` (`operator/concurrentwatcher.go`).
- `AppManagedKind.Watcher` and `AppUnmanagedKind.Watcher` in `simple/app.go`.

**Kubernetes**: In Kubernetes, a **watch** is the *producer side*: `watch.Interface` and the `?watch=true` API call that streams changes. A "watcher" is the *client that opens a watch*. In the SDK, a "watcher" is the *consumer* that handles events. These meanings are opposite.

**Debate**

- Users understand "watcher" quickly ("it watches my kind").
- For anyone who knows Kubernetes, "watcher" means the thing that calls the watch API, which is the informer's job. This is the main source of confusion.
- Alternatives: `EventHandler` (client-go), `ResourceHandler`, `ChangeHandler`.

**Proposal**: Keep **Watcher** as a type name, because it is public and widely used. Always define it as "a handler with per-action methods (Add / Update / Delete)". In new APIs, prefer *handler*. See the section on "watch" below.

### Reconciler

**Current usage**: `operator.Reconciler`, `OpinionatedReconciler`, `SimpleReconciler`, `ReconcileResult.RequeueAfter` (`operator/reconciler.go`), `simple.Reconciler` (`simple/reconciler.go`), `docs/writing-a-reconciler.md`.

**controller-runtime**: `reconcile.Reconciler` receives only a *key* (namespace / name), reads the current state itself, and must be level-based and idempotent.

**Debate**: The SDK passes the *object* and the *action* (`ReconcileRequest`), which is closer to edge-triggered handling. This is convenient, but it means our reconcilers are not exactly controller-runtime reconcilers. The word is correct in spirit (moving toward the desired state). The behavior differs in the details.

**Proposal**: Keep **Reconciler**. Define it as "a handler with one method that moves an object toward its desired state; it must be idempotent". In the docs, say clearly that it receives the object, not only a key.

### Controller

**Current usage** (overloaded):

- `operator.Controller` (`operator/operator.go`) is just `app.Runnable`.
- `operator.InformerController` (`operator/informer_controller.go`) connects informers to watchers and reconcilers, with retries and the dequeue policy.
- `k8s/cache.Controller` (`k8s/cache/controller.go`) is a fork of the client-go cache controller (reflector + DeltaFIFO).
- `resource.ValidatingAdmissionController` / `MutatingAdmissionController` (`resource/admission.go`) and `k8s.Opinionated*AdmissionController` (`k8s/admission.go`) are admission hooks, not control loops.

**Kubernetes**: A controller is a control loop that watches state and moves it toward the desired state (kube-controller-manager). In controller-runtime, a `Controller` = sources + a work queue + one reconciler.

**Debate**: The word has four meanings in our code. `InformerController` is the closest to the Kubernetes meaning. "Admission controller" is a Kubernetes term too, but it means something completely different.

**Proposal**: Do not use **controller** as an umbrella word in docs. When a precise name is needed, say "the informer controller" for `InformerController`. Treat `operator.Controller` as a deprecated alias of `app.Runnable`.

### Operator

**Current usage**: `operator.Operator` (`operator/operator.go`) is a group of Controllers (Runnables). `operator.Runner` (`operator/runner.go`) runs an app as a standalone process with webhooks. `simple.Operator` / `simple.NewStandaloneOperator` (`simple/operator.go`). And the whole `operator` package, which also holds informers, watchers and reconcilers.

**Kubernetes**: The "operator pattern" = custom resources + a controller that encodes how a human would operate the software. It is a pattern, not a component.

**Debate**: As Steve said, it has too many meanings: a package, a type, a deployment, and a pattern. In Grafana, many apps run inside Grafana and not as an operator at all, so "operator" also wrongly suggests a separate process.

**Proposal**: Use **operator** only for the *deployment shape* ("run the app as a standalone operator"). Avoid it as a name for a component. Describe `operator.Operator` as a Runner of Runnables (it overlaps with `app.MultiRunner`), and consider deprecating it.

---

## Delivery semantics

### Delivery, live channel, resync channel

**Current usage**: `operator.WatchOptions`, `WatchLiveOptions`, `WatchResyncOptions`, `WatchDetail`, `WatchDistribution` (`operator/watchoptions.go`). #1708 defines *delivery* (one notification to a handler), the *live channel* (changes as they happen), and the *resync channel* (periodic re-delivery of every object).

**Proposal**: Adopt these as defined in #1708. They are precise and do not depend on Kubernetes.

### Resync

**Current usage**: `ResourceActionResync` (`operator/informer_controller.go`), `WatchResyncOptions` (`operator/watchoptions.go`), resync periods in the Kubernetes-based and custom-cache informers, and `PeriodicInformer`, which is effectively resync-only.

**Kubernetes**: In client-go, the "resync period" replays the *local cache* to the handlers as update events where old == new. It does **not** re-list from the API server. Controller-runtime documents it as rarely needed. "Relist" (a new LIST call to the API) is a different thing.

**Debate**: Our `WatchResyncOptions.Detail` comment says a resync may "re-list" for some mechanisms. This mixes up two Kubernetes ideas: replaying the cache and listing again. Both give the same result to a handler, but they cost very different amounts.

**Proposal**: **Resync** = "periodic re-delivery of every object to handlers, whether or not it changed". How it is implemented (cache replay or relist) is a property of the mechanism. Use **relist** only for the implementation.

### Redelivery / retry / requeue

**Current usage**: `RetryPolicy`, `ExponentialBackoffRetryPolicy`, `DefaultRetryPolicy` (`operator/informer_controller.go`), `ReconcileResult.RequeueAfter` (`operator/reconciler.go`). `WatchResyncOptions` comments call resync "re-delivery".

**Kubernetes**: controller-runtime uses **requeue** (`Result{Requeue, RequeueAfter}`) and the rate-limited work queue for retries after errors.

**Debate**: "Redelivery" is used both for resync (every object, on a timer) and could mean retrying one failed event. These are different things.

**Proposal**: **Retry** = delivering the same event again after a handler error (`RetryPolicy`). **Requeue** = the handler asks to be called again later (`RequeueAfter`). Do not use **redelivery** for resync. If we need a word that covers both retry and requeue, use **redelivery** only for that.

---

## Should we use the word "watch" at all?

**Where it appears today**: `ResourceWatcher`, `OpinionatedWatcher`, `simple.Watcher`, `WatchOptions` and the `Watch*` types, `UseWatchList` / `WatchListPageSize`, `resource.Client.Watch`, `docs/watching-unowned-resources.md`, and the #1708 section "Declaring What a Reconciler Needs From a Watch".

**Argument for keeping it**

- "Watch" is what users want to say: "my app watches the Dashboard kind". It is short and easy to understand.
- `resource.Client.Watch` and watch-list really are Kubernetes watches, and that use is correct.
- Removing it from public API names would be a large breaking change.

**Argument for dropping it (except for the real Kubernetes watch)**

- In Kubernetes, *watch* is one specific API call. Our "watch" may be a periodic list or an event bus, so it promises a transport we may not use.
- The same word means the producer (`Client.Watch`), the consumer (`Watcher`) and the requirements (`WatchOptions`). A reader cannot tell which one is meant.
- Ryan's point applies here: this work may not use Kubernetes tooling at all.

**Alternatives**: "subscribe / subscription" (neutral and common for event buses), "deliveries" (from #1708), "observe", "handle".

**Proposal**

1. Keep **watch** as a *verb in user-facing prose* ("your app watches a kind"), meaning "receives deliveries about it".
2. Use **Kubernetes watch** (or "watch API") when we mean the actual API call.
3. Do not add new type names that start with `Watch` / `Watcher`. Prefer *delivery*, *handler*, *source*. For example, `WatchOptions` could become `DeliveryOptions` before it stops being experimental.
4. Do not rename existing stable types (`ResourceWatcher`, `simple.Watcher`). Define them clearly instead.

---

## Other terms found in the code

| Term | Where | Note / proposal |
|---|---|---|
| Opinionated (watcher / reconciler / admission controller) | `operator/opinionatedwatcher.go`, `operator/reconciler.go`, `k8s/admission.go` | Means "adds finalizers and skips no-op updates". The name does not say what it does. Consider "finalizer-aware" in docs. |
| Sync (`SyncWatcher`, `OpinionatedWatcher.Sync`) | `simple/operator.go`, `operator/opinionatedwatcher.go` | A catch-up call for objects that may have changed while the app was down. This is different from **resync**. Say "startup sync" in docs to avoid confusion. |
| `WaitForSync` / HasSynced | `operator/informer_controller.go` | The client-go meaning: the initial list is fully delivered. Fine as is. |
| Managed / unmanaged kind | `simple/app.go` (`AppManagedKind`, `AppUnmanagedKind`) | Owned by the app (with admission and conversion) or only watched. See `docs/watching-unowned-resources.md`, which says "unowned". Pick one word ("unmanaged" matches the code). |
| Provider | `app.Provider`, `NamedRunnerProvider` (`app/app.go`), `metrics.Provider` | Builds or exposes things. Usually fine, but `NamedRunnerProvider` should match the Runnable rename. |
| Concurrent (informer / watcher) | `operator/informer_concurrent.go`, `operator/concurrentwatcher.go` | A wrapper that handles events for different objects in parallel. Fine. |
| Dequeue policy / event queue | `operator/informer_controller.go` | An internal queue in the informer controller. Not a Kubernetes work queue. Document it if it becomes public. |
| Distribution: all / shared / sharded | `operator/watchoptions.go` | Defined in #1708. Adopt. |
| Detail: full object / identity | `operator/watchoptions.go` | Defined in #1708. Adopt. |
| Admission controller | `resource/admission.go`, `k8s/admission.go` | Kubernetes term for validate/mutate hooks. Not a control loop. Keep, but never shorten to "controller". |

## Open questions for the team

1. Do we want *source* as an official word, or do we broaden *informer* instead?
2. Should the `informers` target be renamed before #1717 is used widely?
3. Rename `NamedRunners()` to `NamedRunnables()` now, while it is new?
4. Rename `WatchOptions` and related types before they stop being experimental?
5. Deprecate `operator.Operator` and `operator.Controller` in favor of `app.MultiRunner` and `app.Runnable`?
