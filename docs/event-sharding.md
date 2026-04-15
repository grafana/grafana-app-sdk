# Event Sharding

`simple.ShardFilter` lets multiple replicas watch the same resources while only one replica handles a given event.

This is useful for HA operators where every replica can maintain its own informer state, but your business logic should run once per object rather than once per replica.

Event sharding should be paired with the memcached informer (`operator.NewMemcachedInformer`). Without a shared cache backend, each replica maintains its own in-memory cache independently. With memcached, replicas share cached object state, which reduces memory pressure and keeps cache contents consistent when ownership of an object moves between replicas. See [Choosing an Informer](operators.md#choosing-an-informer) for a comparison of all available backends.

## How It Works

A shard filter is a small interface:

```go
type ShardFilter interface {
	ShouldProcess(context.Context, resource.Object) (bool, error)
}
```

You attach it through reconcile options:

- `simple.BasicReconcileOptions.ShardFilter` for managed kinds
- `simple.UnmanagedKindReconcileOptions.ShardFilter` for unmanaged kinds

All replicas still list and watch the same resources. The filter only decides whether the current replica should delegate the event to the configured watcher or reconciler.

## Example

```go
type tenantShardFilter struct {
	tenant string
}

func (f tenantShardFilter) ShouldProcess(_ context.Context, obj resource.Object) (bool, error) {
	return obj.GetLabels()["tenant"] == f.tenant, nil
}

func NewApp(cfg app.Config) (app.App, error) {
	return simple.NewApp(simple.AppConfig{
		Name:       "my-app",
		KubeConfig: cfg.KubeConfig,
		InformerConfig: simple.AppInformerConfig{
			// Use a shared memcached cache so replicas don't each maintain independent
			// in-memory state, and so object state stays consistent when shard ownership shifts.
			InformerSupplier: func(kind resource.Kind, clients resource.ClientGenerator, opts operator.InformerOptions) (operator.Informer, error) {
				client, err := clients.ClientFor(kind)
				if err != nil {
					return nil, err
				}
				return operator.NewMemcachedInformer(kind, client, operator.MemcachedInformerOptions{
					ServerAddrs: []string{"memcached:11211"},
				})
			},
		},
		ManagedKinds: []simple.AppManagedKind{{
			Kind: mykind.Kind(),
			Reconciler: &operator.TypedReconciler[*mykind.MyKind]{
				ReconcileFunc: func(ctx context.Context, req operator.TypedReconcileRequest[*mykind.MyKind]) (operator.ReconcileResult, error) {
					return operator.ReconcileResult{}, nil
				},
			},
			ReconcileOptions: simple.BasicReconcileOptions{
				ShardFilter: tenantShardFilter{tenant: "tenant-a"},
			},
		}},
	})
}
```

The same pattern works for unmanaged kinds:

```go
simple.AppUnmanagedKind{
	Kind: otherkind.Kind(),
	Watcher: myWatcher,
	ReconcileOptions: simple.UnmanagedKindReconcileOptions{
		ShardFilter: myFilter,
	},
}
```

## Semantics

`ShouldProcess` runs on the hot event path, before the wrapped watcher or reconciler is called. Keep it fast and deterministic.

- `true, nil`: the event is processed normally
- `false, nil`: the event is skipped on this replica
- `false, err` or `true, err`: the event is treated as a failure and follows the normal informer error and retry path

For update events, the filter uses the new object snapshot when available. If an informer only provides the previous snapshot, the previous object is used for shard selection.

## Interaction With Opinionated Watchers And Reconcilers

Shard filtering happens outside the opinionated wrappers used by `simple.App`.

That means a skipped event does not:

- invoke your watcher or reconciler
- trigger opinionated finalizer handling
- trigger opinionated `Sync` behavior

This keeps shard ownership consistent for both managed and unmanaged kinds.

If your filter needs its own lifecycle, such as joining a ring or maintaining membership state, start it separately and add it to the app with `App.AddRunnable`.

## Metrics And Tracing

`simple.App` records shard filter decisions in Prometheus:

```txt
shard_filter_decisions_total{decision,event_type,group,version,resource}
```

`decision` is one of:

- `processed`
- `skipped`
- `error`

If OpenTelemetry tracing is enabled, shard decisions are also added to the current span with `shard_filter.*` attributes.

## Hash Ring Example

See [examples/operator/simple/sharded-reconciler](../examples/operator/simple/sharded-reconciler/README.md) for a complete example using a `dskit` memberlist-backed hash ring.

That example shows how to:

- implement `simple.ShardFilter`
- wire the filter into `simple.NewApp`
- run multiple replicas locally
- start the filter as an app runnable
