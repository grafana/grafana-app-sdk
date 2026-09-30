package grpcplugin

import (
	"context"
	"slices"

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
	// Delegation has one source of identity: the exchanged access token.
	// Reject ambiguous credentials before invoking even a custom authenticator.
	tokens := md.Get("x-access-token")
	if len(tokens) != 1 || tokens[0] == "" || len(md.Get("x-id-token")) != 0 {
		return nil, status.Error(codes.Unauthenticated, "a single access token and no separate ID token are required")
	}
	info, err := authenticator.Authenticate(ctx, authn.NewGRPCTokenProvider(md))
	if err != nil {
		if authn.IsUnauthenticatedErr(err) {
			return nil, status.Error(codes.Unauthenticated, "invalid access token")
		}
		return nil, status.Error(codes.Internal, "authentication failed")
	}
	if info == nil {
		return nil, status.Error(codes.Unauthenticated, "authenticator returned no identity")
	}
	// authlib's AuthInfo does not retain the raw access token. Keep the token
	// only after successful authentication so downstream clients can exchange
	// it without losing the caller's identity or delegation chain.
	return types.WithAuthInfo(ctx, &authenticatedAuthInfo{AuthInfo: info, accessToken: tokens[0]}), nil
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
	if err := checkAudience(ctx, req.GetKind().GetGroup()); err != nil {
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
	group, err := conversionGroup(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := checkAudience(ctx, group); err != nil {
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
	if err := checkAudience(ctx, req.GetGroup()); err != nil {
		return err
	}
	info, _ := types.AuthInfoFrom(ctx)
	if !types.NamespaceMatches(info.GetNamespace(), req.GetNamespace()) {
		return status.Error(codes.PermissionDenied, "access token does not cover the requested namespace")
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

// The verifier checks the service's configured audiences. Also bind the token
// to this particular request, as a plugin may serve more than one API group.
func checkAudience(ctx context.Context, group string) error {
	if group == "" {
		return status.Error(codes.InvalidArgument, "API group is required")
	}
	info, _ := types.AuthInfoFrom(ctx)
	if !slices.Contains(info.GetAudience(), group) {
		return status.Error(codes.PermissionDenied, "access token does not cover the requested API group")
	}
	return nil
}
