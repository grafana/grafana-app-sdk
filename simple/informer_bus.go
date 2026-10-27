package simple

import (
	"errors"
	"slices"
	"time"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// identityPayloadInformer is implemented by an informer which delivers identity-only objects. watchKind asserts it
// to refuse a pairing with an opinionated reconciler or watcher, which reads the deletion timestamp and the
// finalizer list off the delivered object and would see both as absent.
type identityPayloadInformer interface {
	IdentityPayload() bool
	DeliversOpinionatedMetadata() bool
}

var _ identityPayloadInformer = (*operator.BusInformer)(nil)

// BusInformerConfig configures the informers BusInformerSupplier creates.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusInformerConfig struct {
	// Payload selects which object live events deliver. An empty value is operator.PayloadIdentity, which delivers
	// identity only and so cannot be combined with an opinionated reconciler or watcher.
	Payload operator.BusPayload
	// ResyncInterval is how often the full set is re-listed. The re-list is what catches changes the live stream did
	// not deliver, so it is a correctness interval, not a nicety.
	// A non-positive value uses operator.DefaultBusResyncInterval.
	ResyncInterval time.Duration
	// Group joins the app's replicas into a group across which the bus shares delivery, so that each event reaches
	// one replica rather than all of them. A zero Group delivers every event to every replica.
	Group resourcebus.Group
	// AllowDegradedStart lets an informer start re-list-only when the bus is unavailable, instead of holding its
	// initial list until a subscription opens. See operator.BusInformerOptions.AllowDegradedStart.
	AllowDegradedStart bool
	// AnnouncedMetadata declares what this app's bus announces with each event. Declaring the deletion timestamp,
	// finalizers and generation is what lets an identity payload be used with an opinionated reconciler or watcher,
	// which read those off the delivered object. See operator.AnnouncedMetadata.
	//
	// Prefer AnnouncedMetadataFromManifest, which derives this from the kinds' own declaration so the two cannot
	// disagree.
	AnnouncedMetadata operator.AnnouncedMetadata
	// PageSize is the page size the default lister requests. An empty value uses operator.DefaultListerPageSize.
	PageSize int
	// KeyListerFactory builds a ResourceKeyLister for a kind, for a server which can list identities without object
	// bodies. When set, each kind's re-list reads identities through that projection and falls back to the paginated
	// full-object list for a re-list the server cannot serve that way. Ignored when ListerFactory is set.
	//
	// The lister it returns must honor the given ListWatchOptions, or report operator.ErrProjectionUnsupported —
	// from the factory or from the list — so that the re-list falls back rather than watching objects the app did
	// not ask for. See operator.ResourceKeyLister.
	KeyListerFactory func(kind resource.Kind, opts operator.ListWatchOptions) (operator.ResourceKeyLister, error)
	// ListerFactory replaces the whole ResourceLister for a kind, for a server whose list does not fit either shape
	// above. When nil, the lister is built from KeyListerFactory and the kind's client.
	ListerFactory func(kind resource.Kind, client resource.Client, opts operator.ClientListerOptions) (operator.ResourceLister, error)
	// Metrics are the collectors every informer this supplier creates will share. Set it when an app builds more
	// than one bus supplier — a per-kind InformerSupplier alongside the app-level one, say: each supplier would
	// otherwise build its own collectors with identical descriptors, and registration keeps only the first, leaving
	// the rest updating vectors nothing exports. When nil, the supplier builds one set from MetricsConfig and
	// shares it across the kinds it serves.
	Metrics *operator.BusInformerMetrics
	// MetricsConfig controls the namespace and histogram settings of the collectors the supplier builds when
	// Metrics is nil. It is ignored when Metrics is set.
	MetricsConfig metrics.Config
	// ResourceVersionTime decodes the timestamp a resource version embeds, which is what dates a change for the
	// delivery latency histograms. Resource versions are opaque in the kubernetes API contract, so there is no
	// portable decoding: when this is nil no latency is observed.
	ResourceVersionTime func(int64) time.Time
}

// BusInformerSupplier returns an InformerSupplier which creates operator.BusInformer instances driven by sub.
// A nil subscriber returns DefaultInformerSupplier, so that an app can be wired for a bus which is switched off
// without branching at every call site.
//
// Every informer the returned supplier creates shares one set of metric collectors, since they differ only by label.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func BusInformerSupplier(sub resourcebus.Subscriber, cfg BusInformerConfig) InformerSupplier {
	if sub == nil {
		return DefaultInformerSupplier
	}
	busMetrics := cfg.Metrics
	if busMetrics == nil {
		busMetrics = operator.NewBusInformerMetrics(operator.BusInformerMetricsConfig{
			MetricsConfig:       cfg.MetricsConfig,
			ResourceVersionTime: cfg.ResourceVersionTime,
		})
	}
	return func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		client, err := clients.ClientFor(kind)
		if err != nil {
			return nil, err
		}

		listerOpts := operator.ClientListerOptions{
			ListWatchOptions: options.ListWatchOptions,
			PageSize:         cfg.PageSize,
			Recorder:         busMetrics.RelistRecorder(kind),
		}
		lister, err := cfg.lister(kind, client, listerOpts)
		if err != nil {
			return nil, err
		}

		inf, err := operator.NewBusInformer(kind, sub, lister, operator.BusInformerOptions{
			Namespace:          options.ListWatchOptions.Namespace,
			ListWatchOptions:   options.ListWatchOptions,
			Payload:            cfg.Payload,
			Getter:             client,
			ResyncInterval:     cfg.ResyncInterval,
			Group:              cfg.Group,
			AllowDegradedStart: cfg.AllowDegradedStart,
			AnnouncedMetadata:  cfg.AnnouncedMetadata,
			ErrorHandler:       options.ErrorHandler,
			Metrics:            busMetrics,
		})
		if err != nil {
			return nil, err
		}
		// Watchers run on their own per-key workers, as they do under the other suppliers: the informer's loop owns
		// snapshot mutation and the re-list, and a slow reconcile on that loop would block unrelated objects and
		// the recovery it depends on. Ordering per object is preserved, since a key always lands on one worker.
		return operator.NewConcurrentInformerFromOptions(inf, options)
	}
}

func (c BusInformerConfig) lister(kind resource.Kind, client resource.Client, opts operator.ClientListerOptions) (operator.ResourceLister, error) {
	if c.ListerFactory != nil {
		return c.ListerFactory(kind, client, opts)
	}
	objects, err := operator.NewClientLister(client, kind, opts)
	if err != nil {
		return nil, err
	}
	if c.KeyListerFactory == nil {
		return objects, nil
	}
	// A hydrated payload promises watchers whole objects. Listing identities would hand them full objects live and
	// near-empty ones on every re-list and initial list, which no caller can detect from a resource.Object.
	if c.Payload == operator.PayloadHydrated {
		return nil, errors.New("KeyListerFactory cannot be used with PayloadHydrated: re-lists would deliver less than live events")
	}

	keys, err := c.KeyListerFactory(kind, opts.ListWatchOptions)
	if err != nil {
		// A factory which reports the projection unsupported for this kind's scope leaves the full-object list,
		// rather than failing the informer: that is the same decision the fallback lister makes per re-list.
		if errors.Is(err, operator.ErrProjectionUnsupported) {
			return objects, nil
		}
		return nil, err
	}
	identities, err := operator.NewIdentityLister(kind, keys, operator.IdentityListerOptions{Recorder: opts.Recorder})
	if err != nil {
		return nil, err
	}
	return operator.NewFallbackLister(identities, objects)
}

// AnnouncedMetadataFromManifest derives what a bus announces from the kinds' own declaration in the app manifest,
// so an app states it once rather than restating it here and risking the two disagreeing.
//
// A field counts as announced only where every kind which declares any watch metadata declares that field: the
// informer serves one kind at a time but the configuration is app-wide, so the weakest kind governs. A kind which
// declares nothing is ignored rather than counted as a refusal, since it is not watched over the bus.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func AnnouncedMetadataFromManifest(data app.ManifestData) operator.AnnouncedMetadata {
	specs := data.WatchMetadata()
	if len(specs) == 0 {
		return operator.AnnouncedMetadata{}
	}
	announced := operator.AnnouncedMetadata{DeletionTimestamp: true, Finalizers: true, Generation: true}
	first := true
	for _, spec := range specs {
		announced.DeletionTimestamp = announced.DeletionTimestamp && spec.DeletionTimestamp
		announced.Generation = announced.Generation && spec.Generation
		announced.Finalizers = announced.Finalizers && len(spec.Finalizers) > 0
		// Label keys intersect for the same reason the flags do: a filter is evaluated against whatever kind the
		// informer serves, so only a key every declaring kind announces can be relied on.
		if first {
			announced.Labels = append([]string(nil), spec.Labels...)
			first = false
			continue
		}
		announced.Labels = intersectStrings(announced.Labels, spec.Labels)
	}
	return announced
}

// intersectStrings keeps the values of kept which next also has, preserving order.
func intersectStrings(kept, next []string) []string {
	out := kept[:0]
	for _, value := range kept {
		if slices.Contains(next, value) {
			out = append(out, value)
		}
	}
	return out
}

// PeriodicInformerConfig configures the informers PeriodicInformerSupplier creates.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type PeriodicInformerConfig struct {
	// ResyncInterval is how often the kind is re-listed, which for this informer is how often its handlers run at
	// all. A non-positive value uses operator.DefaultBusResyncInterval.
	ResyncInterval time.Duration
	// PageSize is the page size the default lister requests. An empty value uses operator.DefaultListerPageSize.
	PageSize int
	// ListerFactory replaces the ResourceLister for a kind. When nil, a paginated list over the kind's client is
	// used, which applies the namespace and selectors the informer was configured with.
	ListerFactory func(kind resource.Kind, client resource.Client, opts operator.ClientListerOptions) (operator.ResourceLister, error)
	// Metrics are the collectors the suppliers' informers share, for the re-list request and projection counters.
	// When nil, one set is built from MetricsConfig. See BusInformerConfig.Metrics.
	Metrics *operator.BusInformerMetrics
	// MetricsConfig controls the namespace of the collectors built when Metrics is nil.
	MetricsConfig metrics.Config
}

// PeriodicInformerSupplier returns an InformerSupplier which creates operator.PeriodicInformer instances: no watch,
// no subscription and no snapshot, just a re-list on a cadence delivering every listed object as an add.
//
// Unlike BusInformerSupplier it accepts label and field selectors, because the list is its only source: there is no
// live stream for a filtered list to disagree with, so the selectors reach the server and nothing else has to honor
// them.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func PeriodicInformerSupplier(cfg PeriodicInformerConfig) InformerSupplier {
	listMetrics := cfg.Metrics
	if listMetrics == nil {
		listMetrics = operator.NewBusInformerMetrics(operator.BusInformerMetricsConfig{
			MetricsConfig: cfg.MetricsConfig,
		})
	}
	return func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		client, err := clients.ClientFor(kind)
		if err != nil {
			return nil, err
		}

		listerOpts := operator.ClientListerOptions{
			ListWatchOptions: options.ListWatchOptions,
			PageSize:         cfg.PageSize,
			Recorder:         listMetrics.RelistRecorder(kind),
		}
		lister, err := operator.NewClientLister(client, kind, listerOpts)
		if cfg.ListerFactory != nil {
			lister, err = cfg.ListerFactory(kind, client, listerOpts)
		}
		if err != nil {
			return nil, err
		}

		inf, err := operator.NewPeriodicInformer(kind, lister, operator.PeriodicInformerOptions{
			ResyncInterval: cfg.ResyncInterval,
			ErrorHandler:   options.ErrorHandler,
		})
		if err != nil {
			return nil, err
		}
		return operator.NewConcurrentInformerFromOptions(inf, options)
	}
}

// ManifestInformerConfig configures InformerSupplierFromManifest: the informers it may resolve a kind's declared
// need to, and what each of them is configured with.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type ManifestInformerConfig struct {
	// Subscriber is the change bus, or nil where the deployment has none. A kind which needs only a signal is
	// served by the bus when one is configured and by Watch when it is not.
	Subscriber resourcebus.Subscriber
	// Bus configures the bus informers. Its Payload defaults to operator.PayloadIdentity, which is what a kind
	// asking for a signal has said it can work with, and its AnnouncedMetadata defaults to what the manifest
	// declares.
	Bus BusInformerConfig
	// Periodic configures the periodic informers a kind asking for a cadence resolves to.
	Periodic PeriodicInformerConfig
	// Watch supplies the watch-backed informer. A nil value uses DefaultInformerSupplier; set
	// OptimizedInformerSupplier for kinds with large object counts.
	Watch InformerSupplier
}

// InformerSupplierFromManifest returns an InformerSupplier which resolves each kind's declared need to the cheapest
// informer that satisfies it and the deployment can provide.
//
//	objects (the default) → the watch-backed informer
//	signal                → the bus where one is configured, the watch-backed informer otherwise
//	cadence               → the periodic informer
//
// Every fallback is upward in capability, so it is always safe: a kind which needs a signal, served by a watch
// because no bus is configured, is given strictly more than it asked for at the cost of more memory. That is why
// the manifest states a requirement rather than naming an informer — whether a deployment runs a change bus is not
// something an app can ship knowing.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func InformerSupplierFromManifest(data app.ManifestData, cfg ManifestInformerConfig) InformerSupplier {
	needs := data.WatchNeeds()

	watch := cfg.Watch
	if watch == nil {
		watch = DefaultInformerSupplier
	}
	if cfg.Bus.AnnouncedMetadata.IsZero() {
		cfg.Bus.AnnouncedMetadata = AnnouncedMetadataFromManifest(data)
	}
	bus := BusInformerSupplier(cfg.Subscriber, cfg.Bus)
	periodic := PeriodicInformerSupplier(cfg.Periodic)

	// The watch and the periodic informer need only the API; the bus needs a subscriber.
	available := []app.WatchInformer{app.WatchInformerWatch, app.WatchInformerPeriodic}
	if cfg.Subscriber != nil {
		available = append(available, app.WatchInformerBus)
	}

	return func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		// The rule lives on the need itself, so wiring which builds informers directly resolves identically.
		switch needs[kind.Kind()].Resolve(available...) {
		case app.WatchInformerPeriodic:
			return periodic(kind, clients, options)
		case app.WatchInformerBus:
			return bus(kind, clients, options)
		default: // app.WatchInformerWatch
			return watch(kind, clients, options)
		}
	}
}
