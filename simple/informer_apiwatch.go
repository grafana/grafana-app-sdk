package simple

import (
	"errors"

	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// APIWatchInformerConfig configures the informers APIWatchInformerSupplier creates. Everything else about them —
// the selectors, the cache resync interval, the event timeout, the watch-list settings — comes from the
// operator.InformerOptions the App passes the supplier.
//
// Experimental: this API may change in a minor release.
type APIWatchInformerConfig struct {
	// Optimized builds an operator.CustomCacheInformer rather than the client-go SharedIndexInformer behind
	// operator.KubernetesBasedInformer. It honors InformerOptions.UseWatchList and
	// InformerOptions.WatchListPageSize, which the default informer ignores, and costs less memory on large
	// object counts.
	Optimized bool
}

// APIWatchInformerSupplier returns an InformerSupplier which creates informers backed by the API watch: a stream of
// every change to the kind, carrying whole objects, held in a cache.
//
// It is the mechanism the App falls back to for any kind an InformerSupplierDecider does not pick a supplier for,
// so a deployment only lists it as an InformerSupplierOffer to put it ahead of a cheaper mechanism for the kinds
// both could serve.
//
// The informers it builds state that they deliver each change, and that they re-deliver what they have cached on a
// schedule only when InformerOptions.CacheResyncInterval is set — so a kind needing operator.WatchTriggerSchedule
// from the watch is refused when no interval is configured, rather than waiting for a pass which never comes. That
// pass re-delivers as an update of the cached object, where a re-list delivers an add.
//
// Experimental: this API may change in a minor release.
func APIWatchInformerSupplier(cfg APIWatchInformerConfig) InformerSupplier {
	return func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		if clients == nil {
			return nil, errors.New("a client generator is required to watch the kind")
		}
		client, err := clients.ClientFor(kind)
		if err != nil {
			return nil, err
		}

		if options.ResourceKind == "" {
			options.ResourceKind = kind.GroupVersionKind().String()
		}

		var inf operator.Informer
		if cfg.Optimized {
			inf = operator.NewCustomCacheInformer(
				cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
				operator.NewListerWatcher(client, kind, options.ListWatchOptions),
				kind,
				operator.CustomCacheInformerOptions{InformerOptions: options},
			)
		} else {
			inf, err = operator.NewKubernetesBasedInformer(kind, client, options)
			if err != nil {
				return nil, err
			}
		}

		return operator.NewConcurrentInformerFromOptions(inf, options)
	}
}

// DefaultInformerSupplier is a default InformerSupplier function which creates a basic operator.KubernetesBasedInformer.
// Note: This supplier does NOT respect WatchListPageSize or UseWatchList configuration.
// For memory-optimized informers that support these options, use OptimizedInformerSupplier instead.
var DefaultInformerSupplier = APIWatchInformerSupplier(APIWatchInformerConfig{})

// OptimizedInformerSupplier is an InformerSupplier that uses CustomCacheInformer and respects
// all configuration in InformerOptions, including UseWatchList and WatchListPageSize.
// This supplier provides better memory efficiency for large-scale watches (>10K objects).
//
// Experimental: This API is experimental and may change in future releases.
//
// Recommended for:
//   - Watching cluster-scoped resources (e.g., namespaces, nodes)
//   - Watching large numbers of objects (>10K)
//   - Environments with memory constraints
//
// Configuration tips:
//   - Set WatchListPageSize to 5000-10000 for optimal memory usage
//   - Set UseWatchList to true on Kubernetes 1.27+ for streaming support
//   - Both can be combined: WatchListPageSize provides fallback pagination if UseWatchList is not available
var OptimizedInformerSupplier = APIWatchInformerSupplier(APIWatchInformerConfig{Optimized: true})
