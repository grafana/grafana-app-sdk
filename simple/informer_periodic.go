package simple

import (
	"errors"
	"time"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// PeriodicInformerConfig configures the informers PeriodicInformerMechanism creates.
//
// Experimental: this API may change in a minor release.
type PeriodicInformerConfig struct {
	// SweepInterval is the cadence for kinds which state none of their own in WatchOptions.Sweep.Every.
	// A non-positive value in both uses operator.DefaultPeriodicSweepInterval.
	SweepInterval time.Duration
	// PageSize is the page size the default lister requests for kinds which state none of their own in
	// WatchOptions.Sweep.PageSize. An empty value in both uses operator.DefaultListerPageSize.
	PageSize int
	// ListerFactory replaces the ResourceLister for a kind, for a server whose list does not fit a paginated list
	// over the kind's client — one which can project less than whole objects, say. When nil, that paginated list
	// is used, with the namespace and selectors the informer was configured with.
	//
	// A factory must honor the ListWatchOptions it is given: nothing downstream re-applies them, since the list is
	// the informer's only source.
	ListerFactory func(kind resource.Kind, opts operator.ClientListerOptions) (operator.ResourceLister, error)
}

// PeriodicInformerMechanism is the re-list: operator.PeriodicInformer instances with no watch, no subscription and
// no snapshot, which list the kind on a cadence and deliver every listed object as an add.
//
// It is a sweep and nothing else, so a deployment lists it among its mechanisms and the kinds which need no live
// channel are served by it — see NewSelectingInformerSupplier. A kind needing one is passed over, and refused
// outright if something hands it this mechanism anyway. It states that for itself, so listing it cannot advertise
// the wrong thing.
//
// Each kind sweeps at the cadence and page size it asked for in WatchOptions.Sweep, so one mechanism serves kinds
// which re-evaluate at different rates and read their sets in different sized pages.
//
// Experimental: this API may change in a minor release.
func PeriodicInformerMechanism(cfg PeriodicInformerConfig) InformerMechanism {
	return periodicMechanism{cfg: cfg}
}

// PeriodicInformerSupplier is PeriodicInformerMechanism's informers as a plain InformerSupplier, for an app whose
// every kind is served by the re-list and for wiring an operator without an App. Prefer the mechanism where
// anything selects, so that nothing has to restate what this provides. A kind served this way is still checked,
// since the informer says what it provides.
//
// Experimental: this API may change in a minor release.
func PeriodicInformerSupplier(cfg PeriodicInformerConfig) InformerSupplier {
	return periodicMechanism{cfg: cfg}.NewInformer
}

type periodicMechanism struct {
	cfg PeriodicInformerConfig
}

var _ InformerMechanism = periodicMechanism{}

// WatchOptions is what every informer this mechanism builds provides, whatever the kind: a sweep carrying whole
// objects to every instance, and no live channel.
func (periodicMechanism) WatchOptions(resource.Kind) operator.WatchOptions {
	return operator.WatchOptions{
		Sweep: &operator.WatchSweepOptions{
			Detail:       operator.WatchDetailFullObject,
			Distribution: operator.WatchDistributionAll,
		},
	}
}

func (m periodicMechanism) NewInformer(
	kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions,
) (operator.Informer, error) {
	lister, err := m.lister(kind, clients, options)
	if err != nil {
		return nil, err
	}

	inf, err := operator.NewPeriodicInformer(kind, lister, operator.PeriodicInformerOptions{
		SweepInterval: m.sweepInterval(options),
		ErrorHandler:  options.ErrorHandler,
	})
	if err != nil {
		return nil, err
	}
	// Watchers run on their own per-key workers, as they do under the other suppliers: a slow handler on the
	// informer's own loop would hold up the rest of the sweep and the next one.
	return operator.NewConcurrentInformerFromOptions(inf, options)
}

// sweepInterval is the cadence the kind asked for, which is the whole point of a mechanism taking one rather than
// offering it. CacheResyncInterval is read as well because that is where the App puts the same value for the
// informers which read nothing else, and where an operator wiring this by hand would put it.
func (m periodicMechanism) sweepInterval(options operator.InformerOptions) time.Duration {
	if sweep := options.WatchNeeds.Sweep; sweep != nil && sweep.Every > 0 {
		return sweep.Every
	}
	if options.CacheResyncInterval > 0 {
		return options.CacheResyncInterval
	}
	return m.cfg.SweepInterval
}

// pageSize is the page size the kind asked for, read the same two ways as the cadence.
func (m periodicMechanism) pageSize(options operator.InformerOptions) int {
	if sweep := options.WatchNeeds.Sweep; sweep != nil && sweep.PageSize > 0 {
		return int(sweep.PageSize)
	}
	if options.WatchListPageSize > 0 {
		return int(options.WatchListPageSize)
	}
	return m.cfg.PageSize
}

// lister builds the kind's ResourceLister. The client is read for it only where the default list needs one, so an
// operator wiring this outside an App — with its own clients and no resource.ClientGenerator to hand over — can
// pass a nil generator along with a ListerFactory.
func (m periodicMechanism) lister(
	kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions,
) (operator.ResourceLister, error) {
	opts := operator.ClientListerOptions{
		ListWatchOptions: options.ListWatchOptions,
		PageSize:         m.pageSize(options),
	}
	if m.cfg.ListerFactory != nil {
		return m.cfg.ListerFactory(kind, opts)
	}
	if clients == nil {
		return nil, errors.New("a client generator is required to list the kind")
	}
	client, err := clients.ClientFor(kind)
	if err != nil {
		return nil, err
	}
	return operator.NewClientLister(client, kind, opts)
}
