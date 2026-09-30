package grpcplugin

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	plugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

const delegationGroup = "example.grafana.app"

// delegationObject is a minimal admission or conversion object in the caller's namespace.
var delegationObject = []byte(`{"metadata":{"namespace":"stacks-1"}}`)

// Exercise the actual registration, protobuf transport, verification and stream
// context wrapping. The exchanger models the auth service's signed response;
// it also checks the subject token supplied by the SDK on every hop.
func TestDelegationOverGRPC(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keys := testSigningKey{key: &jose.JSONWebKey{Key: &key.PublicKey, KeyID: "test"}}
	atVerifier := authn.NewAccessTokenVerifier(authn.VerifierConfig{AllowedAudiences: []string{delegationGroup}}, keys)
	idVerifier := authn.NewIDTokenVerifier(authn.VerifierConfig{}, keys)
	authenticator := authn.NewDefaultAuthenticator(atVerifier, idVerifier)
	idClaims := authn.IDTokenClaims{Identifier: "42", Type: types.TypeUser, Namespace: "stacks-1", Username: "alice"}
	idToken := signDelegationToken(t, key, authn.TokenTypeID, "user:42", delegationGroup, idClaims)
	parsedID, err := idVerifier.Verify(context.Background(), idToken)
	require.NoError(t, err)
	caller := authn.NewIDTokenAuthInfo(authn.Claims[authn.AccessTokenClaims]{}, parsedID)

	for _, method := range []string{pluginKeyAdmission, pluginKeyConversion, pluginKeyRouter} {
		t.Run(method, func(t *testing.T) {
			received := make(chan context.Context, 2)
			server := &authenticationTestServer{check: func(ctx context.Context) error { received <- ctx; return nil }}
			protocol := delegationProtocol(t, ServeOpts{AdmissionServer: server, ConversionServer: server, RouteServer: server, Authenticator: authenticator})
			actor := &authn.ActorClaims{Subject: "user:42", IDTokenClaims: idClaims}
			delegated := []string{"dashboards:read"}
			claims := authn.AccessTokenClaims{Namespace: "stacks-1", Actor: actor, Permissions: []string{"service:write"}, DelegatedPermissions: delegated}
			firstToken := signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:grafana", delegationGroup, claims)
			// A second hop preserves the original user under the previous service actor.
			claims.Actor = &authn.ActorClaims{Subject: "access-policy:grafana", Actor: actor}
			secondToken := signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:plugin", delegationGroup, claims)
			calls := 0
			client, err := NewClientV3(protocol, ClientV3Options{Groups: []string{delegationGroup}, TokenExchanger: tokenExchangerFunc(func(_ context.Context, req authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
				require.Equal(t, "stacks-1", req.Namespace)
				require.Equal(t, []string{delegationGroup}, req.Audiences)
				require.Nil(t, req.Subject)
				calls++
				if calls == 1 {
					require.Equal(t, idToken, req.SubjectToken)
					return &authn.TokenExchangeResponse{Token: firstToken}, nil
				}
				require.Equal(t, firstToken, req.SubjectToken)
				return &authn.TokenExchangeResponse{Token: secondToken}, nil
			})})
			require.NoError(t, err)
			original := metadata.Pairs("x-access-token", "stale-access", "x-id-token", "stale-identity", "trace-id", "trace")
			ctx, cancel := context.WithTimeout(types.WithAuthInfo(metadata.NewOutgoingContext(context.Background(), original), caller), 5*time.Second)
			defer cancel()
			require.NoError(t, callDelegationMethod(ctx, client, method))
			firstContext := <-received
			info, ok := types.AuthInfoFrom(firstContext)
			require.True(t, ok)
			require.Equal(t, "user:42", info.GetSubject())
			require.Equal(t, "alice", info.GetUsername())
			require.Equal(t, "stacks-1", info.GetNamespace())
			require.Equal(t, delegated, info.GetTokenDelegatedPermissions())
			require.Empty(t, info.GetTokenPermissions(), "user delegation must not expose the service's permissions")
			require.Equal(t, firstToken, info.GetAccessToken())
			md, _ := metadata.FromIncomingContext(firstContext)
			require.Equal(t, []string{"trace"}, md.Get("trace-id"))
			require.Empty(t, md.Get("x-id-token"))
			require.Equal(t, []string{"stale-access"}, original.Get("x-access-token"))
			require.Equal(t, []string{"stale-identity"}, original.Get("x-id-token"))

			// Use the verified identity on a fresh context because the first RPC has ended.
			require.NoError(t, callDelegationMethod(types.WithAuthInfo(ctx, info), client, method))
			secondInfo, ok := types.AuthInfoFrom(<-received)
			require.True(t, ok)
			require.Equal(t, "user:42", secondInfo.GetSubject())
			require.Equal(t, delegated, secondInfo.GetTokenDelegatedPermissions())
			require.Equal(t, secondToken, secondInfo.GetAccessToken())
			require.Equal(t, 2, calls)
		})
	}

	t.Run("invalid exchanged credentials never reach handlers", func(t *testing.T) {
		otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		notYetValid := map[string]any{"namespace": "stacks-1", "nbf": time.Now().Add(time.Hour).Unix()}
		issuedInFuture := map[string]any{"namespace": "stacks-1", "iat": time.Now().Add(time.Hour).Unix()}
		for _, method := range []string{pluginKeyAdmission, pluginKeyConversion, pluginKeyRouter} {
			for _, tt := range []struct{ name, token string }{
				{"wrong audience", signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:grafana", "other.app", authn.AccessTokenClaims{Namespace: "stacks-1"})},
				{"malformed", "not-a-token"},
				{"wrong signing key", signDelegationToken(t, otherKey, authn.TokenTypeAccess, "access-policy:grafana", delegationGroup, authn.AccessTokenClaims{Namespace: "stacks-1"})},
				{"not yet valid", signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:grafana", delegationGroup, notYetValid)},
				{"issued in the future", signDelegationToken(t, key, authn.TokenTypeAccess, "access-policy:grafana", delegationGroup, issuedInFuture)},
			} {
				t.Run(method+"/"+tt.name, func(t *testing.T) {
					received := make(chan struct{}, 1)
					server := &authenticationTestServer{check: func(context.Context) error { received <- struct{}{}; return nil }}
					protocol := delegationProtocol(t, ServeOpts{AdmissionServer: server, ConversionServer: server, RouteServer: server, Authenticator: authenticator})
					client, err := NewClientV3(protocol, ClientV3Options{Groups: []string{delegationGroup}, TokenExchanger: authn.NewStaticTokenExchanger(tt.token)})
					require.NoError(t, err)
					ctx, cancel := context.WithTimeout(types.WithAuthInfo(context.Background(), caller), 5*time.Second)
					defer cancel()
					require.Equal(t, codes.Unauthenticated, status.Code(callDelegationMethod(ctx, client, method)))
					require.Empty(t, received)
				})
			}
		}
	})
}

func TestDelegationFailsBeforeRPC(t *testing.T) {
	exchangeErr := errors.New("exchange unavailable")
	caller := &testCallerInfo{AuthInfo: authn.NewAccessTokenAuthInfo(authn.Claims[authn.AccessTokenClaims]{Rest: authn.AccessTokenClaims{Namespace: "stacks-1"}}), access: "access", id: "id"}
	for _, method := range []string{pluginKeyAdmission, pluginKeyConversion, pluginKeyRouter} {
		for _, tt := range []struct {
			name      string
			info      types.AuthInfo
			response  *authn.TokenExchangeResponse
			err       error
			want      string
			exchanged bool
		}{
			{name: "missing caller", want: "caller auth info"},
			{name: "no signed token", info: caller.AuthInfo, want: "caller access or ID token"},
			{name: "missing namespace", info: &testCallerInfo{AuthInfo: authn.NewAccessTokenAuthInfo(authn.Claims[authn.AccessTokenClaims]{}), access: "access"}, want: "caller namespace"},
			{name: "exchange failure", info: caller, err: exchangeErr, want: "exchange unavailable", exchanged: true},
			{name: "nil response", info: caller, want: "empty access token", exchanged: true},
			{name: "empty token", info: caller, response: &authn.TokenExchangeResponse{}, want: "empty access token", exchanged: true},
		} {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				exchanged := false
				// Nil RPC clients panic if failure accidentally falls through to an RPC.
				client := &clientV3{groups: []string{delegationGroup}, tokenExchange: tokenExchangerFunc(func(_ context.Context, req authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
					exchanged = true
					require.Equal(t, "access", req.SubjectToken, "access token takes precedence over ID token")
					return tt.response, tt.err
				})}
				ctx := context.Background()
				if tt.info != nil {
					ctx = types.WithAuthInfo(ctx, tt.info)
				}
				err := callDelegationMethod(ctx, client, method)
				require.ErrorContains(t, err, tt.want)
				if tt.err != nil {
					require.ErrorIs(t, err, tt.err)
				}
				require.Equal(t, tt.exchanged, exchanged)
			})
		}
	}
}

func TestConversionAudienceValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		groups []string
	}{
		{name: "empty batch"}, {name: "missing group", groups: []string{""}}, {name: "mixed groups", groups: []string{delegationGroup, "other.app"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &clientV3{groups: []string{delegationGroup}, tokenExchange: tokenExchangerFunc(func(context.Context, authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
				t.Fatal("invalid conversion must not exchange tokens")
				return nil, nil
			})}
			req := &pluginv3.ConvertObjectsRequest{}
			for _, group := range tt.groups {
				req.SetObjects(append(req.GetObjects(), pluginv3.ConvertObjectsRequest_Object_builder{Gvk: pluginv3.GroupVersionKind_builder{Group: new(group)}.Build()}.Build()))
			}
			_, err := client.ConvertObjects(context.Background(), req)
			require.ErrorContains(t, err, "API group")
		})
	}
}

func TestDelegationDisabledPreservesContext(t *testing.T) {
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-access-token", "existing"))
	got, err := (&clientV3{}).addMetadataToContext(ctx, "", "")
	require.NoError(t, err)
	require.Same(t, ctx, got)
}

func callDelegationMethod(ctx context.Context, client ClientV3, method string) error {
	switch method {
	case pluginKeyAdmission:
		_, err := client.AdmissionReview(ctx, pluginv3.AdmissionReviewRequest_builder{Kind: pluginv3.GroupVersionKind_builder{Group: new(delegationGroup)}.Build(), ObjectBytes: delegationObject}.Build())
		return err
	case pluginKeyConversion:
		_, err := client.ConvertObjects(ctx, pluginv3.ConvertObjectsRequest_builder{
			Api:     pluginv3.GroupVersion_builder{Group: new("apiextensions.k8s.io"), Version: new("v1")}.Build(),
			Objects: []*pluginv3.ConvertObjectsRequest_Object{pluginv3.ConvertObjectsRequest_Object_builder{Gvk: pluginv3.GroupVersionKind_builder{Group: new(delegationGroup)}.Build(), Raw: delegationObject}.Build()},
		}.Build())
		return err
	default:
		stream, err := client.CallRoute(ctx, pluginv3.CallRouteRequest_builder{Group: new(delegationGroup), Namespace: new("stacks-1")}.Build())
		if err != nil {
			return err
		}
		for {
			_, err = stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
		}
	}
}

func delegationProtocol(t *testing.T, opts ServeOpts) *testClientProtocol {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	for _, p := range opts.PluginSet() {
		require.NoError(t, p.(plugin.GRPCPlugin).GRPCServer(nil, server))
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return &testClientProtocol{plugins: map[string]any{
		pluginKeyAdmission: pluginv3.NewAdmissionServiceClient(conn), pluginKeyConversion: pluginv3.NewConversionServiceClient(conn), pluginKeyRouter: pluginv3.NewRouteServiceClient(conn),
	}}
}

type tokenExchangerFunc func(context.Context, authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error)

func (f tokenExchangerFunc) Exchange(ctx context.Context, req authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
	return f(ctx, req)
}

type testCallerInfo struct {
	types.AuthInfo
	access, id string
}

func (i *testCallerInfo) GetAccessToken() string { return i.access }
func (i *testCallerInfo) GetIDToken() string     { return i.id }

type testSigningKey struct{ key *jose.JSONWebKey }

func (k testSigningKey) Get(context.Context, string) (*jose.JSONWebKey, error) { return k.key, nil }

func signDelegationToken(t *testing.T, key *ecdsa.PrivateKey, typ, subject, audience string, claims any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, (&jose.SignerOptions{}).WithType(jose.ContentType(typ)).WithHeader("kid", "test"))
	require.NoError(t, err)
	token, err := jwt.Signed(signer).Claims(jwt.Claims{Subject: subject, Audience: jwt.Audience{audience}, Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour))}).Claims(claims).Serialize()
	require.NoError(t, err)
	return token
}

func TestDelegationRequiresResourceGroup(t *testing.T) {
	client := &clientV3{groups: []string{delegationGroup}, tokenExchange: tokenExchangerFunc(func(context.Context, authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
		t.Fatal("missing audience must not exchange tokens")
		return nil, nil
	})}
	_, err := client.AdmissionReview(context.Background(), &pluginv3.AdmissionReviewRequest{})
	require.ErrorContains(t, err, "API group is required")
	_, err = client.CallRoute(context.Background(), &pluginv3.CallRouteRequest{})
	require.ErrorContains(t, err, "API group is required")
}
