package simple

import (
	"errors"
	"time"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// BusInformerConfig configures the informers BusInformerSupplier creates.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusInformerConfig struct {
	// Payload selects which object live events deliver. An empty value is operator.PayloadIdentity, which carries
	// the identity of the changed object plus the metadata the bus announces with it, and so serves the kinds
	// declaring operator.WatchDetailIdentity. operator.PayloadHydrated reads each object before dispatching, at one
	// API request per event, and so serves every kind.
	Payload operator.BusPayload
	// ResyncInterval is how often the full set is re-listed. The re-list is what catches changes the live stream
	// did not deliver, so it is a correctness interval rather than a tuning knob — it bounds how long a replica can
	// be stale. A non-positive value uses operator.DefaultBusResyncInterval.
	ResyncInterval time.Duration
	// Group joins the app's replicas into a group across which the bus shares delivery, so that each event reaches
	// one replica rather than all of them. A zero Group delivers every event to every replica.
	Group resourcebus.Group
	// AllowDegradedStart lets an informer start re-list-only when the bus is unavailable, instead of holding its
	// initial list until a subscription opens. See operator.BusInformerOptions.AllowDegradedStart.
	AllowDegradedStart bool
	// AnnouncedMetadata declares what this app's bus announces with each event, which is what an identity delivery
	// carries beyond the identity and what a label filter on one can be matched against.
	// See operator.AnnouncedMetadata.
	AnnouncedMetadata operator.AnnouncedMetadata
	// PageSize is the page size the default lister requests. An empty value uses operator.DefaultListerPageSize.
	PageSize int
	// ListerFactory replaces the ResourceLister for a kind, for a server whose list does not fit a paginated list
	// over the kind's client — one which can project less than whole objects, say. When nil, that paginated list
	// is used, with the namespace and selectors the informer was configured with.
	//
	// A factory must honor the ListWatchOptions it is given: nothing downstream re-applies them to the list, and a
	// listed object outside them is dropped rather than narrowing the set the server returned.
	ListerFactory func(kind resource.Kind, opts operator.ClientListerOptions) (operator.ResourceLister, error)
}

// BusInformerSupplier returns an InformerSupplier which creates operator.BusInformer instances driven by sub: the
// live stream comes from the bus rather than a kubernetes watch, and the informer keeps no per-object cache.
//
// A nil subscriber returns a nil supplier, which as an InformerSupplierOffer is a mechanism this deployment cannot
// provide — so an app wired for a bus which is switched off falls through to the next offer, or to the API watch,
// without branching at the call site.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func BusInformerSupplier(sub resourcebus.Subscriber, cfg BusInformerConfig) InformerSupplier {
	if sub == nil {
		return nil
	}
	return func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
		client, lister, err := cfg.lister(kind, clients, options)
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

// lister builds the kind's ResourceLister, and returns the client it reads through where the configuration needs
// one: the default list, and the hydrated payload's read per event. With a ListerFactory and an identity payload
// nothing is read through a client, so an operator wiring this outside an App — with its own clients and no
// resource.ClientGenerator to hand over — can pass a nil generator.
func (c BusInformerConfig) lister(
	kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions,
) (resource.Client, operator.ResourceLister, error) {
	opts := operator.ClientListerOptions{
		ListWatchOptions: options.ListWatchOptions,
		PageSize:         c.PageSize,
	}

	var client resource.Client
	if c.ListerFactory == nil || c.Payload == operator.PayloadHydrated {
		if clients == nil {
			return nil, nil, errors.New("a client generator is required to list the kind and to hydrate its events")
		}
		var err error
		if client, err = clients.ClientFor(kind); err != nil {
			return nil, nil, err
		}
	}
	if c.ListerFactory != nil {
		lister, err := c.ListerFactory(kind, opts)
		return client, lister, err
	}
	lister, err := operator.NewClientLister(client, kind, opts)
	return client, lister, err
}
