package simple

import (
	"errors"
	"time"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// PeriodicInformerConfig configures the informers PeriodicInformerSupplier creates.
//
// Experimental: this API may change in a minor release.
type PeriodicInformerConfig struct {
	// ResyncInterval is how often the kind is re-listed, which for this informer is how often its handlers run at
	// all. A non-positive value uses operator.DefaultPeriodicResyncInterval.
	ResyncInterval time.Duration
	// PageSize is the page size the default lister requests. An empty value uses operator.DefaultListerPageSize.
	PageSize int
	// ListerFactory replaces the ResourceLister for a kind, for a server whose list does not fit a paginated list
	// over the kind's client — one which can project less than whole objects, say. When nil, that paginated list
	// is used, with the namespace and selectors the informer was configured with.
	//
	// A factory must honor the ListWatchOptions it is given: nothing downstream re-applies them, since the list is
	// the informer's only source.
	ListerFactory func(kind resource.Kind, opts operator.ClientListerOptions) (operator.ResourceLister, error)
}

// PeriodicInformerSupplier returns an InformerSupplier which creates operator.PeriodicInformer instances: no watch,
// no subscription and no snapshot, just a re-list on a cadence delivering every listed object as an add.
//
// The informers it builds deliver whole objects on the schedule, so it is offered against
// operator.WatchTriggerSchedule and serves the kinds which declare they need no more — see
// NewInformerSupplierDecider. A kind needing a delivery per change is refused when the app is constructed rather
// than served by this.
//
// Experimental: this API may change in a minor release.
func PeriodicInformerSupplier(cfg PeriodicInformerConfig) InformerSupplier {
	return func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		lister, err := cfg.lister(kind, clients, options)
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
		// Watchers run on their own per-key workers, as they do under the other suppliers: a slow handler on the
		// informer's own loop would hold up the rest of the pass and the next one.
		return operator.NewConcurrentInformerFromOptions(inf, options)
	}
}

// lister builds the kind's ResourceLister. The client is read for it only where the default list needs one, so an
// operator wiring this outside an App — with its own clients and no resource.ClientGenerator to hand over — can
// pass a nil generator along with a ListerFactory.
func (c PeriodicInformerConfig) lister(
	kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions,
) (operator.ResourceLister, error) {
	opts := operator.ClientListerOptions{
		ListWatchOptions: options.ListWatchOptions,
		PageSize:         c.PageSize,
	}
	if c.ListerFactory != nil {
		return c.ListerFactory(kind, opts)
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
