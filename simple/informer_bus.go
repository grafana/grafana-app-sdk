package simple

import (
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// BusInformerConfig configures the informers BusInformerSupplier creates.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusInformerConfig struct {
	// RelistInformerConfig configures the re-list each informer is driven by. Its SourceFactory is ignored: the
	// source is the bus.
	RelistInformerConfig
	// Payload selects which object live events deliver. An empty value is operator.PayloadIdentity, which delivers
	// identity only and so cannot be combined with an opinionated reconciler or watcher.
	Payload operator.BusPayload
	// Group joins the app's replicas into a group across which the bus shares delivery, so that each event reaches
	// one replica rather than all of them. A zero Group delivers every event to every replica.
	Group resourcebus.Group
	// BufferSize is how many events each informer's bus watch holds before it counts them as lost and re-lists.
	// A non-positive value uses operator.DefaultBusWatchBufferSize.
	BufferSize int
}

// BusInformerSupplier returns an InformerSupplier which creates operator.RelistInformer instances whose changes
// between re-lists come from sub, through an operator.BusWatchClient which reads through each kind's client.
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
	relist := cfg.RelistInformerConfig
	relist.SourceFactory = func(kind resource.Kind, client resource.Client, options operator.InformerOptions) (operator.ChangeSource, error) {
		return operator.NewBusWatchClient(client, sub, kind, operator.BusWatchOptions{
			Payload:      cfg.Payload,
			Group:        cfg.Group,
			BufferSize:   cfg.BufferSize,
			ErrorHandler: options.ErrorHandler,
		})
	}
	return RelistInformerSupplier(relist)
}
