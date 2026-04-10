package operator

import (
	"context"

	"github.com/grafana/grafana-app-sdk/resource"
)

// ShardFilter determines whether the current replica should process a resource event.
// It is intended for use in HA watcher or reconciler deployments where all replicas
// observe the same objects, but only one replica should handle a given object.
//
// ShouldProcess is called on the hot event path before delegating to the wrapped
// watcher or reconciler. Implementations should therefore return promptly and avoid
// slow or unnecessary work where possible.
//
// Returning false with a nil error means the object definitively belongs to a
// different replica and the event should be skipped. Returning a non-nil error means
// the filter could not determine ownership and the caller should treat the event as
// failed according to its normal error-handling policy.
//
// Implementations must respect context cancellation and deadlines. The provided
// object is the event snapshot used for shard selection and may not reflect newer
// storage state.
type ShardFilter interface {
	ShouldProcess(context.Context, resource.Object) (bool, error)
}
