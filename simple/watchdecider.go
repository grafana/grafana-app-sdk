package simple

import (
	"fmt"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// InformerMechanism is a way of watching a kind which knows what it gives that kind's handlers, so an App can
// select among several. A mechanism answers per kind, since what it can provide may depend on the kind: a bus which
// reads identities through a keys-only lister provides less for a kind whose server cannot serve that projection.
//
// Implement it on a supplier which knows its own configuration — an event bus knows its payload and its delivery
// group — so that what it advertises cannot drift from what it builds. NewInformerMechanism wraps a plain
// InformerSupplier whose provision is fixed.
//
// Experimental: this API may change in a minor release.
type InformerMechanism interface {
	// WatchOptions is what the informers this mechanism builds give a kind's handlers.
	WatchOptions(kind resource.Kind) operator.WatchOptions
	// NewInformer builds the informer for a kind, as an InformerSupplier does.
	NewInformer(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error)
}

// NewInformerMechanism presents an InformerSupplier as an InformerMechanism which provides the same thing for every
// kind, for a supplier which cannot answer for itself.
//
// Experimental: this API may change in a minor release.
func NewInformerMechanism(provides operator.WatchOptions, supplier InformerSupplier) InformerMechanism {
	return fixedMechanism{provides: provides, supplier: supplier}
}

type fixedMechanism struct {
	provides operator.WatchOptions
	supplier InformerSupplier
}

func (f fixedMechanism) WatchOptions(resource.Kind) operator.WatchOptions {
	return f.provides
}

func (f fixedMechanism) NewInformer(
	kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions,
) (operator.Informer, error) {
	return f.supplier(kind, clients, options)
}

// NewSelectingInformerSupplier returns an InformerSupplier which gives each kind the first of these mechanisms
// which satisfies what its handlers need, asking each what it provides for that kind. The order they are given in is
// the deployment's preference between the mechanisms which could serve the same kind. A nil mechanism is skipped,
// which is how one whose transport is switched off is listed rather than substituting another of its own accord.
//
// It is an InformerSupplier like any other, so it goes in AppInformerConfig.InformerSupplier: the App passes each
// kind's needs to the supplier it builds informers with, as InformerOptions.WatchNeeds, and this reads them. A kind
// no mechanism satisfies is built by fallback, which is DefaultInformerSupplier when nil — the API watch, which
// satisfies every need.
//
// The list is the whole policy: each kind gets the most optimal mechanism available to it, and which is most optimal
// is what the order says, since the first one which satisfies a kind wins. List them best first. A deployment with
// all three usually lists the event bus, then the re-list, then the API watch — the bus is the cheapest way to be
// told of a change and its own re-list sweeps as well, so it serves every kind it can, the re-list takes the kinds
// whose sweep it cannot, and the watch takes the rest — but nothing here requires that: list the re-list first and
// every kind which only sweeps goes to it instead, leaving the bus subscription to the kinds needing changes as they
// happen.
//
// A kind which should have an informer of its own needs no second hook either: wrap this, answer for that kind, and
// defer to it for the rest.
//
// Experimental: this API may change in a minor release.
func NewSelectingInformerSupplier(fallback InformerSupplier, mechanisms ...InformerMechanism) InformerSupplier {
	if fallback == nil {
		fallback = DefaultInformerSupplier
	}
	return func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		for _, mechanism := range mechanisms {
			if mechanism == nil || !options.WatchNeeds.SatisfiedBy(mechanism.WatchOptions(kind)) {
				continue
			}
			return mechanism.NewInformer(kind, clients, options)
		}
		return fallback(kind, clients, options)
	}
}

// informerFor builds the informer for a kind: the mechanism its handlers' needs select, configured with those
// needs, and checked against them if it says what it provides.
func (a *App) informerFor(kind AppUnmanagedKind) (operator.Informer, error) {
	needs := watchNeeds(kind)
	infSupplier := a.cfg.InformerConfig.InformerSupplier
	if infSupplier == nil {
		infSupplier = DefaultInformerSupplier
	}

	opts := a.cfg.InformerConfig.InformerOptions
	opts.ListWatchOptions = operator.ListWatchOptions{
		Namespace:      kind.ReconcileOptions.Namespace,
		LabelFilters:   kind.ReconcileOptions.LabelFilters,
		FieldSelectors: kind.ReconcileOptions.FieldSelectors,
	}
	if opts.MetricsConfig == (metrics.Config{}) {
		opts.MetricsConfig = a.cfg.InformerConfig.MetricsConfig
	}
	// The needs go to the mechanism as they are, so one which knows what to do with them configures itself. The two
	// options which overlap them are also set, for the informers which only read those.
	opts.WatchNeeds = needs
	if needs.Sweep != nil {
		if needs.Sweep.Every > 0 {
			opts.CacheResyncInterval = needs.Sweep.Every
		}
		if needs.Sweep.PageSize > 0 {
			opts.WatchListPageSize = needs.Sweep.PageSize
		}
	}

	inf, err := infSupplier(kind.Kind, a.clientGenerator, opts)
	if err != nil {
		return nil, err
	}
	if err := checkWatchNeeds(kind, needs, inf); err != nil {
		return nil, err
	}
	return inf, nil
}

// watchNeeds is what a kind's handlers need of the changes they are watched by, with every default filled in. The
// Opinionated variants read the deletion timestamp and the finalizer list off the object on the event, to tell a
// delete in progress from a fresh object without a read, so they need whole objects on every channel they are
// served by.
func watchNeeds(kind AppUnmanagedKind) operator.WatchOptions {
	declared := kind.ReconcileOptions.WatchOptions
	needs := operator.WatchOptions{}
	if declared.Live == nil && declared.Sweep == nil {
		needs.Live = &operator.WatchLiveOptions{}
	} else {
		if declared.Live != nil {
			live := *declared.Live
			needs.Live = &live
		}
		if declared.Sweep != nil {
			sweep := *declared.Sweep
			needs.Sweep = &sweep
		}
	}
	if needs.Live != nil {
		if needs.Live.Detail == "" || kind.ReconcileOptions.UseOpinionated {
			needs.Live.Detail = operator.WatchDetailFullObject
		}
		if needs.Live.Distribution == "" {
			needs.Live.Distribution = operator.WatchDistributionAll
		}
	}
	if needs.Sweep != nil {
		if needs.Sweep.Detail == "" || kind.ReconcileOptions.UseOpinionated {
			needs.Sweep.Detail = operator.WatchDetailFullObject
		}
		if needs.Sweep.Distribution == "" {
			needs.Sweep.Distribution = operator.WatchDistributionAll
		}
	}
	return needs
}

// checkWatchNeeds reports an error when an informer which says what it provides provides less than its kind's
// handlers need, which is a mechanism offered or set against a need it cannot serve. An informer which says nothing
// is not checked: nothing can be read off it, and every informer the SDK ships delivers each change carrying the
// whole object. The one need an unstating informer may silently miss is the scheduled sweep, which the API watch
// only performs when a cache resync is configured, so that pairing is warned about.
func checkWatchNeeds(kind AppUnmanagedKind, needs operator.WatchOptions, inf operator.Informer) error {
	provided, stated := operator.InformerWatchOptions(inf)
	if !stated || needs.SatisfiedBy(provided) {
		return nil
	}
	handler := "its reconciler or watcher"
	if kind.ReconcileOptions.UseOpinionated {
		handler = "its opinionated reconciler or watcher"
	}
	return fmt.Errorf("informer for kind '%s' provides %s, but %s needs %s",
		kind.Kind.GroupVersionKind().String(), provided, handler, needs)
}
