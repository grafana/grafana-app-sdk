// Package resourcebus describes the contract between an app and a bus which announces resource changes out of band
// of the kubernetes watch, such as a message bus the storage layer publishes change notifications to. It is what an
// informer subscribes to in place of a watch.
//
// It is deliberately a leaf package: it depends on the standard library and apimachinery's schema types only, so an
// adapter for a concrete bus can implement it without taking on the rest of the SDK.
//
// Experimental: the resource change bus is a work in progress. Its APIs may change or be removed
// without notice. Use it only for experimentation.
package resourcebus

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Verb is the kind of change an Event announces.
type Verb string

const (
	// VerbAdded announces an object which has been created.
	VerbAdded Verb = "added"
	// VerbModified announces an object which has been updated. A delete which only sets a deletion timestamp
	// (so that finalizers can run) is a modification, not a VerbDeleted.
	VerbModified Verb = "modified"
	// VerbDeleted announces an object which has been removed from storage.
	VerbDeleted Verb = "deleted"
)

// Event is a single change announcement. It carries identity only: a subscriber which needs the object reads it from
// the API, because the bus is a signal that something changed, not a replica of storage.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Event struct {
	// Verb is the kind of change which occurred.
	Verb Verb
	// Namespace of the changed object. Empty for cluster-scoped kinds.
	Namespace string
	// Name of the changed object.
	Name string
	// ResourceVersion the change was written at. It is opaque, as the kubernetes API contract requires:
	// consumers which need to order changes must document their own assumption about its format,
	// and degrade when it does not hold.
	ResourceVersion string
}

// Subscriber opens subscriptions to a bus which announces resource changes.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Subscriber interface {
	// Subscribe delivers every change to the given resource in the given namespace (an empty namespace means every
	// namespace) to handler until ctx is done or the returned Subscription is closed.
	// handler must not block: it is called on the delivery path.
	Subscribe(ctx context.Context, gvr schema.GroupVersionResource, namespace string,
		handler func(context.Context, Event), opts ...SubscribeOption) (Subscription, error)
}

// Subscription is an open subscription to a bus.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Subscription interface {
	// WaitReady blocks until the subscription is able to receive events, or ctx is done.
	// ctx must carry a deadline: an implementation which cannot report readiness otherwise never returns.
	WaitReady(ctx context.Context) error
	// Close stops delivery to the handler and releases the subscription's resources.
	Close() error
}

// GroupDelivery is how a bus shares delivery of an event among the members of a Group.
//
// A Subscriber which cannot provide the requested delivery must fail the subscription rather than substituting
// another: the difference decides whether an event reaches one member or all of them, so a silent substitution would
// either duplicate every event across the members or drop the events a member was meant to handle.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type GroupDelivery string

const (
	// GroupDeliveryDefault leaves the sharing to the bus, which is what a subscriber asks for when it needs the
	// members to divide the events between them and does not care how.
	GroupDeliveryDefault GroupDelivery = ""
	// GroupDeliveryRoundRobin asks the bus to hand each event to one member in turn, with no affinity between an
	// object and the member which handles its events.
	GroupDeliveryRoundRobin GroupDelivery = "round-robin"
)

// Group is a set of subscribers which share delivery of a resource's events: each event reaches one member of the
// group rather than all of them, so a member sees only a subset of the live stream. A consumer which must observe
// every change therefore cannot rely on the live stream alone — an informer over a group stays reconciled through
// its periodic re-list.
//
// A zero Group is no group at all: every subscriber receives every event.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Group struct {
	// Name identifies the group. Subscribers which pass the same name for the same resource and namespace share
	// delivery; an empty name is no group.
	Name string
	// Delivery is how the bus shares events among the members. An empty value leaves it to the bus.
	Delivery GroupDelivery
}

// SubscribeConfig is the set of options a Subscriber applies to one subscription.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type SubscribeConfig struct {
	// Group is the group of subscribers this subscription joins, which share delivery between them rather than each
	// receiving every event. A zero Group subscribes outside any group. See Group.
	Group Group
	// OnReconnect is called each time the subscription is re-established after a gap. Events published during the gap
	// were never delivered, so a consumer uses this to trigger whatever recovery it has. It must not block.
	OnReconnect func()
}

// SubscribeOption configures one subscription.
type SubscribeOption func(*SubscribeConfig)

// WithGroup joins the subscription to a group of subscribers which share delivery. See Group.
func WithGroup(group Group) SubscribeOption {
	return func(cfg *SubscribeConfig) {
		cfg.Group = group
	}
}

// WithOnReconnect registers a callback invoked each time the subscription is re-established after a gap.
// See SubscribeConfig.OnReconnect.
func WithOnReconnect(fn func()) SubscribeOption {
	return func(cfg *SubscribeConfig) {
		cfg.OnReconnect = fn
	}
}

// NewSubscribeConfig applies opts in order and returns the resulting config, so that Subscriber implementations
// resolve options the same way.
func NewSubscribeConfig(opts ...SubscribeOption) SubscribeConfig {
	cfg := SubscribeConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}
