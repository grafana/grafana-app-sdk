package grpcplugin

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

func TestNewClientV3RequiresGroupsWithTokenExchange(t *testing.T) {
	exchanger := authn.NewStaticTokenExchanger("token")
	for _, groups := range [][]string{nil, {delegationGroup, ""}} {
		client, err := NewClientV3(&testClientProtocol{}, ClientV3Options{TokenExchanger: exchanger, Groups: groups})
		require.Nil(t, client)
		require.ErrorContains(t, err, "API groups")
	}
}

func TestClientV3ExchangeScope(t *testing.T) {
	user := &testCallerInfo{AuthInfo: authn.NewAccessTokenAuthInfo(authn.Claims[authn.AccessTokenClaims]{Rest: authn.AccessTokenClaims{Namespace: "stacks-1"}}), id: "user-id"}
	service := &testCallerInfo{AuthInfo: authn.NewAccessTokenAuthInfo(authn.Claims[authn.AccessTokenClaims]{Rest: authn.AccessTokenClaims{Namespace: "*"}})}
	object := func(ns string) []byte { return fmt.Appendf(nil, `{"metadata":{"namespace":%q}}`, ns) }
	admission := func(objects ...[]byte) func(context.Context, *clientV3) error {
		return func(ctx context.Context, c *clientV3) error {
			req := pluginv3.AdmissionReviewRequest_builder{Kind: pluginv3.GroupVersionKind_builder{Group: new(delegationGroup)}.Build(), ObjectBytes: objects[0]}.Build()
			if len(objects) > 1 {
				req.SetOldObjectBytes(objects[1])
			}
			_, err := c.AdmissionReview(ctx, req)
			return err
		}
	}
	conversion := func(objects ...[]byte) func(context.Context, *clientV3) error {
		return func(ctx context.Context, c *clientV3) error {
			req := &pluginv3.ConvertObjectsRequest{}
			for _, raw := range objects {
				req.SetObjects(append(req.GetObjects(), pluginv3.ConvertObjectsRequest_Object_builder{Gvk: pluginv3.GroupVersionKind_builder{Group: new(delegationGroup)}.Build(), Raw: raw}.Build()))
			}
			_, err := c.ConvertObjects(ctx, req)
			return err
		}
	}
	route := func(group, namespace string) func(context.Context, *clientV3) error {
		return func(ctx context.Context, c *clientV3) error {
			_, err := c.CallRoute(ctx, pluginv3.CallRouteRequest_builder{Group: new(group), Namespace: new(namespace)}.Build())
			return err
		}
	}

	for _, tt := range []struct {
		name          string
		info          types.AuthInfo
		isService     func(context.Context) bool
		call          func(context.Context, *clientV3) error
		wantNamespace string
		wantSubject   string
		wantErr       string
	}{
		{name: "user route", info: user, call: route(delegationGroup, "stacks-1"), wantNamespace: "stacks-1", wantSubject: "user-id"},
		{name: "user cluster route uses caller namespace", info: user, call: route(delegationGroup, ""), wantNamespace: "stacks-1", wantSubject: "user-id"},
		{name: "user route in other tenant", info: user, call: route(delegationGroup, "stacks-2"), wantErr: "does not cover the requested namespace"},
		{name: "group not served by plugin", info: user, call: route("other.app", "stacks-1"), wantErr: `API group "other.app" is not served`},
		{name: "user admission in other tenant", info: user, call: admission(object("stacks-2")), wantErr: "does not cover the requested namespace"},
		{name: "malformed admission object", info: user, call: admission([]byte("not json")), wantErr: "decode object metadata"},
		{name: "service route narrowed to request", info: service, isService: func(context.Context) bool { return true }, call: route(delegationGroup, "stacks-1"), wantNamespace: "stacks-1"},
		{name: "service admission narrowed to object", info: service, isService: func(context.Context) bool { return true }, call: admission(object("stacks-3")), wantNamespace: "stacks-3"},
		{name: "service conversion across namespaces", info: service, isService: func(context.Context) bool { return true }, call: conversion(object("stacks-1"), object("stacks-2")), wantNamespace: "*"},
		{name: "service conversion in one namespace", info: service, isService: func(context.Context) bool { return true }, call: conversion(object("stacks-1"), object("stacks-1")), wantNamespace: "stacks-1"},
		{name: "unrecognized tokenless service", info: service, isService: func(context.Context) bool { return false }, call: route(delegationGroup, "stacks-1"), wantErr: "caller access or ID token is required"},
		{name: "tokenless service without hook", info: service, call: route(delegationGroup, "stacks-1"), wantErr: "caller access or ID token is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			exchanged := false
			// Nil RPC clients stop each call after the exchange.
			client := &clientV3{groups: []string{delegationGroup}, isServiceIdentity: tt.isService, tokenExchange: tokenExchangerFunc(func(_ context.Context, req authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
				exchanged = true
				require.Equal(t, tt.wantNamespace, req.Namespace)
				require.Equal(t, tt.wantSubject, req.SubjectToken)
				require.Equal(t, []string{delegationGroup}, req.Audiences)
				return nil, errStopAfterExchange
			})}
			err := tt.call(types.WithAuthInfo(context.Background(), tt.info), client)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.False(t, exchanged)
				return
			}
			require.ErrorIs(t, err, errStopAfterExchange)
			require.True(t, exchanged)
		})
	}
}

var errStopAfterExchange = fmt.Errorf("stop after exchange")

func TestServerObjectNamespaces(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keys := testSigningKey{key: &jose.JSONWebKey{Key: &key.PublicKey, KeyID: "test"}}
	authenticator := authn.NewAccessTokenAuthenticator(authn.NewAccessTokenVerifier(authn.VerifierConfig{AllowedAudiences: []string{delegationGroup}}, keys))
	object := func(ns string) []byte { return fmt.Appendf(nil, `{"metadata":{"namespace":%q}}`, ns) }
	for _, tt := range []struct {
		name           string
		tokenNamespace string
		objects        [][]byte // admission: new, old
		want           codes.Code
	}{
		{"same tenant", "stacks-1", [][]byte{object("stacks-1")}, codes.OK},
		{"delete carries only the old object", "stacks-1", [][]byte{nil, object("stacks-1")}, codes.OK},
		{"other tenant", "stacks-1", [][]byte{object("stacks-2")}, codes.PermissionDenied},
		{"old object in other tenant", "stacks-1", [][]byte{object("stacks-1"), object("stacks-2")}, codes.PermissionDenied},
		{"cluster-scoped object from tenant", "stacks-1", [][]byte{object("")}, codes.PermissionDenied},
		{"cluster-scoped object with wildcard", "*", [][]byte{object("")}, codes.OK},
		{"no objects", "stacks-1", [][]byte{nil}, codes.InvalidArgument},
		{"malformed object", "stacks-1", [][]byte{[]byte("not json")}, codes.InvalidArgument},
	} {
		for _, method := range []string{pluginKeyAdmission, pluginKeyConversion} {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				received := make(chan struct{}, 1)
				server := &authenticationTestServer{check: func(context.Context) error { received <- struct{}{}; return nil }}
				protocol := delegationProtocol(t, ServeOpts{AdmissionServer: server, ConversionServer: server, Authenticator: authenticator})
				token := signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:plugin", delegationGroup, authn.AccessTokenClaims{Namespace: tt.tokenNamespace})
				ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-access-token", token)), 5*time.Second)
				defer cancel()
				kind := pluginv3.GroupVersionKind_builder{Group: new(delegationGroup)}.Build()
				if method == pluginKeyAdmission {
					req := pluginv3.AdmissionReviewRequest_builder{Kind: kind, ObjectBytes: tt.objects[0]}.Build()
					if len(tt.objects) > 1 {
						req.SetOldObjectBytes(tt.objects[1])
					}
					_, err = protocol.plugins[pluginKeyAdmission].(pluginv3.AdmissionServiceClient).AdmissionReview(ctx, req)
				} else {
					req := &pluginv3.ConvertObjectsRequest{}
					for _, raw := range tt.objects {
						req.SetObjects(append(req.GetObjects(), pluginv3.ConvertObjectsRequest_Object_builder{Gvk: kind, Raw: raw}.Build()))
					}
					_, err = protocol.plugins[pluginKeyConversion].(pluginv3.ConversionServiceClient).ConvertObjects(ctx, req)
				}
				require.Equal(t, tt.want, status.Code(err))
				if tt.want != codes.OK {
					require.Empty(t, received, "rejected requests must not invoke handlers")
				}
			})
		}
	}
}

func TestAuthenticatedAuthInfoRedactsAccessToken(t *testing.T) {
	const token = "secret-access-token"
	info := &authenticatedAuthInfo{
		AuthInfo:    authn.NewAccessTokenAuthInfo(authn.Claims[authn.AccessTokenClaims]{Rest: authn.AccessTokenClaims{Namespace: "stacks-1"}}),
		accessToken: token,
	}
	var logged bytes.Buffer
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("caller", "info", info)
	for _, out := range []string{fmt.Sprint(info), fmt.Sprintf("%+v", info), fmt.Sprintf("%#v", info), logged.String()} {
		require.NotContains(t, out, token)
		require.Contains(t, out, "stacks-1")
	}
	require.Equal(t, token, info.GetAccessToken())
}

func TestServerPluginIDAudience(t *testing.T) {
	const pluginID = "example-app"
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keys := testSigningKey{key: &jose.JSONWebKey{Key: &key.PublicKey, KeyID: "test", Algorithm: string(jose.ES256)}}
	authenticator := authn.NewAccessTokenAuthenticator(authn.NewAccessTokenVerifier(authn.VerifierConfig{AllowedAudiences: []string{pluginID, "other-app", delegationGroup, "second.grafana.app"}}, keys))
	for _, tt := range []struct {
		name, pluginID, audience, group, namespace string
		want                                       codes.Code
	}{
		{name: "plugin ID covers the group", pluginID: pluginID, audience: pluginID, group: delegationGroup, namespace: "stacks-1", want: codes.OK},
		{name: "plugin ID covers another served group", pluginID: pluginID, audience: pluginID, group: "second.grafana.app", namespace: "stacks-1", want: codes.OK},
		{name: "plugin ID still checks namespace", pluginID: pluginID, audience: pluginID, group: delegationGroup, namespace: "stacks-2", want: codes.PermissionDenied},
		{name: "another plugin's ID", pluginID: pluginID, audience: "other-app", group: delegationGroup, namespace: "stacks-1", want: codes.PermissionDenied},
		{name: "plugin ID not configured", audience: pluginID, group: delegationGroup, namespace: "stacks-1", want: codes.PermissionDenied},
		{name: "group token for another group", pluginID: pluginID, audience: delegationGroup, group: "second.grafana.app", namespace: "stacks-1", want: codes.PermissionDenied},
		{name: "group token", pluginID: pluginID, audience: delegationGroup, group: delegationGroup, namespace: "stacks-1", want: codes.OK},
	} {
		for _, method := range []string{pluginKeyAdmission, pluginKeyConversion, pluginKeyRouter} {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				received := make(chan struct{}, 1)
				server := &authenticationTestServer{check: func(context.Context) error { received <- struct{}{}; return nil }}
				protocol := delegationProtocol(t, ServeOpts{AdmissionServer: server, ConversionServer: server, RouteServer: server, Authenticator: authenticator, PluginID: tt.pluginID})
				token := signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:grafana", tt.audience, authn.AccessTokenClaims{Namespace: "stacks-1"})
				ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-access-token", token)), 5*time.Second)
				defer cancel()
				kind := pluginv3.GroupVersionKind_builder{Group: new(tt.group)}.Build()
				object := fmt.Appendf(nil, `{"metadata":{"namespace":%q}}`, tt.namespace)
				switch method {
				case pluginKeyAdmission:
					_, err = protocol.plugins[method].(pluginv3.AdmissionServiceClient).AdmissionReview(ctx, pluginv3.AdmissionReviewRequest_builder{Kind: kind, ObjectBytes: object}.Build())
				case pluginKeyConversion:
					_, err = protocol.plugins[method].(pluginv3.ConversionServiceClient).ConvertObjects(ctx, pluginv3.ConvertObjectsRequest_builder{
						Objects: []*pluginv3.ConvertObjectsRequest_Object{pluginv3.ConvertObjectsRequest_Object_builder{Gvk: kind, Raw: object}.Build()},
					}.Build())
				default:
					var stream grpc.ServerStreamingClient[pluginv3.CallRouteResponse]
					stream, err = protocol.plugins[method].(pluginv3.RouteServiceClient).CallRoute(ctx, pluginv3.CallRouteRequest_builder{Group: new(tt.group), Namespace: new(tt.namespace)}.Build())
					require.NoError(t, err)
					_, err = stream.Recv()
				}
				require.Equal(t, tt.want, status.Code(err), "%v", err)
				if tt.want != codes.OK {
					require.Empty(t, received, "rejected requests must not invoke handlers")
				}
			})
		}
	}
}
