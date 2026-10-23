// Package v3 defines the clients a host uses to call plugin protocol v3
// services. grpcplugin.NewClientV3 and grpcplugin.WithAuthentication return
// implementations that authenticate each request as its caller, so the
// methods take no gRPC call options. NewLazyClient resolves a plugin's client
// from a ClientLoader on each request.
//
// Unlike protocol v2, v3 requests carry no plugin context (the plugin's
// instance settings), and plugins serve every tenant from one process, so
// implementations must be safe for multi-tenant use.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
package v3

import (
	"context"

	"google.golang.org/grpc"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// AdmissionClient calls a plugin's admission service.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type AdmissionClient interface {
	AdmissionReview(ctx context.Context, in *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error)
}

// ConversionClient calls a plugin's conversion service.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type ConversionClient interface {
	ConvertObjects(ctx context.Context, in *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error)
}

// RouteClient calls a plugin's routes.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type RouteClient interface {
	CallRoute(ctx context.Context, in *pluginv3.CallRouteRequest) (grpc.ServerStreamingClient[pluginv3.CallRouteResponse], error)
}

// Client calls all of a plugin's v3 services.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type Client interface {
	AdmissionClient
	ConversionClient
	RouteClient
}
