package operator

import "context"

// Delivery is how an event reached a ResourceWatcher. An informer which can tell the difference sets it on the
// context it dispatches with, so a watcher that cares reads it with DeliveryFromContext and one that does not is
// unaffected.
//
// The distinction is load-bearing for instrumentation which attributes an event to its cause: a live event is one the
// change source announced, carrying the version that announcement was made at, while a re-list event is the
// informer's own diff of a list against its snapshot, carrying a version the lister read. Measuring one against the
// other — the delay between a notification and a reconcile's read of it, say — is only meaningful for live events.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Delivery string

const (
	// DeliveryUnknown is the zero value, reported for an informer which does not distinguish the two. A watch-backed
	// informer is in this case: a resync replays its cache rather than re-listing, so what it dispatches is neither.
	DeliveryUnknown Delivery = ""
	// DeliveryLive is an event the change source announced.
	DeliveryLive Delivery = "live"
	// DeliveryRelist is an event the informer derived from a list: a change the live stream did not deliver here, a
	// re-delivery of an unchanged object, or the initial list.
	DeliveryRelist Delivery = "relist"
)

type deliveryContextKey struct{}

// DeliveryFromContext reports how the event being handled reached the watcher, or DeliveryUnknown outside a dispatch
// and for an informer which does not distinguish.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func DeliveryFromContext(ctx context.Context) Delivery {
	delivery, ok := ctx.Value(deliveryContextKey{}).(Delivery)
	if !ok {
		return DeliveryUnknown
	}
	return delivery
}

// ContextWithDelivery returns ctx carrying how an event reached a watcher. Informers call it before dispatching; an
// informer implemented outside the SDK uses it to expose the same signal.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func ContextWithDelivery(ctx context.Context, delivery Delivery) context.Context {
	return context.WithValue(ctx, deliveryContextKey{}, delivery)
}
