# Advanced Operator Features

This guide covers advanced features of the SDK's operator and resource packages that go beyond the basics covered in [Operators and Event-Based Design](./operators.md). It assumes you are already familiar with the core concepts of informers, reconcilers, and watchers.

## Resource Clients

The SDK provides several levels of abstraction for interacting with resources in storage.

### `resource.Client`

The `resource.Client` interface is the primary way to perform CRUD operations on resources of a specific kind. A single `Client` instance operates on one (group, version, kind). It supports:

- **Get** / **GetInto** -- retrieve a resource by namespace and name
- **Create** / **CreateInto** -- create a new resource
- **Update** / **UpdateInto** -- update an existing resource (optionally with a specific `ResourceVersion`)
- **Patch** / **PatchInto** -- apply a JSON Patch (RFC 6902) to a resource
- **Delete** -- delete a resource
- **List** / **ListInto** -- list resources with filtering, pagination, and label/field selectors
- **Watch** -- open a long-lived watch connection for change events

The `Into` variants accept a target object and unmarshal the response into it, which is useful when you want to reuse an existing object or work with a concrete type.

To obtain a `Client` for a given kind, use a `resource.ClientGenerator`:

```go
client, err := clientGenerator.ClientFor(myKind.Kind())
if err != nil {
    return err
}

obj, err := client.Get(ctx, resource.Identifier{
    Namespace: "default",
    Name:      "my-resource",
})
```

### `resource.TypedClient[T, L]`

`TypedClient` wraps a `Client` to provide compile-time type safety. Instead of returning `resource.Object`, its methods return the concrete type `T`:

```go
typedClient := resource.NewTypedClient[*v1.MyKind, *v1.MyKindList](client, myKind.Kind())

// Returns *v1.MyKind directly -- no type assertion needed
obj, err := typedClient.Get(ctx, resource.Identifier{
    Namespace: "default",
    Name:      "my-resource",
})

// Create automatically sets GroupVersionKind on the object
created, err := typedClient.Create(ctx, obj, resource.CreateOptions{})
```

`TypedClient` automatically sets the `GroupVersionKind` on objects during `Create` and `Update` calls, so the caller does not need to set it manually.

### `resource.UpdateObject[T]` -- Safe Read-Modify-Write

Concurrent updates to a resource can cause `409 Conflict` errors when the `ResourceVersion` has changed between your read and write. `UpdateObject` handles this pattern safely:

```go
updated, err := resource.UpdateObject[*v1.MyKind](
    ctx, client,
    resource.Identifier{Namespace: "default", Name: "my-resource"},
    func(obj *v1.MyKind, isRetry bool) (*v1.MyKind, error) {
        // Modify the object. On retries, obj is a fresh copy from the server.
        obj.Spec.Counter++
        return obj, nil
    },
    resource.UpdateOptions{Subresource: "status"},
)
```

The function:

1. Fetches the current state of the object with `Get`
2. Calls your `updateFunc` to apply modifications
3. Calls `UpdateInto` with the object's current `ResourceVersion`
4. On a `409 Conflict`, re-fetches the object and retries (up to 2 retries)

The `isRetry` parameter in the callback tells you whether this is the first attempt or a retry after a conflict.

## Informer Strategies

The SDK provides three informer implementations. All satisfy the `operator.Informer` interface:

```go
type Informer interface {
    app.Runnable
    WaitForSync(ctx context.Context) error
    AddEventHandler(handler ResourceWatcher) error
}
```

### Comparison

| Feature | `KubernetesBasedInformer` | `CustomCacheInformer` | `ConcurrentInformer` |
|---|---|---|---|
| Cache backend | In-memory (client-go SharedIndexInformer) | Pluggable (`cache.Store`) | Delegates to wrapped informer |
| Watch-list support | No | Yes (`UseWatchList` option) | Inherits from wrapped informer |
| Concurrent event handling | No | No | Yes (worker pool) |
| Health checks | Built-in | No | Delegates to wrapped informer |
| Best for | Standard workloads | Large-scale or custom caching | High-throughput event processing |

### `KubernetesBasedInformer`

This is the standard informer that wraps a `cache.SharedIndexInformer` from client-go. It is the most straightforward choice and works well for most workloads.

```go
informer, err := operator.NewKubernetesBasedInformer(
    myKind.Kind(), listWatchClient, operator.InformerOptions{
        ListWatchOptions: operator.ListWatchOptions{
            Namespace:    "default",
            LabelFilters: []string{"app=myapp"},
        },
        CacheResyncInterval: 30 * time.Minute,
        EventTimeout:        10 * time.Second,
    },
)
```

It provides a built-in health check that reports unhealthy until the initial list sync completes. To skip this behavior (useful for large initial syncs), set `HealthCheckIgnoreSync: true`.

### `CustomCacheInformer`

The `CustomCacheInformer` decouples the cache backend from the informer by accepting any `cache.Store` implementation. This is useful when you want to use an external cache (such as memcached) or a custom in-memory store.

```go
store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
lw := operator.NewListerWatcher(client, myKind.Kind(), operator.ListWatchOptions{})

informer := operator.NewCustomCacheInformer(
    store, lw, myKind.Kind(), operator.CustomCacheInformerOptions{
        InformerOptions: operator.InformerOptions{
            UseWatchList:      true,
            WatchListPageSize: 5000,
        },
        ProcessorBufferSize: 2048,
    },
)
```

Key options in `CustomCacheInformerOptions`:

- `ProcessorBufferSize` -- the event buffer size before the processor blocks (default: 1024)
- `WaitForSyncInitialInterval` / `WaitForSyncMaxInterval` -- control the polling interval for `WaitForSync`

#### Watch-List Support

When `UseWatchList` is enabled, the informer uses streaming LIST (introduced in Kubernetes 1.27) instead of traditional paginated LIST. This reduces API server memory usage for large resource sets. `WatchListPageSize` controls chunk size for traditional LIST fallback and is recommended at 5000-10000 for large clusters.

### `ConcurrentInformer`

The `ConcurrentInformer` wraps any other `Informer` and adds concurrent event processing via a worker pool. Events for the same object are always routed to the same worker, preserving per-object ordering.

```go
baseInformer, _ := operator.NewKubernetesBasedInformer(
    myKind.Kind(), client, operator.InformerOptions{},
)

concurrentInformer, err := operator.NewConcurrentInformerFromOptions(
    baseInformer, operator.InformerOptions{
        MaxConcurrentWorkers: 20,
    },
)
```

`MaxConcurrentWorkers` sets the number of workers per `ResourceWatcher` (default: 10). Increase this value if your event handlers are I/O-bound and you need higher throughput.

### When to Use Which

- **Standard workloads** with moderate resource counts: use `KubernetesBasedInformer`.
- **Large clusters** (10k+ resources) or custom cache backends: use `CustomCacheInformer`, optionally with `UseWatchList`.
- **High event throughput** with slow handlers: wrap your informer in `ConcurrentInformer`.
- **Distributed caching** across replicas: use `NewMemcachedInformer` (see below).

## Memcached Caching

For operators running multiple replicas or managing very large resource sets, the SDK provides a memcached-backed cache via `MemcachedStore` and the convenience constructor `NewMemcachedInformer`.

```go
informer, err := operator.NewMemcachedInformer(
    myKind.Kind(), listWatchClient, operator.MemcachedInformerOptions{
        ServerAddrs: []string{"memcached-1:11211", "memcached-2:11211"},
        CustomCacheInformerOptions: operator.CustomCacheInformerOptions{
            InformerOptions: operator.InformerOptions{
                CacheResyncInterval: 10 * time.Minute,
            },
        },
    },
)
```

This creates a `CustomCacheInformer` with a `MemcachedStore` as its cache backend. The `MemcachedStore` implements `cache.Store` and stores objects as JSON in memcached.

### `MemcachedStoreConfig` Options

| Option | Description |
|---|---|
| `Addrs` | List of memcached server addresses |
| `ServerSelector` | Custom server selector (overrides `Addrs`) with automatic refresh on timeouts |
| `KeySyncInterval` | Interval for syncing the in-memory key list to memcached. Set to 0 to disable `List()` / `ListKeys()` support |
| `Timeout` | Connection timeout |
| `MaxIdleConns` | Maximum idle connections |
| `PageSize` | Page size for `List()` operations (default: 500) |
| `ShardKey` | Unique identifier when running multiple `MemcachedStore` instances against the same memcached cluster |

If you need more control, create a `MemcachedStore` directly and pass it to `NewCustomCacheInformer`:

```go
store, err := operator.NewMemcachedStore(myKind.Kind(), operator.MemcachedStoreConfig{
    Addrs:           []string{"memcached:11211"},
    KeySyncInterval: 5 * time.Minute,
    Timeout:         500 * time.Millisecond,
    ShardKey:        "replica-0",
})

lw := operator.NewListerWatcher(client, myKind.Kind(), operator.ListWatchOptions{})
informer := operator.NewCustomCacheInformer(store, lw, myKind.Kind(), operator.CustomCacheInformerOptions{})
```

## RetryProcessor

The `RetryProcessor` manages retryable event processing using a sharded worker pool with priority queues. It is used internally by `InformerController` but can also be used standalone.

### How It Works

Each `RetryRequest` contains a key, a function to execute, and metadata about the retry attempt. Requests are routed to workers based on a hash of the key, so retries for the same resource are always processed by the same worker, avoiding concurrent conflicting retries.

Workers maintain a min-heap sorted by `RetryAfter` time. On each check interval (or wake signal), the worker pops all due items, executes them, and re-enqueues failures according to the `RetryPolicy`.

### Creating a RetryProcessor

```go
processor := operator.NewRetryProcessor(
    operator.RetryProcessorConfig{
        WorkerPoolSize: 8,       // Number of concurrent workers (default: 4)
        CheckInterval:  500 * time.Millisecond, // How often workers check for due retries (default: 1s)
    },
    func() operator.RetryPolicy {
        return operator.ExponentialBackoffRetryPolicy(time.Second, 5)
    },
)
```

### Using RetryProcessor

```go
// Enqueue a retry
processor.Enqueue(operator.RetryRequest{
    Key:        "default/my-resource",
    RetryAfter: time.Now().Add(5 * time.Second),
    RetryFunc: func() (*time.Duration, error) {
        err := doSomething()
        if err != nil {
            return nil, err // Let RetryPolicy decide when to retry
        }
        return nil, nil // Success -- no requeue
    },
    Attempt: 0,
    Action:  operator.ResourceActionUpdate,
})

// Remove pending retries for a key
processor.DequeueAll("default/my-resource")

// Run the processor (blocks until ctx is canceled)
go processor.Run(ctx)
```

### RetryPolicy

A `RetryPolicy` decides whether a failed attempt should be retried, and after how long:

```go
type RetryPolicy func(err error, attempt int) (bool, time.Duration)
```

The SDK provides `ExponentialBackoffRetryPolicy`:

```go
// Retries with exponential backoff: 1s, 2s, 4s, 8s, 16s, then gives up
policy := operator.ExponentialBackoffRetryPolicy(time.Second, 5)
```

When a `RetryFunc` returns a non-nil `*time.Duration`, the request is explicitly requeued after that duration regardless of the `RetryPolicy`. This allows handlers to request specific requeue timing (similar to `ReconcileResult.RequeueAfter`).

### InformerController Integration

The `InformerController` uses a `RetryProcessor` internally. You configure retry behavior through the controller's fields:

```go
controller := &operator.InformerController{
    RetryPolicy:        operator.ExponentialBackoffRetryPolicy(time.Second, 10),
    RetryDequeuePolicy: operator.OpinionatedRetryDequeuePolicy,
    ErrorHandler: func(ctx context.Context, err error) {
        logging.FromContext(ctx).Error("event processing error", "error", err)
    },
}
```

The `RetryDequeuePolicy` controls whether pending retries are canceled when a new event arrives for the same object. The default behavior dequeues all pending retries. When using `OpinionatedWatcher` or `OpinionatedReconciler`, set this to `OpinionatedRetryDequeuePolicy`.

## Runner Patterns

The `app` package provides several runner patterns for managing the lifecycle of multiple components.

### `app.Runnable`

All runners work with the `app.Runnable` interface:

```go
type Runnable interface {
    Run(context.Context) error
}
```

### `MultiRunner`

`MultiRunner` runs multiple `Runnable` instances concurrently. If any runner exits with an error, the `ErrorHandler` decides whether to cancel all others.

```go
runner := app.NewMultiRunner()
runner.Runners = append(runner.Runners, operatorRunner, webhookServer, metricsServer)
runner.ErrorHandler = func(ctx context.Context, err error) bool {
    log.Error("runner failed", "error", err)
    return true // Cancel all other runners
}

// Optional: set a timeout for graceful shutdown
timeout := 30 * time.Second
runner.ExitWait = &timeout

err := runner.Run(ctx)
```

If `ErrorHandler` returns `false`, the failed runner is ignored and the others continue. If `ExitWait` is nil, `Run` blocks until all runners exit. If set, it returns `ErrRunnerExitTimeout` after the timeout.

### `SingletonRunner`

`SingletonRunner` allows multiple `Run()` calls on the same underlying `Runnable`, but only starts the wrapped runnable once. Subsequent calls share the same running instance and wait for it to complete.

```go
runner := app.NewSingletonRunner(myOperator, true)

// Both goroutines share the same running instance
go runner.Run(ctx1)
go runner.Run(ctx2)
```

When `StopOnAny` is `true`, canceling any one `Run()` call's context stops all of them. This is useful for leader election scenarios where the same operator binary runs in multiple replicas but only the leader's instance should be active.

### `DynamicMultiRunner`

`DynamicMultiRunner` extends `MultiRunner` by allowing runners to be added and removed while `Run()` is executing.

```go
runner := app.NewDynamicMultiRunner()
runner.AddRunnable(coreOperator)

// Start running
go runner.Run(ctx)

// Later, add a new component dynamically
runner.AddRunnable(newInformerController)

// Or remove one
runner.RemoveRunnable(coreOperator)
```

Added runners are started immediately if the `DynamicMultiRunner` is already running. Removed runners have their context canceled. This is useful for operators that need to dynamically manage watched resources based on runtime configuration.

All three runner types implement `metrics.Provider` and `health.Checker`, collecting Prometheus collectors and health checks from their underlying runners.

## Further Reading

- [Operators and Event-Based Design](./operators.md) -- core operator concepts
- [Writing a Reconciler](./writing-a-reconciler.md) -- detailed reconciler guide
- [Resource Objects](./resource-objects.md) -- working with resource types
