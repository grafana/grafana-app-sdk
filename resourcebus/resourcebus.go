// Package resourcebus defines the interface between an app and a bus that announces resource changes outside the
// Kubernetes watch, such as a message bus that the storage layer publishes to. The bus informers in the operator
// package subscribe to it instead of a watch.
//
// An adapter for a specific bus implements Subscriber. This package depends only on the standard library and
// apimachinery's schema types, so an adapter does not need the rest of the SDK.
//
// Experimental: the resource change bus is a work in progress. Its APIs may change or be removed
// without notice. Use it only for experimentation.
package resourcebus

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Verb is the type of change that an Event announces.
type Verb string

const (
	// VerbAdded announces that an object was created.
	VerbAdded Verb = "added"
	// VerbModified announces that an object was updated. A delete that only sets the deletion timestamp (so that
	// finalizers can run) is a VerbModified, not a VerbDeleted.
	VerbModified Verb = "modified"
	// VerbDeleted announces that an object was removed from storage.
	VerbDeleted Verb = "deleted"
)

// Event announces one change. It contains the object's identity, but never the object. A subscriber that needs the
// object reads it from the API.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Event struct {
	// Verb is the type of change.
	Verb Verb
	// Namespace of the changed object. Empty for cluster-scoped kinds.
	Namespace string
	// Name of the changed object.
	Name string
	// ResourceVersion is the version that the change was written at. Treat it as opaque. A consumer that orders
	// changes by it must document what format it expects. NewBusRecovery expects increasing integers.
	ResourceVersion string
}

// ErrSubscriptionRejected marks a permanent subscription failure, such as missing permissions or an unsupported
// delivery mode. Wrap it in the error, so the informer stops and reports the error instead of retrying.
var ErrSubscriptionRejected = errors.New("resource bus subscription rejected")

// Subscriber opens subscriptions to a bus that announces resource changes.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Subscriber interface {
	// Subscribe delivers change notifications for gvr in namespace to handler, at most once each, until ctx is done
	// or the Subscription is closed. An empty namespace means every namespace. handler must not block.
	Subscribe(ctx context.Context, gvr schema.GroupVersionResource, namespace string,
		handler func(context.Context, Event), opts ...SubscribeOption) (Subscription, error)
}

// Subscription is an open subscription to a bus.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Subscription interface {
	// WaitReady blocks until the subscription can receive events, or until ctx is done. ctx always has a deadline.
	WaitReady(ctx context.Context) error
	// Close stops delivery to the handler and releases the subscription.
	Close() error
}

// GroupDelivery is how a bus shares events among the members of a Group.
//
// A Subscriber that cannot provide the requested delivery must return an error. It must not use a different delivery
// mode instead, because that would duplicate or drop events.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type GroupDelivery string

const (
	// GroupDeliveryDefault lets the bus decide how the members divide the events. It still divides them: it does
	// not mean broadcast.
	GroupDeliveryDefault GroupDelivery = ""
	// GroupDeliveryRoundRobin sends each event to one member, with no affinity between an object and a member.
	GroupDeliveryRoundRobin GroupDelivery = "round-robin"
)

// Group shares delivery among subscribers. Each notification goes to at most one member, so a member that needs
// every change must recover missed changes another way. An empty Name means broadcast: every subscriber gets every
// notification.
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Group struct {
	// Name identifies the group. Subscribers with the same Name, resource, and namespace share delivery. An empty
	// Name means no group.
	Name string
	// Delivery is how the bus shares events among the members. Empty means GroupDeliveryDefault.
	Delivery GroupDelivery
}

// SubscribeConfig holds the options for one subscription. Build it with NewSubscribeConfig.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type SubscribeConfig struct {
	// Group is the group that this subscription joins. A zero Group means no group, so this subscriber gets every
	// event.
	Group Group
	// OnReconnect is called each time the subscription connects again after a gap. Events from the gap are lost, so
	// the consumer can use it to start a recovery. It must not block.
	OnReconnect func()
}

// SubscribeOption configures one subscription.
type SubscribeOption func(*SubscribeConfig)

// WithGroup joins the subscription to group. See Group.
func WithGroup(group Group) SubscribeOption {
	return func(cfg *SubscribeConfig) {
		cfg.Group = group
	}
}

// WithOnReconnect sets fn as SubscribeConfig.OnReconnect.
func WithOnReconnect(fn func()) SubscribeOption {
	return func(cfg *SubscribeConfig) {
		cfg.OnReconnect = fn
	}
}

// NewSubscribeConfig applies opts in order and returns the result. Subscriber implementations should use it to read
// their options.
func NewSubscribeConfig(opts ...SubscribeOption) SubscribeConfig {
	cfg := SubscribeConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}
