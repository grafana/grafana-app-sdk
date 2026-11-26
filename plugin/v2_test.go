package plugin

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/grafana-plugin-sdk-go/backend"

	"github.com/grafana/grafana-app-sdk/resource"
)

const v2TestPluginID = "my-app"

// v2TestTokens returns an authenticator that accepts tokens for v2TestPluginID,
// and a function that signs one with the given namespace and audience.
func v2TestTokens(t *testing.T) (authn.Authenticator, func(namespace, audience string) string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key},
		(&jose.SignerOptions{}).WithType("at+jwt").WithHeader("kid", "test"))
	require.NoError(t, err)
	keys := staticKeys{"test": {Key: &key.PublicKey, KeyID: "test", Algorithm: "ES256", Use: "sig"}}
	verifier := authn.NewAccessTokenVerifier(authn.VerifierConfig{AllowedAudiences: []string{v2TestPluginID}}, keys)
	sign := func(namespace, audience string) string {
		token, err := jwt.Signed(signer).Claims(jwt.Claims{
			Subject:  "user:u1",
			Audience: jwt.Audience{audience},
			Expiry:   jwt.NewNumericDate(time.Now().Add(time.Minute)),
			IssuedAt: jwt.NewNumericDate(time.Now()),
		}).Claims(map[string]any{"namespace": namespace}).Serialize()
		require.NoError(t, err)
		return token
	}
	return authn.NewAccessTokenAuthenticator(verifier), sign
}

// v2Recorder is the plugin's handler: it records the context each call reached it with.
type v2Recorder struct {
	ctx context.Context
}

func (r *v2Recorder) handler() *backend.Handlers {
	return &backend.Handlers{
		CheckHealthHandler: backend.CheckHealthHandlerFunc(func(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
			r.ctx = ctx
			return &backend.CheckHealthResult{Status: backend.HealthStatusOk}, nil
		}),
		CallResourceHandler: backend.CallResourceHandlerFunc(func(ctx context.Context, _ *backend.CallResourceRequest, _ backend.CallResourceResponseSender) error {
			r.ctx = ctx
			return nil
		}),
	}
}

func newV2TestHandler(t *testing.T, auth v2Auth) (backend.Handler, *v2Recorder) {
	t.Helper()
	rec := &v2Recorder{}
	h, err := backend.HandlerFromMiddlewares(rec.handler(), newV2Middleware(auth))
	require.NoError(t, err)
	return h, rec
}

var nopSender = backend.CallResourceResponseSenderFunc(func(*backend.CallResourceResponse) error { return nil })

func v2Resource(namespace, token string) *backend.CallResourceRequest {
	req := &backend.CallResourceRequest{
		PluginContext: backend.PluginContext{
			PluginID:  v2TestPluginID,
			Namespace: namespace,
			AppInstanceSettings: &backend.AppInstanceSettings{
				JSONData:                []byte(`{"message":"hello"}`),
				DecryptedSecureJSONData: map[string]string{"apiKey": "s3cret"},
				Updated:                 time.UnixMilli(1700000000000),
				APIVersion:              "v0alpha1",
			},
		},
		Path: "clients",
	}
	if token != "" {
		req.SetHTTPHeader(v2AccessTokenHeader, token)
	}
	return req
}

func TestV2MiddlewareAuthentication(t *testing.T) {
	authenticator, sign := v2TestTokens(t)

	t.Run("a call without a token has no identity", func(t *testing.T) {
		h, rec := newV2TestHandler(t, (&runConfig{authenticator: authenticator}).v2Auth())
		require.NoError(t, h.CallResource(context.Background(), v2Resource("default", ""), nopSender))
		_, ok := types.AuthInfoFrom(rec.ctx)
		assert.False(t, ok)
	})

	t.Run("a verified token is the caller, and is kept for delegation", func(t *testing.T) {
		h, rec := newV2TestHandler(t, (&runConfig{authenticator: authenticator}).v2Auth())
		token := sign("default", v2TestPluginID)
		require.NoError(t, h.CallResource(context.Background(), v2Resource("default", token), nopSender))
		info, ok := types.AuthInfoFrom(rec.ctx)
		require.True(t, ok)
		assert.Equal(t, "default", info.GetNamespace())
		assert.Equal(t, token, info.GetAccessToken())
	})

	t.Run("a Bearer prefix is accepted", func(t *testing.T) {
		h, rec := newV2TestHandler(t, (&runConfig{authenticator: authenticator}).v2Auth())
		require.NoError(t, h.CallResource(context.Background(), v2Resource("default", "Bearer "+sign("default", v2TestPluginID)), nopSender))
		_, ok := types.AuthInfoFrom(rec.ctx)
		assert.True(t, ok)
	})

	t.Run("health checks are authenticated too", func(t *testing.T) {
		h, rec := newV2TestHandler(t, (&runConfig{authenticator: authenticator}).v2Auth())
		req := &backend.CheckHealthRequest{PluginContext: backend.PluginContext{PluginID: v2TestPluginID, Namespace: "default"}}
		req.SetHTTPHeader(v2AccessTokenHeader, sign("default", v2TestPluginID))
		_, err := h.CheckHealth(context.Background(), req)
		require.NoError(t, err)
		_, ok := types.AuthInfoFrom(rec.ctx)
		assert.True(t, ok)

		req.SetHTTPHeader(v2AccessTokenHeader, "not-a-jwt")
		_, err = h.CheckHealth(context.Background(), req)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	for name, tt := range map[string]struct {
		auth  v2Auth
		token string
		ns    string
		code  codes.Code
	}{
		"an invalid token":                 {auth: (&runConfig{authenticator: authenticator}).v2Auth(), token: "not-a-jwt", ns: "default", code: codes.Unauthenticated},
		"a token for another audience":     {auth: (&runConfig{authenticator: authenticator}).v2Auth(), token: sign("default", "other-app"), ns: "default", code: codes.Unauthenticated},
		"a token for another namespace":    {auth: (&runConfig{authenticator: authenticator}).v2Auth(), token: sign("stacks-2", v2TestPluginID), ns: "stacks-1", code: codes.PermissionDenied},
		"a token without an authenticator": {auth: (&runConfig{insecureSkipAuthentication: false}).v2Auth(), token: sign("default", v2TestPluginID), ns: "default", code: codes.Unauthenticated},
	} {
		t.Run(name+" is rejected", func(t *testing.T) {
			h, rec := newV2TestHandler(t, tt.auth)
			err := h.CallResource(context.Background(), v2Resource(tt.ns, tt.token), nopSender)
			assert.Equal(t, tt.code, status.Code(err))
			assert.Nil(t, rec.ctx, "the handler must not run")
		})
	}

	t.Run("insecure skip authentication parses an unverified token", func(t *testing.T) {
		h, rec := newV2TestHandler(t, (&runConfig{insecureSkipAuthentication: true}).v2Auth())
		require.NoError(t, h.CallResource(context.Background(), v2Resource("stacks-1", sign("stacks-2", "other-app")), nopSender))
		info, ok := types.AuthInfoFrom(rec.ctx)
		require.True(t, ok)
		assert.Equal(t, "stacks-2", info.GetNamespace())
	})
}

func TestV2MiddlewareRouteRequestInfo(t *testing.T) {
	h, rec := newV2TestHandler(t, (&runConfig{insecureSkipAuthentication: false}).v2Auth())

	t.Run("resource calls get the app instance settings", func(t *testing.T) {
		require.NoError(t, h.CallResource(context.Background(), v2Resource("default", ""), nopSender))
		info := resource.RouteRequestInfoFrom(rec.ctx)
		require.NotNil(t, info)
		assert.Equal(t, resource.FullIdentifier{
			Namespace: "default", Name: "instance", Group: v2TestPluginID, Version: "v0alpha1", Kind: "Settings", Plural: "app",
		}, info.FullIdentifier)
		assert.Equal(t, "1700000000000", info.ResourceVersion)
		assert.JSONEq(t, `{
			"apiVersion": "my-app/v0alpha1",
			"kind": "Settings",
			"metadata": {"name": "instance", "namespace": "default"},
			"spec": {"jsonData": {"message": "hello"}}
		}`, string(info.Parent))
		assert.Equal(t, resource.DecryptedSecureValues{"apiKey": "s3cret"}, info.DecryptedSecureValues)

		// The secure values never appear when the info is serialized.
		raw, err := json.Marshal(info)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "s3cret")
	})

	t.Run("the plugin context's API version wins", func(t *testing.T) {
		req := v2Resource("default", "")
		req.PluginContext.APIVersion = "v1"
		require.NoError(t, h.CallResource(context.Background(), req, nopSender))
		assert.Equal(t, "v1", resource.RouteRequestInfoFrom(rec.ctx).Version)
	})

	t.Run("calls without app instance settings get none", func(t *testing.T) {
		req := v2Resource("default", "")
		req.PluginContext.AppInstanceSettings = nil
		require.NoError(t, h.CallResource(context.Background(), req, nopSender))
		assert.Nil(t, resource.RouteRequestInfoFrom(rec.ctx))
	})

	t.Run("route info already in the context is kept", func(t *testing.T) {
		existing := &resource.RouteRequestInfo{FullIdentifier: resource.FullIdentifier{Group: "other"}}
		require.NoError(t, h.CallResource(resource.WithRouteRequestInfo(context.Background(), existing), v2Resource("default", ""), nopSender))
		assert.Same(t, existing, resource.RouteRequestInfoFrom(rec.ctx))
	})
}

func TestWithV2Middleware(t *testing.T) {
	mw := newV2Middleware((&runConfig{insecureSkipAuthentication: false}).v2Auth())

	t.Run("adds the middleware", func(t *testing.T) {
		opts, err := withV2Middleware(nil, mw)("my-app", backend.ServeOpts{})
		require.NoError(t, err)
		assert.Len(t, opts.HandlerMiddlewares, 1)
	})

	t.Run("runs the caller's decorator first", func(t *testing.T) {
		called := false
		decorate := func(_ string, opts backend.ServeOpts) (backend.ServeOpts, error) {
			called = true
			opts.HandlerMiddlewares = append(opts.HandlerMiddlewares, backend.HandlerMiddlewareFunc(func(next backend.Handler) backend.Handler { return next }))
			return opts, nil
		}
		opts, err := withV2Middleware(decorate, mw)("my-app", backend.ServeOpts{})
		require.NoError(t, err)
		assert.True(t, called)
		assert.Len(t, opts.HandlerMiddlewares, 2)
	})
}
