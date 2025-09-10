# ADR 0001: WatchListPageSize Configuration Option

## Status

Accepted

## Context

The Grafana App SDK needs to support configuration of the `WatchListPageSize` option for Kubernetes informers. This option controls the chunk size of initial and resync watch lists, which can be important for performance tuning in large-scale deployments.

The `WatchListPageSize` is a feature of the Kubernetes client-go cache.Reflector that allows controlling pagination of list requests. When set to a value > 0, it enables pagination for list operations, which can be useful for:

- Reducing memory usage during initial sync
- Improving performance for large resource sets
- Controlling API server load

However, it should be used carefully as paginated lists are always served directly from etcd, which can be significantly less efficient and may lead to performance and scalability problems.

The configuration is provided at the app level through `simple.AppInformerConfig` to ensure proper separation of concerns and logical placement within the SDK architecture.

## Decision

We will add `WatchListPageSize` configuration support in the Grafana App SDK:

1. **App Level**: Add `WatchListPageSize` field to `simple.AppInformerConfig`
2. **Informer Level**: Add `WatchListPageSize` field to `operator.InformerOptions`

The configuration is provided at the app level through `InformerConfig` to ensure proper separation of concerns and logical placement within the SDK architecture. The configuration flows through the informer creation chain and is made available for future implementation of the actual pagination logic.

## Consequences

### Positive

- **Logical Placement**: Configuration is placed at the app level through `InformerConfig`, which is the appropriate location for informer-specific settings
- **Performance Tuning**: Enables fine-grained control over list operations for large-scale deployments
- **Backward Compatibility**: Default value of 0 (disabled) ensures existing code continues to work
- **Documentation**: Comprehensive documentation and examples provided
- **Separation of Concerns**: Clear distinction between runner-level and app-level configuration

### Negative

- **Complexity**: Adds configuration options that need to be understood by users
- **Performance Risk**: If misused, can lead to worse performance due to etcd direct access
- **Implementation Gap**: The actual pagination logic requires deeper integration with cache.Reflector (currently marked with TODO)

### Neutral

- **Future Work**: The configuration is in place but the actual usage in informers requires additional implementation
- **Testing**: Basic test coverage ensures the configuration flows correctly through the system

## Implementation Details

### Files Modified

1. **simple/app.go**
   - Added `WatchListPageSize int64` to `AppInformerConfig` (inherited from `operator.InformerOptions`)
   - Updated `InformerSupplier` signature to include config parameter
   - Modified `DefaultInformerSupplier` to pass the configuration

2. **operator/informer_kubernetes.go**
   - Added `WatchListPageSize int64` to `InformerOptions`
   - Added TODO comment for future implementation of actual pagination support

3. **operator/informer_customcache.go**
   - Added `WatchListPageSize int64` to `CustomCacheInformerOptions`
   - Implemented actual usage of `WatchListPageSize` in the `newInformer` function
   - Added `NewDefaultCustomCacheInformer` helper function for easy creation of custom cache informers with in-memory cache

4. **Documentation and Examples**
   - Updated `examples/operator/simple/reconciler/main.go`
   - Updated `examples/operator/simple/watcher/main.go`
   - Updated `codegen/templates/app/app.tmpl`
   - Updated `docs/operators.md`

5. **Testing**
   - Added test in `simple/app_test.go` to verify configuration flow

### Usage Examples

**App Level Configuration:**
```go
config := simple.AppConfig{
    Name:       "my-app",
    KubeConfig: cfg.KubeConfig,
    InformerConfig: simple.AppInformerConfig{
        InformerOptions: operator.InformerOptions{
            WatchListPageSize: 1000, // Enable pagination
        },
    },
    ManagedKinds: []simple.AppManagedKind{...},
}
```

**Custom Cache Informer Configuration:**
```go
// For custom informers, you can configure WatchListPageSize directly using the helper function
customInformer := operator.NewDefaultCustomCacheInformer(
    kind,
    client,
    operator.CustomCacheInformerOptions{
        WatchListPageSize: 1000, // Enable pagination for custom informers
        CacheResyncInterval: 10 * time.Minute,
    },
)

// Or manually create the cache for more control
store := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{
    cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
})

listerWatcher := operator.NewListerWatcher(client, kind, operator.ListWatchOptions{})

customInformer := operator.NewCustomCacheInformer(
    store,
    listerWatcher,
    kind,
    operator.CustomCacheInformerOptions{
        WatchListPageSize: 1000, // Enable pagination for custom informers
        CacheResyncInterval: 10 * time.Minute,
    },
)
```

**Example with Custom Cache Informer (from main.go):**
```go
func NewApp(config app.Config) (app.App, error) {
    // Set up the reconciler
    reconciler := &simple.Reconciler{
        ReconcileFunc: func(ctx context.Context, request operator.ReconcileRequest) (operator.ReconcileResult, error) {
            log.Printf(
                "Reconciling object:\n\taction: %s\n\tobject: %v\n",
                operator.ResourceActionFromReconcileAction(request.Action),
                request.Object,
            )
            return operator.ReconcileResult{}, nil
        },
    }

    // Create a custom cache informer with WatchListPageSize support using the new helper function
    customInformer := operator.NewDefaultCustomCacheInformer(
        kind,
        client,
        operator.CustomCacheInformerOptions{
            WatchListPageSize: 1000, // Enable pagination for better performance
            CacheResyncInterval: 10 * time.Minute,
        },
    )

    // Create the app with the custom informer
    return simple.NewApp(simple.AppConfig{
        Name:       "simple-reconciler-app",
        KubeConfig: config.KubeConfig,
        ManagedKinds: []simple.AppManagedKind{{
            Kind:             kind,
            Reconciler:       reconciler,
            ReconcileOptions: simple.BasicReconcileOptions{},
        }},
    })
}
```

## Current Implementation Status

### Fully Implemented
- **Custom Cache Informers**: The `WatchListPageSize` is fully implemented and used in `operator.NewCustomCacheInformer` and the `newInformer` function
- **Helper Functions**: Added `NewDefaultCustomCacheInformer` for easy creation of custom cache informers with in-memory cache
- **Configuration Flow**: The configuration flows correctly from app level through to informer creation
- **Documentation**: Comprehensive examples and documentation provided

### Partially Implemented
- **Kubernetes-Based Informers**: The `WatchListPageSize` configuration is available but not yet used in `cache.NewSharedIndexInformer` due to limitations in the Kubernetes client-go API

## Future Work

1. **Implement Kubernetes-Based Informer Pagination**: The TODO in `operator/informer_kubernetes.go` needs to be addressed to actually use the `WatchListPageSize` in the informer creation. This requires deeper integration with the Kubernetes client-go cache.Reflector API.

2. **Performance Testing**: Validate the performance impact of different page sizes in real-world scenarios.

3. **Documentation**: Add more detailed guidance on when and how to use this feature, including performance considerations.

4. **Integration**: Complete the integration with Kubernetes client-go cache.Reflector to enable the actual pagination functionality for all informer types.

## References

- [Kubernetes client-go cache.Reflector](https://github.com/kubernetes/client-go/blob/master/tools/cache/reflector.go)
- [Kubernetes Enhancement Proposal 3157: Watch List](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/3157-watch-list) 