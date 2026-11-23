package resource

import (
	"context"
	"encoding/json"
)

// RouteRequestInfo describes the resource associated with a custom route request.
type RouteRequestInfo struct {
	FullIdentifier

	// The resource version for the parent object.  It will change when values change
	ResourceVersion string `json:"resourceVersion,omitempty"`

	// Parent contains the raw parent object for subresource requests.
	Parent json.RawMessage `json:"parent,omitempty"`

	// When the parent resource contains secure values
	DecryptedSecureValues DecryptedSecureValues `json:"decryptedSecureValues,omitempty"`
}

// The key type is unexported to prevent collisions
type contextKey int

const (
	// routeInfoKey is the context key for custom route request metadata.
	routeInfoKey contextKey = iota
)

// WithRouteRequestInfo returns a context containing the custom route request metadata.
func WithRouteRequestInfo(ctx context.Context, info *RouteRequestInfo) context.Context {
	return context.WithValue(ctx, routeInfoKey, info)
}

// RouteRequestInfoFrom returns the custom route request metadata, or nil if absent.
func RouteRequestInfoFrom(ctx context.Context) *RouteRequestInfo {
	info, ok := ctx.Value(routeInfoKey).(*RouteRequestInfo)
	if ok && info != nil {
		return info
	}
	return nil
}
