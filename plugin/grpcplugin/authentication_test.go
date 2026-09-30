package grpcplugin

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

func TestServerAuthentication(t *testing.T) {
	info := authn.NewAccessTokenAuthInfo(authn.Claims[authn.AccessTokenClaims]{})
	handlerErr := status.Error(codes.FailedPrecondition, "handler error")
	for _, service := range []string{pluginKeyAdmission, pluginKeyConversion, pluginKeyRouter} {
		t.Run(service, func(t *testing.T) {
			for _, tt := range []struct {
				name       string
				token      string
				authErr    error
				handlerErr error
				disabled   bool
				fallback   bool
				want       codes.Code
			}{
				{name: "authenticated", token: "valid"},
				{name: "missing token", want: codes.Unauthenticated},
				{name: "invalid token", token: "invalid", authErr: authn.ErrInvalidAudience, want: codes.Unauthenticated},
				{name: "authenticator failure", token: "valid", authErr: errors.New("unavailable"), want: codes.Internal},
				{name: "handler error", token: "valid", handlerErr: handlerErr, want: codes.FailedPrecondition},
				{name: "disabled", disabled: true},
				{name: "fallback rejects unauthenticated", fallback: true, want: codes.Unauthenticated},
				{name: "authenticated fallback", token: "valid", fallback: true, want: codes.Unimplemented},
			} {
				t.Run(tt.name, func(t *testing.T) {
					ctx := context.Background()
					if tt.token != "" {
						ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("x-access-token", tt.token))
					}
					called := false
					server := &authenticationTestServer{check: func(got context.Context) error {
						called = true
						gotInfo, ok := types.AuthInfoFrom(got)
						require.Equal(t, !tt.disabled, ok)
						if !tt.disabled {
							require.Same(t, info, gotInfo.(*authenticatedAuthInfo).AuthInfo)
							require.Equal(t, tt.token, gotInfo.GetAccessToken())
						}
						return tt.handlerErr
					}}
					opts := ServeOpts{AdmissionServer: server, ConversionServer: server, RouteServer: server}
					if !tt.disabled {
						opts.Authenticator = authenticatorFunc(func(ctx context.Context, provider authn.TokenProvider) (types.AuthInfo, error) {
							token, ok := provider.AccessToken(ctx)
							if !ok {
								return nil, authn.ErrMissingRequiredToken
							}
							require.Equal(t, tt.token, token)
							return info, tt.authErr
						})
					}
					if tt.fallback {
						switch service {
						case pluginKeyAdmission:
							opts.AdmissionServer = nil
						case pluginKeyConversion:
							opts.ConversionServer = nil
						case pluginKeyRouter:
							opts.RouteServer = nil
						}
					}
					plugins := opts.PluginSet()
					var err error
					switch service {
					case pluginKeyAdmission:
						_, err = plugins[service].(*admissionGRPCPlugin).server.AdmissionReview(ctx, &pluginv3.AdmissionReviewRequest{})
					case pluginKeyConversion:
						_, err = plugins[service].(*conversionGRPCPlugin).server.ConvertObjects(ctx, &pluginv3.ConvertObjectsRequest{})
					case pluginKeyRouter:
						stream := &authenticationTestStream{ctx: ctx}
						err = plugins[service].(*routeGRPCPlugin).server.CallRoute(&pluginv3.CallRouteRequest{}, stream)
						require.Equal(t, called, stream.sent)
					}
					require.Equal(t, tt.want, status.Code(err))
					require.Equal(t, !tt.fallback && (tt.disabled || (tt.token != "" && tt.authErr == nil)), called)
				})
			}
		})
	}
}

type authenticatorFunc func(context.Context, authn.TokenProvider) (types.AuthInfo, error)

func (f authenticatorFunc) Authenticate(ctx context.Context, provider authn.TokenProvider) (types.AuthInfo, error) {
	return f(ctx, provider)
}

type authenticationTestServer struct {
	UnimplementedV3Server
	check func(context.Context) error
}

func (s *authenticationTestServer) AdmissionReview(ctx context.Context, _ *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	return &pluginv3.AdmissionReviewResponse{}, s.check(ctx)
}
func (s *authenticationTestServer) ConvertObjects(ctx context.Context, _ *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	return &pluginv3.ConvertObjectsResponse{}, s.check(ctx)
}
func (s *authenticationTestServer) CallRoute(_ *pluginv3.CallRouteRequest, stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	if err := stream.Send(&pluginv3.CallRouteResponse{}); err != nil {
		return err
	}
	return s.check(stream.Context())
}

type authenticationTestStream struct {
	grpc.ServerStreamingServer[pluginv3.CallRouteResponse]
	ctx  context.Context
	sent bool
}

func (s *authenticationTestStream) Context() context.Context               { return s.ctx }
func (s *authenticationTestStream) Send(*pluginv3.CallRouteResponse) error { s.sent = true; return nil }

func TestAuthenticateRejectsMissingIdentity(t *testing.T) {
	ctx, err := authenticate(context.Background(), authenticatorFunc(func(context.Context, authn.TokenProvider) (types.AuthInfo, error) {
		return nil, nil
	}))
	require.Nil(t, ctx)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}
