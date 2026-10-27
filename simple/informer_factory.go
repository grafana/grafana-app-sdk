package simple

import (
	"slices"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// KindWatchDeclaration is what a kind's manifest declares about watching it: what its handlers need from the change
// stream, and what metadata a change bus announces with each event.
//
// The generated per-kind informers carry it, so wiring which builds informers itself resolves exactly as an App
// does without reading the manifest again.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type KindWatchDeclaration struct {
	// Needs is what the kind's handlers require from the change stream. Its zero value is app.WatchNeedsObjects.
	Needs app.ManifestVersionKindWatchNeeds
	// AnnouncedMetadata is the metadata the kind's change notifications carry, which is what lets an identity
	// payload serve an opinionated reconciler and what a label selector on that payload may be evaluated against.
	AnnouncedMetadata operator.AnnouncedMetadata
}

// WatchDeclarationsFromManifest reads each kind's watch declaration out of the manifest, keyed by kind name. A
// kind served in several versions is merged as the manifest's own accessors merge it: the strongest need across
// versions, and the union of the announced metadata.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func WatchDeclarationsFromManifest(data app.ManifestData) map[string]KindWatchDeclaration {
	out := make(map[string]KindWatchDeclaration)
	for kind, need := range data.WatchNeeds() {
		out[kind] = KindWatchDeclaration{Needs: need}
	}
	for _, version := range data.Versions {
		for _, kind := range version.Kinds {
			if kind.Watch == nil || kind.Watch.Metadata == nil {
				continue
			}
			decl := out[kind.Kind]
			decl.AnnouncedMetadata = unionAnnouncedMetadata(decl.AnnouncedMetadata, *kind.Watch.Metadata)
			out[kind.Kind] = decl
		}
	}
	return out
}

// unionAnnouncedMetadata adds one version's declaration to what the kind's other versions declare. It is a union
// rather than an intersection because the declaration is the kind's own: every version of a kind is announced by
// the same publisher, unlike the app-wide AnnouncedMetadataFromManifest, which has to take the weakest kind.
func unionAnnouncedMetadata(announced operator.AnnouncedMetadata, md app.ManifestVersionKindWatchMetadata) operator.AnnouncedMetadata {
	for _, label := range md.Labels {
		if !slices.Contains(announced.Labels, label) {
			announced.Labels = append(announced.Labels, label)
		}
	}
	announced.Finalizers = announced.Finalizers || len(md.Finalizers) > 0
	announced.Generation = announced.Generation || md.Generation
	announced.DeletionTimestamp = announced.DeletionTimestamp || md.DeletionTimestamp
	return announced
}

// InformerFactory builds the informer each kind declared it needs, resolved against what the deployment can provide.
// It is InformerSupplierFromManifest for wiring which has the declarations already — generated per-kind informers,
// or an operator which builds its informers itself rather than through simple.App.
//
// Build one per process and ask it for every kind: its informers share one set of metric collectors, which is what
// keeps a kind's series stable when its need resolves to a different informer in another deployment.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type InformerFactory struct {
	cfg       ManifestInformerConfig
	available []app.WatchInformer
	delivery  *operator.InformerDeliveryMetrics
	watch     InformerSupplier
	periodic  InformerSupplier
}

// NewInformerFactory returns a factory which resolves each kind's declaration against what cfg says this deployment
// can provide.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewInformerFactory(cfg ManifestInformerConfig) *InformerFactory {
	cfg = withSharedDeliveryMetrics(cfg)
	if cfg.Watch == nil {
		cfg.Watch = DefaultInformerSupplier
	}
	// The watch and the periodic informer need only the API; the bus needs a subscriber.
	available := []app.WatchInformer{app.WatchInformerWatch, app.WatchInformerPeriodic}
	if cfg.Subscriber != nil {
		available = append(available, app.WatchInformerBus)
	}
	return &InformerFactory{
		cfg:       cfg,
		available: available,
		delivery:  cfg.Bus.Metrics.Delivery(),
		watch:     cfg.Watch,
		periodic:  PeriodicInformerSupplier(cfg.Periodic),
	}
}

// Informer builds the informer the kind declared it needs. A zero declaration reads as needing objects, which the
// watch-backed informer serves.
//
// The declaration's announced metadata is used unless the factory was configured with its own, which a deployment
// whose bus announces something the manifests do not state would set.
func (f *InformerFactory) Informer(
	kind resource.Kind, decl KindWatchDeclaration, clients resource.ClientGenerator, opts operator.InformerOptions,
) (operator.Informer, error) {
	opts.DeliveryMetrics = f.delivery
	// The rule lives on the need itself, so an App and wiring which builds informers directly resolve identically.
	switch decl.Needs.Resolve(f.available...) {
	case app.WatchInformerPeriodic:
		return f.periodic(kind, clients, opts)
	case app.WatchInformerBus:
		return f.bus(decl)(kind, clients, opts)
	default: // app.WatchInformerWatch
		return f.watch(kind, clients, opts)
	}
}

// bus builds the kind's bus supplier. It is per kind because the announced metadata is: a filter is evaluated
// against whichever kind the informer serves, so one supplier shared across kinds could only use what they all
// announce. The collectors are injected, so building one per kind registers nothing twice.
func (f *InformerFactory) bus(decl KindWatchDeclaration) InformerSupplier {
	cfg := f.cfg.Bus
	if cfg.AnnouncedMetadata.IsZero() {
		cfg.AnnouncedMetadata = decl.AnnouncedMetadata
	}
	return BusInformerSupplier(f.cfg.Subscriber, cfg)
}

// withSharedDeliveryMetrics fills in one set of delivery collectors across the bus, periodic and watch informers,
// so that a kind whose need resolves differently in another deployment — or after a bus is switched on — reports on
// the same series rather than on a new family.
func withSharedDeliveryMetrics(cfg ManifestInformerConfig) ManifestInformerConfig {
	delivery := cfg.Bus.Metrics.Delivery()
	if delivery == nil {
		delivery = cfg.Periodic.Metrics
	}
	if delivery == nil {
		delivery = operator.NewInformerDeliveryMetrics(operator.InformerDeliveryMetricsConfig{
			MetricsConfig:       cfg.Bus.MetricsConfig,
			ResourceVersionTime: cfg.Bus.ResourceVersionTime,
		})
	}
	cfg.Periodic.Metrics = delivery
	if cfg.Bus.Metrics == nil {
		cfg.Bus.Metrics = operator.NewBusInformerMetrics(operator.BusInformerMetricsConfig{
			MetricsConfig: cfg.Bus.MetricsConfig,
			Delivery:      delivery,
		})
	}
	return cfg
}
