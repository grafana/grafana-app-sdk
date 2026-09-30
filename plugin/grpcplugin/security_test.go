package grpcplugin

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestServerRejectsCredentialConfusion(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keys := testSigningKey{key: &jose.JSONWebKey{Key: &key.PublicKey, KeyID: "test"}}
	authenticator := authn.NewDefaultAuthenticator(
		authn.NewAccessTokenVerifier(authn.VerifierConfig{AllowedAudiences: []string{delegationGroup, "other.app"}}, keys),
		authn.NewIDTokenVerifier(authn.VerifierConfig{}, keys),
	)
	claims := authn.AccessTokenClaims{Namespace: "stacks-1", Actor: &authn.ActorClaims{
		Subject: "user:alice", IDTokenClaims: authn.IDTokenClaims{Type: types.TypeUser, Identifier: "alice"},
	}}
	token := signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:plugin", delegationGroup, claims)
	wrongAudience := signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:plugin", "other.app", claims)
	otherIdentity := signDelegationToken(t, key, authn.TokenTypeID, "user:bob", delegationGroup,
		authn.IDTokenClaims{Type: types.TypeUser, Identifier: "bob", Namespace: "stacks-1"})
	for _, method := range []string{pluginKeyAdmission, pluginKeyConversion, pluginKeyRouter} {
		for _, tt := range []struct {
			name string
			md   metadata.MD
			want codes.Code
		}{
			{"different request audience", metadata.Pairs("x-access-token", wrongAudience), codes.PermissionDenied},
			{"separate identity override", metadata.Pairs("x-access-token", token, "x-id-token", otherIdentity), codes.Unauthenticated},
			{"duplicate access tokens", metadata.Pairs("x-access-token", token, "x-access-token", wrongAudience), codes.Unauthenticated},
		} {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				called := make(chan struct{}, 1)
				server := &authenticationTestServer{check: func(context.Context) error { called <- struct{}{}; return nil }}
				protocol := delegationProtocol(t, ServeOpts{AdmissionServer: server, ConversionServer: server, RouteServer: server, Authenticator: authenticator})
				client, err := NewClientV3(protocol, ClientV3Options{})
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), tt.md), 5*time.Second)
				defer cancel()
				require.Equal(t, tt.want, status.Code(callDelegationMethod(ctx, client, method)))
				require.Empty(t, called, "rejected requests must not invoke handlers")
			})
		}
	}
}

func TestAuthenticationDoesNotExposeInternalErrors(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-access-token", "valid"))
	_, _, err := serverAuth{authenticator: authenticatorFunc(func(context.Context, authn.TokenProvider) (types.AuthInfo, error) {
		return nil, errors.New("fetching https://internal-signer?credential=secret failed")
	})}.authenticate(ctx)
	require.Equal(t, codes.Internal, status.Code(err))
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), "internal-signer")
}

func TestServerRequestScope(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keys := testSigningKey{key: &jose.JSONWebKey{Key: &key.PublicKey, KeyID: "test"}}
	authenticator := authn.NewAccessTokenAuthenticator(authn.NewAccessTokenVerifier(authn.VerifierConfig{AllowedAudiences: []string{delegationGroup}}, keys))
	for _, tt := range []struct {
		name, tokenNamespace, requestNamespace string
		want                                   codes.Code
	}{
		{"same tenant", "stacks-1", "stacks-1", codes.OK},
		{"different tenant", "stacks-1", "stacks-2", codes.PermissionDenied},
		{"cluster request from tenant", "stacks-1", "", codes.PermissionDenied},
		{"wildcard request from tenant", "stacks-1", "*", codes.PermissionDenied},
		{"wildcard token for tenant", "*", "stacks-1", codes.OK},
		{"wildcard token for cluster", "*", "", codes.OK},
		{"missing token namespace", "", "", codes.PermissionDenied},
	} {
		t.Run(tt.name, func(t *testing.T) {
			received := make(chan struct{}, 1)
			server := &authenticationTestServer{check: func(context.Context) error { received <- struct{}{}; return nil }}
			protocol := delegationProtocol(t, ServeOpts{RouteServer: server, Authenticator: authenticator})
			client := protocol.plugins[pluginKeyRouter].(pluginv3.RouteServiceClient)
			token := signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:plugin", delegationGroup, authn.AccessTokenClaims{Namespace: tt.tokenNamespace})
			ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-access-token", token)), 5*time.Second)
			defer cancel()
			req := &pluginv3.CallRouteRequest{}
			req.SetGroup(delegationGroup)
			req.SetNamespace(tt.requestNamespace)
			stream, err := client.CallRoute(ctx, req)
			require.NoError(t, err)
			_, err = stream.Recv()
			require.Equal(t, tt.want, status.Code(err))
			if tt.want != codes.OK {
				require.Empty(t, received)
			}
		})
	}

	for _, tt := range []struct {
		name   string
		groups []string
	}{
		{name: "empty conversion"},
		{name: "missing object group", groups: []string{""}},
		{name: "mixed object groups", groups: []string{delegationGroup, "other.app"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			received := make(chan struct{}, 1)
			server := &authenticationTestServer{check: func(context.Context) error { received <- struct{}{}; return nil }}
			protocol := delegationProtocol(t, ServeOpts{ConversionServer: server, Authenticator: authenticator})
			client := protocol.plugins[pluginKeyConversion].(pluginv3.ConversionServiceClient)
			token := signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:plugin", delegationGroup, authn.AccessTokenClaims{Namespace: "stacks-1"})
			ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-access-token", token)), 5*time.Second)
			defer cancel()
			req := &pluginv3.ConvertObjectsRequest{}
			for _, group := range tt.groups {
				kind := &pluginv3.GroupVersionKind{}
				kind.SetGroup(group)
				obj := &pluginv3.ConvertObjectsRequest_Object{}
				obj.SetGvk(kind)
				req.SetObjects(append(req.GetObjects(), obj))
			}
			_, err := client.ConvertObjects(ctx, req)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.Empty(t, received)
		})
	}
}
