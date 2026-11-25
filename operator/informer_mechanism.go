package operator

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/health"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// InformerSupplier is a function which creates an Informer for a kind. The options carry what the kind's handlers
// need, as WatchNeeds, so a supplier can both configure what it builds — see InformerOptions.ResyncOptions — and
// choose what to build.
type InformerSupplier func(
	kind resource.Kind, clients resource.ClientGenerator, options InformerOptions,
) (Informer, error)

// InformerMechanism is a way of watching a kind which says what it gives that kind's handlers, so several can be
// selected between. Implement it on a supplier which knows its own configuration — an event bus knows its payload
// and delivery group — so what it advertises cannot drift from what it builds. It answers per kind, since what it
// can provide may depend on the kind.
//
// Experimental: this API may change in a minor release.
type InformerMechanism interface {
	// WatchOptions is what the informers this mechanism builds give a kind's handlers.
	WatchOptions(kind resource.Kind) WatchOptions
	// NewInformer builds the informer for a kind, as an InformerSupplier does.
	NewInformer(kind resource.Kind, clients resource.ClientGenerator, options InformerOptions) (Informer, error)
}

// NewInformerMechanism presents a supplier whose provision is fixed as an InformerMechanism.
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

// NewSelectingInformerSupplier returns an InformerSupplier which gives each kind the first of these mechanisms
// whose advertisement satisfies what that kind's handlers need, asking each what it provides for that kind. A nil
// mechanism is skipped, which is how one whose transport is switched off is listed.
//
// Order the list by increasing capability and each kind takes the cheapest thing which serves it: a re-list, which
// only re-delivers everything on a schedule; then a bus which only notifies; then one with a re-list of its own,
// which does both.
//
// A kind no mechanism satisfies is built by fallback, which must be able to build an informer for any kind — when
// it is nil such a kind is an error rather than a panic. The informer a mechanism builds is passed through
// InformerWithWatchOptions, so its advertisement survives an informer which does not describe itself.
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

// InformerWithWatchOptions returns inf unchanged when inf itself says what it provides, and otherwise an informer
// which says provides on its behalf, so a mechanism's advertisement is not lost to an informer which does not
// describe itself.
//
// Only inf counts, not what it wraps: a mechanism which builds on top of a stock informer provides what the mechanism
// says, not what the informer underneath it would say on its own. InformerWatchOptions does follow the chain, since
// there it is reading a wrapper which changed nothing — the two directions are deliberately different.
//
// The wrapper is observable: it forwards the optional interfaces InformerController reads, but a caller which
// type-asserts the informer back to a concrete type will not find it. A nil informer is returned as nil, so a
// supplier which returns one still fails where it is added rather than panicking on first use.
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

// advertisedInformer carries what its mechanism said it provides. It forwards the optional interfaces
// InformerController reads, so wrapping costs the informer neither its metrics nor its health checks; it
// deliberately does not implement health.Check itself, which would add one the informer does not have.
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
