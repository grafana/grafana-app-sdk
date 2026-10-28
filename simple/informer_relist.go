package simple

import (
	"time"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// identityPayloadInformer is implemented by an informer which may deliver identity-only objects. watchKind asserts
// it to refuse a pairing with an opinionated reconciler or watcher, which reads the deletion timestamp and the
// finalizer list off the delivered object and would see both as absent.
type identityPayloadInformer interface {
	IdentityPayload() bool
}

var _ identityPayloadInformer = (*operator.RelistInformer)(nil)

// RelistInformerConfig configures the informers RelistInformerSupplier creates.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type RelistInformerConfig struct {
	// SourceFactory builds the ChangeSource which delivers a kind's changes between re-lists. KubernetesWatchSource
	// watches the API server through the kind's client. When nil the informer is re-list only, and reacts to changes
	// on the re-list cadence.
	SourceFactory func(kind resource.Kind, client resource.Client, options operator.InformerOptions) (operator.ChangeSource, error)
	// ResyncInterval is how often the full set is re-listed. The re-list is what catches changes the source did not
	// deliver, so it is a correctness interval, not a nicety.
	// A non-positive value uses operator.DefaultRelistInterval.
	ResyncInterval time.Duration
	// AllowDegradedStart lets an informer start re-list-only when its source is unavailable, instead of holding its
	// initial list until a watch opens. See operator.RelistInformerOptions.AllowDegradedStart.
	AllowDegradedStart bool
	// PageSize is the page size the default lister requests. An empty value uses operator.DefaultListerPageSize.
	PageSize int
	// KeyListerFactory builds a ResourceKeyLister for a kind, for a server which can list identities without object
	// bodies. When set, each kind's re-list reads identities through that projection and falls back to the paginated
	// full-object list for a re-list the server cannot serve that way. Ignored when ListerFactory is set.
	KeyListerFactory func(kind resource.Kind) (operator.ResourceKeyLister, error)
	// HonorsListWatchOptions declares that KeyListerFactory's listers apply the namespace and filters server-side.
	// See operator.IdentityListerOptions.HonorsListWatchOptions.
	HonorsListWatchOptions bool
	// ListerFactory replaces the whole ResourceLister for a kind, for a server whose list does not fit either shape
	// above. When nil, the lister is built from KeyListerFactory and the kind's client.
	ListerFactory func(kind resource.Kind, client resource.Client, opts operator.ClientListerOptions) (operator.ResourceLister, error)
	// MetricsConfig controls the namespace and histogram settings of the relist informer metrics. The collectors are
	// shared by every kind the supplier creates an informer for.
	MetricsConfig metrics.Config
	// ResourceVersionTime decodes the timestamp a resource version embeds, which is what dates a change for the
	// delivery latency histograms. Resource versions are opaque in the kubernetes API contract, so there is no
	// portable decoding: when this is nil no latency is observed.
	ResourceVersionTime func(int64) time.Time
}

// KubernetesWatchSource is a RelistInformerConfig.SourceFactory which watches the API server through the kind's
// client. Paired with a re-list it gives a watch-driven informer which keeps no per-object cache.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
func KubernetesWatchSource(_ resource.Kind, client resource.Client, _ operator.InformerOptions) (operator.ChangeSource, error) {
	return client, nil
}

// RelistInformerSupplier returns an InformerSupplier which creates operator.RelistInformer instances.
//
// Every informer the returned supplier creates shares one set of metric collectors, since they differ only by label.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
func RelistInformerSupplier(cfg RelistInformerConfig) InformerSupplier {
	relistMetrics := operator.NewRelistInformerMetrics(operator.RelistInformerMetricsConfig{
		MetricsConfig:       cfg.MetricsConfig,
		ResourceVersionTime: cfg.ResourceVersionTime,
	})
	return func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		client, err := clients.ClientFor(kind)
		if err != nil {
			return nil, err
		}

		listerOpts := operator.ClientListerOptions{
			ListWatchOptions: options.ListWatchOptions,
			PageSize:         cfg.PageSize,
			Recorder:         relistMetrics.RelistRecorder(kind),
		}
		lister, err := cfg.lister(kind, client, listerOpts)
		if err != nil {
			return nil, err
		}

		var source operator.ChangeSource
		if cfg.SourceFactory != nil {
			if source, err = cfg.SourceFactory(kind, client, options); err != nil {
				return nil, err
			}
		}

		return operator.NewRelistInformer(kind, lister, source, operator.RelistInformerOptions{
			ListWatchOptions:   options.ListWatchOptions,
			ResyncInterval:     cfg.ResyncInterval,
			AllowDegradedStart: cfg.AllowDegradedStart,
			ErrorHandler:       options.ErrorHandler,
			Metrics:            relistMetrics,
		})
	}
}

func (c RelistInformerConfig) lister(kind resource.Kind, client resource.Client, opts operator.ClientListerOptions) (operator.ResourceLister, error) {
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

	keys, err := c.KeyListerFactory(kind)
	if err != nil {
		return nil, err
	}
	identities, err := operator.NewIdentityLister(kind, keys, operator.IdentityListerOptions{
		ListWatchOptions:       opts.ListWatchOptions,
		HonorsListWatchOptions: c.HonorsListWatchOptions,
		Recorder:               opts.Recorder,
	})
	if err != nil {
		return nil, err
	}
	return operator.NewFallbackLister(identities, objects)
}
