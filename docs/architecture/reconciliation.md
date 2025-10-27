# Reconciliation Architecture

This document provides detailed visual representations of the reconciliation code paths in the grafana-app-sdk, tracing the complete flow from application startup through event processing.

## Overview

The reconciliation system in grafana-app-sdk follows a Kubernetes-inspired operator pattern with two primary flows:

1. **Initial Sync (Cold Start)**: The complete bootstrapping process from application start to processing the initial LIST of resources
2. **Event Handling (Hot Path)**: The ongoing watch loop that processes CREATE, UPDATE, and DELETE events

These diagrams show the exact code paths with file locations, making it easier to understand and debug the reconciliation flow.

---

## Diagram 1: Initial Sync (Cold Start) - Complete Code Path

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          COLD START / INITIAL SYNC                          │
└─────────────────────────────────────────────────────────────────────────────┘

1. User Application Start
┌──────────────────────────────────────────────────────────────────┐
│ main()                                                           │
│ examples/operator/simple/reconciler/main.go:88-98                │
│ • Creates context with signal handling                           │
│ • Calls runner.Run(ctx, simple.NewAppProvider(...))             │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
2. Operator Runner Setup
┌──────────────────────────────────────────────────────────────────┐
│ Runner.Run()                                                     │
│ operator/runner.go:129-261                                       │
│ • Gets manifest data (line 135-137)                             │
│ • Creates app.Config (line 139-143)                             │
│ • Calls provider.NewApp(appConfig) (line 146-149)              │
│ • Gets runner from app (line 232-235)                           │
│ • Starts multi-runner (line 261)                                │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
3. User's App Factory
┌──────────────────────────────────────────────────────────────────┐
│ NewApp()                                                         │
│ examples/operator/simple/reconciler/main.go:106-133              │
│ • Creates Reconciler with ReconcileFunc (line 108-118)          │
│ • Calls simple.NewApp() with config (line 121-132)             │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
4. Simple App Initialization
┌──────────────────────────────────────────────────────────────────┐
│ simple.NewApp()                                                  │
│ simple/app.go:306-373                                            │
│ • Creates ClientRegistry (line 308-310)                         │
│ • Creates InformerController (line 312)                         │
│ • Loops through ManagedKinds (line 341-352)                    │
│ • Calls a.manageKind() for each (line 342)                     │
│ • Adds InformerController to runner (line 371)                 │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
5. Kind Management
┌──────────────────────────────────────────────────────────────────┐
│ App.manageKind()                                                 │
│ simple/app.go:432-465                                            │
│ • Registers custom routes (line 436-450)                        │
│ • If reconciler/watcher exists, calls watchKind() (line 453)   │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
6. Watch Setup
┌──────────────────────────────────────────────────────────────────┐
│ App.watchKind()                                                  │
│ simple/app.go:468-533                                            │
│ • Gets InformerSupplier (line 473-476)                         │
│ • Sets ListWatchOptions (line 478-483)                         │
│ • Calls infSupplier() to create informer (line 485)           │
│ • Adds informer to controller (line 489)                       │
│ • Wraps reconciler in OpinionatedReconciler if needed (496)   │
│ • Adds reconciler to controller (line 503)                     │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
7. Informer Registration
┌──────────────────────────────────────────────────────────────────┐
│ InformerController.AddInformer()                                 │
│ operator/informer_controller.go:241-261                          │
│ • Creates SimpleWatcher with Add/Update/Delete funcs (line 249) │
│ • Adds event handler to informer (line 249-256)                │
│ • Adds informer to runner (line 258)                            │
│ • Stores informer in map (line 259)                             │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
8. Reconciler Registration
┌──────────────────────────────────────────────────────────────────┐
│ InformerController.AddReconciler()                               │
│ operator/informer_controller.go:302-311                          │
│ • Stores reconciler in map (line 309)                           │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
9. Controller Start
┌──────────────────────────────────────────────────────────────────┐
│ InformerController.Run()                                         │
│ operator/informer_controller.go:328-335                          │
│ • Starts retry ticker goroutine (line 333)                     │
│ • Calls c.runner.Run(ctx) which starts all informers (line 334)│
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
10. Informer Execution
┌──────────────────────────────────────────────────────────────────┐
│ CustomCacheInformer.Run()                                        │
│ operator/informer_customcache.go:154-199                         │
│ • Creates controller with newInformer() (line 169-178)         │
│ • Starts processor (line 188)                                   │
│ • Starts controller.Run() (line 197)                            │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
11. Cache Controller Start
┌──────────────────────────────────────────────────────────────────┐
│ Controller.RunWithContext()                                      │
│ k8s/cache/controller.go:65-104                                   │
│ • Creates Reflector with options (line 79-90)                  │
│ • Sets WatchListPageSize (line 92)                             │
│ • Starts reflector in goroutine (line 100)                     │
│ • Starts processLoop() (line 102)                               │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
12. Initial LIST Operation
┌──────────────────────────────────────────────────────────────────┐
│ Reflector.RunWithContext()                                       │
│ k8s/cache/reflector.go (Kubernetes client-go code)              │
│ • Performs initial LIST request to API server                   │
│ • Populates DeltaFIFO queue with all existing objects          │
│ • Each object gets Delta type: Sync/Replaced                    │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
13. Process Initial Objects
┌──────────────────────────────────────────────────────────────────┐
│ Controller.processLoop()                                         │
│ k8s/cache/controller.go:122-139                                  │
│ • Pops deltas from queue (line 131)                            │
│ • Calls Process function for each delta                         │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
14. Delta Processing
┌──────────────────────────────────────────────────────────────────┐
│ processDeltas()                                                  │
│ operator/informer_customcache.go:486-521                         │
│ • For Sync/Replaced/Added/Updated: (line 497-511)              │
│   - Adds object to cache store (line 507)                      │
│   - Calls handler.OnAdd(obj, isInInitialList=true) (line 510) │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
15. Event Distribution
┌──────────────────────────────────────────────────────────────────┐
│ CustomCacheInformer.OnAdd()                                      │
│ operator/informer_customcache.go:232-236                         │
│ • Distributes add event to processor (line 233-236)            │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
16. Event Handler Invocation
┌──────────────────────────────────────────────────────────────────┐
│ toResourceEventHandlerFuncs.AddFunc()                            │
│ operator/informer_customcache.go:350-375                         │
│ • Creates context (line 351-352)                                │
│ • Transforms object to resource.Object (line 356)              │
│ • Calls handler.Add(ctx, cast) (line 370)                      │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
17. Controller Event Handling
┌──────────────────────────────────────────────────────────────────┐
│ InformerController.informerAddFunc()                             │
│ operator/informer_controller.go:391-443                          │
│ • Starts event metrics (line 398)                              │
│ • For each watcher: (line 404-425)                             │
│   - Dequeues retries if needed (line 409)                      │
│   - Calls watcher.Add() (line 412)                             │
│ • For each reconciler: (line 427-440)                          │
│   - Dequeues retries if needed (line 432)                      │
│   - Creates ReconcileRequest (line 435-438)                    │
│   - Calls doReconcile() (line 439)                             │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
18. Reconcile Execution
┌──────────────────────────────────────────────────────────────────┐
│ InformerController.doReconcile()                                 │
│ operator/informer_controller.go:570-617                          │
│ • Updates metrics (line 573-582)                                │
│ • Calls reconciler.Reconcile(ctx, req) (line 587)              │
│ • Handles retry logic (line 592-616)                            │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
19. User Reconciliation Logic
┌──────────────────────────────────────────────────────────────────┐
│ ReconcileFunc()                                                  │
│ examples/operator/simple/reconciler/main.go:109-116              │
│ • User's business logic executes                                │
│ • Logs the reconciliation action                                │
│ • Returns ReconcileResult                                        │
└──────────────────────────────────────────────────────────────────┘
```

---

## Diagram 2: Event Handling (Create/Update/Delete) - Ongoing Operations

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    ONGOING EVENT HANDLING (HOT PATH)                        │
└─────────────────────────────────────────────────────────────────────────────┘

1. Kubernetes Watch Stream
┌──────────────────────────────────────────────────────────────────┐
│ Reflector Watch Loop                                             │
│ k8s/cache/reflector.go (Kubernetes client-go)                    │
│ • Maintains long-lived WATCH connection to API server           │
│ • Receives watch events: ADDED, MODIFIED, DELETED               │
│ • Adds events to DeltaFIFO queue as Add/Update/Delete deltas   │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
2. Queue Processing
┌──────────────────────────────────────────────────────────────────┐
│ Controller.processLoop()                                         │
│ k8s/cache/controller.go:122-139                                  │
│ • Continuously pops events from DeltaFIFO (line 131)            │
│ • Calls Process function with deltas                            │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
3. Delta Type Handling
┌──────────────────────────────────────────────────────────────────┐
│ processDeltas()                                                  │
│ operator/informer_customcache.go:486-521                         │
│ • Processes deltas oldest to newest (line 494)                  │
│ • For Added/Updated: (line 497-511)                             │
│   - Updates cache store (line 502/507)                          │
│   - Calls OnUpdate() or OnAdd() (line 505/510)                 │
│ • For Deleted: (line 512-516)                                   │
│   - Removes from cache (line 513)                               │
│   - Calls OnDelete() (line 516)                                 │
└────────────────────────────┬─────────────────────────────────────┘
                             │
               ┌─────────────┴─────────────┐
               │                           │
               ▼                           ▼
    ╔════════════════╗          ╔════════════════╗
    ║   ADD EVENT    ║          ║ UPDATE EVENT   ║
    ╚════════════════╝          ╚════════════════╝
               │                           │
               ▼                           ▼
4a. Add Event Distribution    4b. Update Event Distribution
┌──────────────────────────┐  ┌──────────────────────────┐
│ OnAdd(obj, false)        │  │ OnUpdate(oldObj, newObj) │
│ informer_customcache.go: │  │ informer_customcache.go: │
│   232-236                │  │   240-244                │
└──────────┬───────────────┘  └──────────┬───────────────┘
           │                             │
           ▼                             ▼
5a. Add Handler              5b. Update Handler
┌──────────────────────────┐  ┌──────────────────────────┐
│ AddFunc()                │  │ UpdateFunc()             │
│ informer_customcache.go: │  │ informer_customcache.go: │
│   350-375                │  │   376-408                │
│ • Creates context        │  │ • Creates context        │
│ • Transforms to          │  │ • Transforms old & new   │
│   resource.Object        │  │   to resource.Object     │
│ • Calls handler.Add()    │  │ • Calls handler.Update() │
└──────────┬───────────────┘  └──────────┬───────────────┘
           │                             │
           └─────────────┬───────────────┘
                         │
                         ▼
6. Controller Routing
┌──────────────────────────────────────────────────────────────────┐
│ InformerController Event Functions                               │
│ operator/informer_controller.go:391-557                          │
│                                                                  │
│ • informerAddFunc() (line 391-443) for CREATE                   │
│ • informerUpdateFunc() (line 446-498) for UPDATE                │
│ • informerDeleteFunc() (line 501-557) for DELETE                │
│                                                                  │
│ Each function:                                                   │
│ 1. Starts event metrics                                          │
│ 2. Iterates through registered watchers                          │
│ 3. Iterates through registered reconcilers                       │
└────────────────────────────┬─────────────────────────────────────┘
                             │
               ┌─────────────┴─────────────┐
               │                           │
               ▼                           ▼
7a. Watcher Processing       7b. Reconciler Processing
┌──────────────────────────┐  ┌──────────────────────────┐
│ watchers.Range()         │  │ reconcilers.Range()      │
│ informer_controller.go:  │  │ informer_controller.go:  │
│   404-425 (Add)          │  │   427-440 (Add)          │
│   459-480 (Update)       │  │   482-495 (Update)       │
│   514-538 (Delete)       │  │   540-554 (Delete)       │
│                          │  │                          │
│ • Generates retry key    │  │ • Generates retry key    │
│ • Dequeues old retries   │  │ • Dequeues old retries   │
│ • Calls watcher method   │  │ • Creates ReconcileReq   │
│ • Handles errors/retries │  │ • Calls doReconcile()    │
└──────────────────────────┘  └──────────┬───────────────┘
                                         │
                                         ▼
8. Reconcile Invocation
┌──────────────────────────────────────────────────────────────────┐
│ InformerController.doReconcile()                                 │
│ operator/informer_controller.go:570-617                          │
│ • Records inflight metrics (line 573-576)                       │
│ • Starts latency timer (line 577-582)                           │
│ • Calls reconciler.Reconcile(ctx, req) (line 587)              │
│ • Handles ReconcileResult:                                       │
│   - If RequeueAfter set: schedules retry (line 592-603)        │
│   - If error: calls ErrorHandler & RetryPolicy (line 604-616)  │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
9. User Reconcile Logic
┌──────────────────────────────────────────────────────────────────┐
│ Reconciler.Reconcile()                                           │
│ examples/operator/simple/reconciler/main.go:109-116              │
│ • Receives ReconcileRequest with:                                │
│   - Action: Created/Updated/Deleted                             │
│   - Object: snapshot at time of event                           │
│ • Executes user's business logic                                │
│ • Returns ReconcileResult with optional:                         │
│   - RequeueAfter: for retry scheduling                          │
│   - State: for stateful retries                                 │
└──────────────────────────────────────────────────────────────────┘

              ╔════════════════════════════════╗
              ║   DELETE EVENT (Separate Path) ║
              ╚════════════════════════════════╝
                             │
                             ▼
10. Delete Event Distribution
┌──────────────────────────────────────────────────────────────────┐
│ OnDelete(obj)                                                    │
│ operator/informer_customcache.go:248-252                         │
│ • Distributes delete event to processor                         │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
11. Delete Handler
┌──────────────────────────────────────────────────────────────────┐
│ DeleteFunc()                                                     │
│ operator/informer_customcache.go:409-434                         │
│ • Creates context                                                │
│ • Transforms to resource.Object                                 │
│ • Calls handler.Delete(ctx, cast)                               │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
12. Controller Delete Handling
┌──────────────────────────────────────────────────────────────────┐
│ InformerController.informerDeleteFunc()                          │
│ operator/informer_controller.go:501-557                          │
│ • Similar flow to Add/Update but with ReconcileActionDeleted   │
└──────────────────────────────────────────────────────────────────┘
```

---

## Key Components Summary

| Component | File:Line | Purpose |
|-----------|-----------|---------|
| **Runner.Run()** | operator/runner.go:129-261 | Orchestrates app lifecycle |
| **simple.NewApp()** | simple/app.go:306-373 | Creates app with managed kinds |
| **InformerController** | operator/informer_controller.go | Coordinates informers, watchers, and reconcilers |
| **CustomCacheInformer** | operator/informer_customcache.go | Custom cache implementation with processor |
| **Controller** | k8s/cache/controller.go | Manages reflector and queue processing |
| **Reflector** | k8s/cache/reflector.go | Performs LIST/WATCH operations |
| **processDeltas()** | operator/informer_customcache.go:486-521 | Converts deltas to events |
| **informerAddFunc()** | operator/informer_controller.go:391-443 | Routes ADD events to reconcilers |
| **informerUpdateFunc()** | operator/informer_controller.go:446-498 | Routes UPDATE events to reconcilers |
| **informerDeleteFunc()** | operator/informer_controller.go:501-557 | Routes DELETE events to reconcilers |
| **doReconcile()** | operator/informer_controller.go:570-617 | Executes reconciler with retry logic |
| **User ReconcileFunc** | examples/.../main.go:109-116 | Business logic implementation |

---

## Key Insights

### Initial Sync vs Hot Path

- **Initial Sync**: Uses LIST operation to retrieve all existing resources, then processes them as "Add" events with `isInInitialList=true`
- **Hot Path**: Uses long-lived WATCH connection to receive streaming updates as they happen

### Event Flow Characteristics

1. **Sequential Processing**: Events for a single object are processed sequentially through the queue
2. **Multiple Handlers**: Each event can trigger multiple watchers and reconcilers registered for that kind
3. **Retry Logic**: Both watchers and reconcilers support automatic retry with configurable backoff policies
4. **Metrics**: Comprehensive metrics at each stage (event counts, latency, inflight operations)

### Component Responsibilities

- **Reflector**: Kubernetes API communication (LIST/WATCH)
- **DeltaFIFO Queue**: Event buffering and deduplication
- **Controller**: Queue processing and delta handling
- **InformerController**: Event routing to watchers/reconcilers
- **Reconciler**: User business logic

### OpinionatedReconciler Pattern

The SDK provides an `OpinionatedReconciler` wrapper (operator/reconciler.go:120-345) that:
- Adds finalizers to track resource lifecycle
- Converts initial "Add" events to "Resync" events on restart
- Ensures delete handlers run even if the operator was down
- Manages finalizer cleanup automatically

This pattern is recommended for production use to avoid missing critical delete events during operator downtime.
