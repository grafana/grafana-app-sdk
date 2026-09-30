package grpcplugin

import (
	"context"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

func authenticate(ctx context.Context, authenticator authn.Authenticator) (context.Context, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	info, err := authenticator.Authenticate(ctx, authn.NewGRPCTokenProvider(md))
	if err != nil {
		if authn.IsUnauthenticatedErr(err) {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	if info == nil {
		return nil, status.Error(codes.Unauthenticated, "authenticator returned no identity")
	}
	// authlib's AuthInfo does not retain the raw access token. Keep the token
	// only after successful authentication so downstream clients can exchange
	// it without losing the caller's identity or delegation chain.
	token, _ := authn.NewGRPCTokenProvider(md).AccessToken(ctx)
	return types.WithAuthInfo(ctx, &authenticatedAuthInfo{AuthInfo: info, accessToken: token}), nil
}

type authenticatedAdmissionServer struct {
	pluginv3.AdmissionServiceServer
	authenticator authn.Authenticator
}

func (s *authenticatedAdmissionServer) AdmissionReview(ctx context.Context, req *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	ctx, err := authenticate(ctx, s.authenticator)
	if err != nil {
		return nil, err
	}
	return s.AdmissionServiceServer.AdmissionReview(ctx, req)
}

type authenticatedConversionServer struct {
	pluginv3.ConversionServiceServer
	authenticator authn.Authenticator
}

func (s *authenticatedConversionServer) ConvertObjects(ctx context.Context, req *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	ctx, err := authenticate(ctx, s.authenticator)
	if err != nil {
		return nil, err
	}
	return s.ConversionServiceServer.ConvertObjects(ctx, req)
}

type authenticatedRouteServer struct {
	pluginv3.RouteServiceServer
	authenticator authn.Authenticator
}

func (s *authenticatedRouteServer) CallRoute(req *pluginv3.CallRouteRequest, stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	ctx, err := authenticate(stream.Context(), s.authenticator)
	if err != nil {
		return err
	}
	return s.RouteServiceServer.CallRoute(req, &authenticatedRouteStream{stream, ctx})
}

type authenticatedRouteStream struct {
	grpc.ServerStreamingServer[pluginv3.CallRouteResponse]
	ctx context.Context
}

func (s *authenticatedRouteStream) Context() context.Context {
	return s.ctx
}

// authenticatedAuthInfo preserves the verified token for onward delegation.
type authenticatedAuthInfo struct {
	types.AuthInfo
	accessToken string
}

func (a *authenticatedAuthInfo) GetAccessToken() string {
	return a.accessToken
}
