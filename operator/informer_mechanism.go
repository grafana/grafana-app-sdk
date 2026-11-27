package operator

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/health"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// InformerSupplier creates an Informer for a kind. options.WatchNeeds holds what the kind's handlers need, so a
// supplier can use it to configure the informer (see InformerOptions.ResyncOptions) or to choose which informer to
// build.
type InformerSupplier func(
	kind resource.Kind, clients resource.ClientGenerator, options InformerOptions,
) (Informer, error)

// InformerMechanism is a way to watch a kind that also says what it provides. NewSelectingInformerSupplier uses it
// to choose between mechanisms. What it provides can depend on the kind.
//
// Experimental: this API may change in a minor release.
type InformerMechanism interface {
	// WatchOptions returns what the informers built for kind provide to its handlers.
	WatchOptions(kind resource.Kind) WatchOptions
	// NewInformer builds the informer for kind, like an InformerSupplier.
	NewInformer(kind resource.Kind, clients resource.ClientGenerator, options InformerOptions) (Informer, error)
}

// PreviousComparer is implemented by a mechanism, and by the informers it builds, when it can apply
// WatchResyncOptions.IgnoreUnchanged by comparing each object with its last delivery. Its resync must go to every
// instance or be sharded, and its deliveries must contain at least metadata. Nothing checks these rules yet.
//
// Experimental: this API may change in a minor release.
type PreviousComparer interface {
	ComparesPrevious() bool
}

// NewInformerMechanism returns an InformerMechanism that builds informers with supplier and provides the same
// WatchOptions for every kind.
//
// Experimental: this API may change in a minor release.
func NewInformerMechanism(provides WatchOptions, supplier InformerSupplier) InformerMechanism {
	return fixedMechanism{provides: provides, supplier: supplier}
}

type fixedMechanism struct {
	provides WatchOptions
	supplier InformerSupplier
}

func (f fixedMechanism) WatchOptions(resource.Kind) WatchOptions {
	return f.provides
}

func (f fixedMechanism) NewInformer(
	kind resource.Kind, clients resource.ClientGenerator, options InformerOptions,
) (Informer, error) {
	return f.supplier(kind, clients, options)
}

// NewSelectingInformerSupplier returns an InformerSupplier that gives each kind the first mechanism whose
// WatchOptions satisfy the kind's WatchNeeds. It skips nil mechanisms, and mechanisms that cannot serve the kind's
// query (the bus mechanisms do not support label or field selectors).
//
// List the mechanisms from cheapest to most capable, so each kind gets the cheapest one that works. For example: a
// periodic LIST, then bus notifications, then a bus with recovery.
//
// A kind that no mechanism satisfies is built by fallback, which must work for any kind. If fallback is nil, such a
// kind returns an error. The selection is not logged.
//
// Experimental: this API may change in a minor release.
func NewSelectingInformerSupplier(fallback InformerSupplier, mechanisms ...InformerMechanism) InformerSupplier {
	return func(kind resource.Kind, clients resource.ClientGenerator, options InformerOptions) (Informer, error) {
		for _, mechanism := range mechanisms {
			if mechanism == nil {
				continue
			}
			if scoped, ok := mechanism.(interface{ supportsQuery(ListWatchOptions) bool }); ok &&
				!scoped.supportsQuery(options.ListWatchOptions) {
				continue
			}
			provides := mechanism.WatchOptions(kind)
			if !options.WatchNeeds.SatisfiedBy(provides) {
				continue
			}
			inf, err := mechanism.NewInformer(kind, clients, options)
			if err != nil {
				return nil, err
			}
			return InformerWithWatchOptions(inf, provides), nil
		}
		if fallback == nil {
			return nil, fmt.Errorf("no watch mechanism provides what kind '%s' needs (%s), and no fallback supplier"+
				" was given to build it with", kind.GroupVersionKind().String(), options.WatchNeeds)
		}
		return fallback(kind, clients, options)
	}
}

// InformerWithWatchOptions returns inf unchanged if inf implements WatchOptionsInformer. Otherwise it wraps inf in
// an informer that reports provides. It checks only inf itself, not the informers that inf wraps.
//
// The wrapper forwards metrics and health checks. A caller that type-asserts the result to a concrete informer type
// will not find it. A nil inf returns nil.
//
// Experimental: this API may change in a minor release.
func InformerWithWatchOptions(inf Informer, provides WatchOptions) Informer {
	if inf == nil {
		return nil
	}
	if _, ok := inf.(WatchOptionsInformer); ok {
		return inf
	}
	return advertisedInformer{Informer: inf, provides: provides}
}

// advertisedInformer reports what its mechanism provides. It forwards the informer's metrics and health checks. It
// does not implement health.Check itself, so it adds no health check of its own.
type advertisedInformer struct {
	Informer
	provides WatchOptions
}

func (a advertisedInformer) WatchOptions() WatchOptions {
	return a.provides
}

func (a advertisedInformer) Unwrap() Informer {
	return a.Informer
}

func (a advertisedInformer) PrometheusCollectors() []prometheus.Collector {
	if provider, ok := a.Informer.(metrics.Provider); ok {
		return provider.PrometheusCollectors()
	}
	return nil
}

func (a advertisedInformer) HealthChecks() []health.Check {
	var checks []health.Check
	if check, ok := a.Informer.(health.Check); ok {
		checks = append(checks, check)
	}
	if checker, ok := a.Informer.(health.Checker); ok {
		checks = append(checks, checker.HealthChecks()...)
	}
	return checks
}
