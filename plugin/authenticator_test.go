package plugin

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/grafana/authlib/authn"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/grafana/grafana-app-sdk/app"
)

func TestBuildAuthenticatorVerifiesSignedAccessTokens(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	jwks := testJWKS(t, jose.JSONWebKey{Key: &key.PublicKey, KeyID: "test", Algorithm: string(jose.ES256), Use: "sig"})

	for _, tt := range []struct {
		name string
		env  func(t *testing.T)
	}{
		{name: "downloaded from URL", env: func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(jwks))
			}))
			t.Cleanup(server.Close)
			t.Setenv(EnvVarGrafanaAuthenticationJWKSURL, server.URL)
		}},
		{name: "explicit JWKS", env: func(t *testing.T) {
			t.Setenv(EnvVarGrafanaAuthenticationJWKS, jwks)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.env(t)
			authenticator, err := buildAuthenticator("example-app", &app.ManifestData{Group: "example.grafana.app"})
			require.NoError(t, err)

			authenticate := func(token string) error {
				_, err := authenticator.Authenticate(context.Background(), authn.NewGRPCTokenProvider(metadata.Pairs("x-access-token", token)))
				return err
			}

			// Either the manifest's API group or the plugin ID is an accepted audience.
			for _, audience := range []string{"example.grafana.app", "example-app"} {
				info, err := authenticator.Authenticate(context.Background(), authn.NewGRPCTokenProvider(metadata.Pairs("x-access-token", signTestToken(t, key, "test", audience))))
				require.NoError(t, err, audience)
				require.Equal(t, "stacks-1", info.GetNamespace())
			}

			err = authenticate(signTestToken(t, key, "test", "other.grafana.app"))
			require.True(t, authn.IsUnauthenticatedErr(err), "wrong audience: %v", err)

			err = authenticate(signTestToken(t, key, "unknown", "example.grafana.app"))
			require.True(t, authn.IsUnauthenticatedErr(err), "unknown key ID: %v", err)

			otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)
			require.Error(t, authenticate(signTestToken(t, otherKey, "test", "example.grafana.app")), "a token signed by another key must be rejected")
		})
	}
}

func TestBuildAuthenticatorConfiguration(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	public := jose.JSONWebKey{Key: &key.PublicKey, KeyID: "a", Algorithm: string(jose.ES256), Use: "sig"}
	validJWKS := testJWKS(t, public)
	withKID := func(k jose.JSONWebKey, kid string) jose.JSONWebKey { k.KeyID = kid; return k }
	encryption := public
	encryption.Use = "enc"

	manifest := &app.ManifestData{Group: "example.grafana.app"}
	for _, tt := range []struct {
		name     string
		url      string
		jwks     string
		pluginID string
		manifest *app.ManifestData
		wantErr  string
	}{
		{name: "https", url: "https://auth.example.com/v1/jwks", manifest: manifest},
		{name: "http on localhost", url: "http://localhost:3000/jwks", manifest: manifest},
		{name: "http on loopback IP", url: "http://127.0.0.1:3000/jwks", manifest: manifest},
		{name: "http on IPv6 loopback", url: "http://[::1]:3000/jwks", manifest: manifest},
		{name: "explicit JWKS", jwks: validJWKS, manifest: manifest},
		{name: "explicit JWKS with several keys", jwks: testJWKS(t, public, withKID(public, "b")), manifest: manifest},
		{name: "missing", manifest: manifest, wantErr: "set GRAFANA_AUTHENTICATION_JWKS_URL or GRAFANA_AUTHENTICATION_JWKS"},
		{name: "both sources", url: "https://auth.example.com/v1/jwks", jwks: validJWKS, manifest: manifest, wantErr: "set only one of"},
		{name: "http on remote host", url: "http://auth.example.com/jwks", manifest: manifest, wantErr: "https is required"},
		{name: "relative", url: "/jwks", manifest: manifest, wantErr: "absolute URL"},
		{name: "file URL", url: "file:///etc/jwks", manifest: manifest, wantErr: "absolute URL"},
		{name: "ftp scheme", url: "ftp://auth.example.com/jwks", manifest: manifest, wantErr: "unsupported scheme"},
		{name: "malformed JWKS", jwks: "not json", manifest: manifest, wantErr: "decode JWKS"},
		{name: "empty JWKS", jwks: `{"keys":[]}`, manifest: manifest, wantErr: "JWKS has no keys"},
		{name: "key without ID", jwks: testJWKS(t, withKID(public, "")), manifest: manifest, wantErr: "has no key ID"},
		{name: "private key", jwks: testJWKS(t, jose.JSONWebKey{Key: key, KeyID: "a", Algorithm: string(jose.ES256)}), manifest: manifest, wantErr: "not a public key"},
		{name: "encryption key", jwks: testJWKS(t, encryption), manifest: manifest, wantErr: "not a signing key"},
		{name: "duplicate key ID", jwks: testJWKS(t, public, public), manifest: manifest, wantErr: "duplicate key ID"},
		{name: "plugin ID without manifest", url: "https://auth.example.com/v1/jwks", pluginID: "example-app"},
		{name: "plugin ID without manifest group", url: "https://auth.example.com/v1/jwks", pluginID: "example-app", manifest: &app.ManifestData{}},
		{name: "no audience", url: "https://auth.example.com/v1/jwks", manifest: &app.ManifestData{}, wantErr: "a plugin ID or manifest API group is required"},
		{name: "no audience without manifest", url: "https://auth.example.com/v1/jwks", wantErr: "a plugin ID or manifest API group is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvVarGrafanaAuthenticationJWKSURL, tt.url)
			t.Setenv(EnvVarGrafanaAuthenticationJWKS, tt.jwks)
			authenticator, err := buildAuthenticator(tt.pluginID, tt.manifest)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, authenticator)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, authenticator)
		})
	}

	t.Setenv(EnvVarGrafanaAuthenticationJWKSURL, "")
	t.Setenv(EnvVarGrafanaAuthenticationJWKS, "")
	_, err = buildAuthenticator("example-app", manifest)
	require.ErrorIs(t, err, ErrNoSigningKeys)
}

func testJWKS(t *testing.T, keys ...jose.JSONWebKey) string {
	t.Helper()
	raw, err := json.Marshal(jose.JSONWebKeySet{Keys: keys})
	require.NoError(t, err)
	return string(raw)
}

func signTestToken(t *testing.T, key *ecdsa.PrivateKey, keyID, audience string) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, (&jose.SignerOptions{}).WithType(jose.ContentType(authn.TokenTypeAccess)).WithHeader("kid", keyID))
	require.NoError(t, err)
	token, err := jwt.Signed(signer).Claims(jwt.Claims{Subject: "access-policy:grafana", Audience: jwt.Audience{audience}, Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour))}).
		Claims(authn.AccessTokenClaims{Namespace: "stacks-1"}).Serialize()
	require.NoError(t, err)
	return token
}
