// Package resourcebus describes the contract between an app and a delta source which announces resource changes out
// of band of the kubernetes watch, such as a message bus the storage layer publishes change notifications to.
//
// It is deliberately a leaf package: it depends on the standard library and apimachinery's schema types only, so an
// adapter for a concrete bus can implement it without taking on the rest of the SDK.
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
type Subscriber interface {
	// Subscribe delivers every change to the given resource in the given namespace (an empty namespace means every
	// namespace) to handler until ctx is done or the returned Subscription is closed.
	// handler must not block: it is called on the delivery path.
	Subscribe(ctx context.Context, gvr schema.GroupVersionResource, namespace string,
		handler func(context.Context, Event), opts ...SubscribeOption) (Subscription, error)
}

// Subscription is an open subscription to a bus.
type Subscription interface {
	// WaitReady blocks until the subscription is able to receive events, or ctx is done.
	// ctx must carry a deadline: an implementation which cannot report readiness otherwise never returns.
	WaitReady(ctx context.Context) error
	// Close stops delivery to the handler and releases the subscription's resources.
	Close() error
}

// SubscribeConfig is the set of options a Subscriber applies to one subscription.
type SubscribeConfig struct {
	// SharedGroup is the name of a group of subscribers which share delivery: each event reaches exactly one member
	// of the group rather than all of them. An empty name subscribes outside any group, so every subscriber receives
	// every event. A grouped subscriber sees only a subset of the live events, which is why an informer over a shared
	// group relies on its periodic re-list rather than the live stream to stay reconciled.
	SharedGroup string
	// OnReconnect is called each time the subscription is re-established after a gap. Events published during the gap
	// were never delivered, so a consumer uses this to trigger whatever recovery it has. It must not block.
	OnReconnect func()
}

// SubscribeOption configures one subscription.
type SubscribeOption func(*SubscribeConfig)

// WithSharedGroup joins the subscription to a named group of subscribers which share delivery.
// See SubscribeConfig.SharedGroup.
func WithSharedGroup(name string) SubscribeOption {
	return func(cfg *SubscribeConfig) {
		cfg.SharedGroup = name
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
