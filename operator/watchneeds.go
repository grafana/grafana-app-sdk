package operator

import (
	"fmt"
	"slices"
	"strings"
)

// WatchNeeds is what a kind's handlers require of the change stream: the delivery they need, and the object
// information they read off each change notification.
//
// It states a requirement rather than naming an informer, because which informers a deployment can offer is not
// something an app ships knowing. Resolve turns it into the cheapest informer a deployment can actually serve it
// with, and every fallback is upward in capability, so an unavailable option never fails.
//
// Experimental: this API may change in a minor release.
type WatchNeeds struct {
	// Stream is the delivery the handlers require. Its zero value is WatchStreamObjects.
	Stream WatchStream
	// Metadata is the object metadata the handlers read off a change notification. It is only consulted for an
	// informer whose notifications do not carry the object itself: a notification carrying the whole object
	// satisfies every metadata need.
	Metadata WatchMetadata
}

// WatchStream is the delivery a kind's handlers require of the change stream.
type WatchStream string

const (
	// WatchStreamObjects means the handlers read objects from the informer's cache, so a cache has to exist.
	// This is the default, and the only value which requires a watch-backed informer.
	WatchStreamObjects WatchStream = "objects"
	// WatchStreamSignal means the handlers re-read the object they act on, so knowing which object changed is
	// enough. A change bus can serve this without caching any object; a watch serves it too, at more memory.
	WatchStreamSignal WatchStream = "signal"
	// WatchStreamCadence means the handlers re-evaluate every object on a schedule and individual changes need
	// not be delivered at all — age-based cleanup, for instance. A periodic re-list serves this and needs no watch.
	WatchStreamCadence WatchStream = "cadence"
)

// WatchMetadata names parts of an object's metadata. It is read two ways: as what a kind's handlers need to see on a
// change notification, and as what an informer's notifications actually carry. Resolve compares the two, which is
// why both sides use one type.
//
// Each part is named rather than announced wholesale because announcing everything would make every notification
// carry the object.
type WatchMetadata struct {
	// Labels are metadata.labels keys.
	Labels []string
	// Annotations are metadata.annotations keys.
	Annotations []string
	// Finalizers are metadata.finalizers values. A handler wrapped by OpinionatedReconciler or OpinionatedWatcher
	// needs its own finalizer, so that a delete in progress can be told from a fresh object without a read.
	Finalizers []string
	// Generation is metadata.generation, which tells a spec change from a subresource one.
	Generation bool
	// CreationTimestamp is metadata.creationTimestamp.
	CreationTimestamp bool
	// DeletionTimestamp is metadata.deletionTimestamp, which makes a delete in progress visible.
	DeletionTimestamp bool
}

// IsZero reports whether no metadata is named.
func (m WatchMetadata) IsZero() bool {
	return len(m.Labels) == 0 && len(m.Annotations) == 0 && len(m.Finalizers) == 0 &&
		!m.Generation && !m.CreationTimestamp && !m.DeletionTimestamp
}

// Missing returns the parts of m which announced does not carry, so that a deployment whose producer announces less
// than its consumers read can say which keys are short rather than only that something is.
func (m WatchMetadata) Missing(announced WatchMetadata) WatchMetadata {
	return WatchMetadata{
		Labels:            missingValues(m.Labels, announced.Labels),
		Annotations:       missingValues(m.Annotations, announced.Annotations),
		Finalizers:        missingValues(m.Finalizers, announced.Finalizers),
		Generation:        m.Generation && !announced.Generation,
		CreationTimestamp: m.CreationTimestamp && !announced.CreationTimestamp,
		DeletionTimestamp: m.DeletionTimestamp && !announced.DeletionTimestamp,
	}
}

// SatisfiedBy reports whether announced carries every part of m.
func (m WatchMetadata) SatisfiedBy(announced WatchMetadata) bool {
	return m.Missing(announced).IsZero()
}

// String names the parts of m, for logs and errors.
func (m WatchMetadata) String() string {
	parts := make([]string, 0, 6)
	for _, label := range m.Labels {
		parts = append(parts, "labels."+label)
	}
	for _, annotation := range m.Annotations {
		parts = append(parts, "annotations."+annotation)
	}
	for _, finalizer := range m.Finalizers {
		parts = append(parts, "finalizers."+finalizer)
	}
	if m.Generation {
		parts = append(parts, "generation")
	}
	if m.CreationTimestamp {
		parts = append(parts, "creationTimestamp")
	}
	if m.DeletionTimestamp {
		parts = append(parts, "deletionTimestamp")
	}
	return strings.Join(parts, ", ")
}

func missingValues(needed, announced []string) []string {
	var missing []string
	for _, value := range needed {
		if !slices.Contains(announced, value) {
			missing = append(missing, value)
		}
	}
	return missing
}

// InformerKind names a kind of informer which can serve a change stream.
type InformerKind string

const (
	// InformerKindWatch is a watch-backed informer, which caches every object of the kind. Its notifications
	// carry the whole object, so it satisfies every WatchNeeds.
	InformerKindWatch InformerKind = "watch"
	// InformerKindBus is a change-bus informer, which caches no object body and carries only the metadata its
	// producer announces.
	InformerKindBus InformerKind = "bus"
	// InformerKindPeriodic is a re-list-only informer, which keeps nothing between passes.
	InformerKindPeriodic InformerKind = "periodic"
)

// InformerOffer is an informer a deployment can provide, and what its change notifications carry. Availability is
// stated rather than assumed because a deployment may run no change bus, and an app ships without knowing.
//
// Experimental: this API may change in a minor release.
type InformerOffer struct {
	// Kind names the informer.
	Kind InformerKind
	// Announces is the object metadata this informer's notifications carry, for an informer whose notifications do
	// not carry the object itself. A nil value means they carry the whole object, which satisfies every metadata
	// need; a non-nil value is exhaustive, so a need it does not name is a need this informer cannot serve.
	Announces *WatchMetadata
}

// satisfies reports whether the offered informer can serve the need: its delivery is in the need's preference list,
// which the caller has already established, and its notifications carry every part of the metadata the need reads.
func (o InformerOffer) satisfies(needs WatchNeeds) bool {
	if o.Announces == nil {
		return true
	}
	return needs.Metadata.SatisfiedBy(*o.Announces)
}

// Resolve reports the cheapest of the available informers which satisfies the need, and the reason a cheaper one
// was passed over when one was. The rule lives on the need itself, so wiring which builds informers directly — a
// standalone operator, rather than an App — reaches the same answer as simple.App does.
//
// An informer is passed over when it does not announce metadata the handlers read: a reconciler which decides from a
// finalizer cannot be served by a bus whose producer does not announce that finalizer, even though the bus serves
// the delivery it asked for. The fallback is always upward in capability, so it is safe, and the reason is a
// diagnostic rather than an error — the returned informer serves the need either way. A need for a signal with no
// usable bus resolves to a watch, which delivers strictly more at the cost of more memory; a need nothing available
// satisfies resolves to a watch, which satisfies every need.
func (n WatchNeeds) Resolve(available ...InformerOffer) (InformerKind, string) {
	var preference []InformerKind
	switch n.Stream {
	case WatchStreamCadence:
		preference = []InformerKind{InformerKindPeriodic, InformerKindBus, InformerKindWatch}
	case WatchStreamSignal:
		preference = []InformerKind{InformerKindBus, InformerKindWatch}
	default: // WatchStreamObjects, and an unset value which defaults to it
		preference = []InformerKind{InformerKindWatch}
	}
	// The first reason is the one worth hearing: it is the cheapest option the deployment lost.
	passedOver := ""
	for _, candidate := range preference {
		for _, offer := range available {
			if offer.Kind != candidate {
				continue
			}
			if !offer.satisfies(n) {
				if passedOver == "" {
					passedOver = fmt.Sprintf("the %s informer does not announce %s, which this kind's handlers read",
						offer.Kind, n.Metadata.Missing(*offer.Announces))
				}
				continue
			}
			return candidate, passedOver
		}
	}
	return InformerKindWatch, passedOver
}
