package grpcplugin

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/grafana/grafana-app-sdk/k8s"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// idTokenMetadataKey is the gRPC metadata key that carries the caller's Grafana
// ID token from Grafana to the plugin.
const idTokenMetadataKey = "x-grafana-id" //nolint:gosec // A metadata key, not a credential.

// outgoingCallerContext sends the caller's ID token on ctx (see k8s.WithIDToken)
// as gRPC metadata.
func outgoingCallerContext(ctx context.Context) context.Context {
	token, ok := k8s.IDTokenFromContext(ctx)
	if !ok {
		return ctx
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(idTokenMetadataKey, token)
	return metadata.NewOutgoingContext(ctx, md)
}

// incomingCallerContext puts the caller's ID token sent as gRPC metadata on
// ctx, so token exchange clients, such as those built with
// plugin.BuildKubeConfig, act as the caller.
func incomingCallerContext(ctx context.Context) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)
	if tokens := md.Get(idTokenMetadataKey); len(tokens) > 0 {
		return k8s.WithIDToken(ctx, tokens[0])
	}
	return ctx
}

// The types below pass the caller's ID token between Grafana and the plugin
// for every v3 RPC, so the services themselves don't need to.

type callerAdmissionClient struct {
	pluginv3.AdmissionServiceClient
}

func (c callerAdmissionClient) AdmissionReview(ctx context.Context, in *pluginv3.AdmissionReviewRequest, opts ...grpc.CallOption) (*pluginv3.AdmissionReviewResponse, error) {
	return c.AdmissionServiceClient.AdmissionReview(outgoingCallerContext(ctx), in, opts...)
}

type callerConversionClient struct {
	pluginv3.ConversionServiceClient
}

func (c callerConversionClient) ConvertObjects(ctx context.Context, in *pluginv3.ConvertObjectsRequest, opts ...grpc.CallOption) (*pluginv3.ConvertObjectsResponse, error) {
	return c.ConversionServiceClient.ConvertObjects(outgoingCallerContext(ctx), in, opts...)
}

type callerRouteClient struct {
	pluginv3.RouteServiceClient
}

func (c callerRouteClient) CallRoute(ctx context.Context, in *pluginv3.CallRouteRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pluginv3.CallRouteResponse], error) {
	return c.RouteServiceClient.CallRoute(outgoingCallerContext(ctx), in, opts...)
}

type callerAdmissionServer struct {
	pluginv3.AdmissionServiceServer
}

func (s callerAdmissionServer) AdmissionReview(ctx context.Context, req *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	return s.AdmissionServiceServer.AdmissionReview(incomingCallerContext(ctx), req)
}

type callerConversionServer struct {
	pluginv3.ConversionServiceServer
}

func (s callerConversionServer) ConvertObjects(ctx context.Context, req *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	return s.ConversionServiceServer.ConvertObjects(incomingCallerContext(ctx), req)
}

type callerRouteServer struct {
	pluginv3.RouteServiceServer
}

func (s callerRouteServer) CallRoute(req *pluginv3.CallRouteRequest, stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	return s.RouteServiceServer.CallRoute(req, callerStream[pluginv3.CallRouteResponse]{
		ServerStreamingServer: stream,
		ctx:                   incomingCallerContext(stream.Context()),
	})
}

// callerStream replaces a server stream's context.
type callerStream[T any] struct {
	grpc.ServerStreamingServer[T]
	ctx context.Context
}

func (s callerStream[T]) Context() context.Context {
	return s.ctx
}
