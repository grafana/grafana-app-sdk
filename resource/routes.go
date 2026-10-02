package resource

import (
	"context"
)

type RouteRequestInfo struct {
	FullIdentifier

	// The resource version for the parent object.  It will change when values change
	ResourceVersion string

	// For sub-resource requests -- this is the body o
	RawParentResource []byte

	// When the parent resource contains secure values
	DecryptedSecureValues DecryptedSecureValues
}

// The key type is unexported to prevent collisions
type contextKey int

const (
	// namespaceKey is the context key for the request namespace.
	routeInfoKey contextKey = iota
)

// WithNamespace returns a copy of parent in which the namespace value is set
func WithRouteRequestInfo(ctx context.Context, info *RouteRequestInfo) context.Context {
	return context.WithValue(ctx, routeInfoKey, info)
}

// NamespaceFrom returns the value of the namespace key on the ctx
func RouteRequestInfoFrom(ctx context.Context) *RouteRequestInfo {
	info, ok := ctx.Value(routeInfoKey).(*RouteRequestInfo)
	if ok && info != nil {
		return info
	}
	return nil
}
