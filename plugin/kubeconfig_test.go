package plugin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
)

// Keep compatibility with callers that store the original function value.
var _ func(app.ManifestData) (*rest.Config, error) = BuildKubeConfig

func TestTokenExchangeAudiences(t *testing.T) {
	manifest := app.ManifestData{
		Group: "example.test",
		ExtraPermissions: &app.Permissions{
			AccessKinds: []app.KindPermission{
				{Group: "other.test"},
				{Group: "example.test"},
			},
		},
	}
	tests := []struct {
		name      string
		manifest  app.ManifestData
		additions []string
		want      []string
	}{
		{
			name:     "manifest audiences are preserved",
			manifest: manifest,
			want:     []string{"example.test", "other.test"},
		},
		{
			name:      "additional audience is appended",
			manifest:  manifest,
			additions: []string{"router.test"},
			want:      []string{"example.test", "other.test", "router.test"},
		},
		{
			name:      "duplicate and empty additions are ignored",
			manifest:  manifest,
			additions: []string{"other.test", "router.test", "", "router.test"},
			want:      []string{"example.test", "other.test", "router.test"},
		},
		{
			name: "empty manifest group retains existing behavior",
			want: []string{""},
		},
		{
			name:      "empty additions do not remove an empty manifest group",
			additions: []string{"", "router.test"},
			want:      []string{"", "router.test"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TokenExchangeAudiences(tt.manifest, tt.additions...)
			assert.Equal(t, tt.want, got)
		})
	}
}

func setKubeConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"API_ACCESS_ROUTER_URL", "API_ACCESS_CAP_TOKEN", "API_ACCESS_TOKEN_EXCHANGE_URL", "API_ACCESS_BEARER_TOKEN", "API_ACCESS_USERNAME", "API_ACCESS_PASSWORD", "API_ACCESS_CA_FILE", "API_ACCESS_INSECURE_TLS"} {
		t.Setenv(key, "")
	}
	t.Setenv("API_ACCESS_ROUTER_URL", "https://router.test")
}

func TestBuildKubeConfigWithOptions(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    *rest.Config
		wantErr string
	}{
		{
			name: "bearer authentication",
			env:  map[string]string{"API_ACCESS_BEARER_TOKEN": "test-token"},
			want: &rest.Config{Host: "https://router.test", APIPath: "/apis", BearerToken: "test-token"},
		},
		{
			name: "basic authentication",
			env:  map[string]string{"API_ACCESS_USERNAME": "user", "API_ACCESS_PASSWORD": "password"},
			want: &rest.Config{Host: "https://router.test", APIPath: "/apis", Username: "user", Password: "password"},
		},
		{
			name: "bearer takes precedence over basic authentication",
			env:  map[string]string{"API_ACCESS_BEARER_TOKEN": "test-token", "API_ACCESS_USERNAME": "user", "API_ACCESS_PASSWORD": "password"},
			want: &rest.Config{Host: "https://router.test", APIPath: "/apis", BearerToken: "test-token"},
		},
		{
			name: "CA file is preserved",
			env:  map[string]string{"API_ACCESS_BEARER_TOKEN": "test-token", "API_ACCESS_CA_FILE": "/example/ca.pem"},
			want: &rest.Config{Host: "https://router.test", APIPath: "/apis", BearerToken: "test-token", TLSClientConfig: rest.TLSClientConfig{CAFile: "/example/ca.pem"}},
		},
		{
			name: "insecure TLS is preserved",
			env:  map[string]string{"API_ACCESS_BEARER_TOKEN": "test-token", "API_ACCESS_INSECURE_TLS": "true"},
			want: &rest.Config{Host: "https://router.test", APIPath: "/apis", BearerToken: "test-token", TLSClientConfig: rest.TLSClientConfig{Insecure: true}},
		},
		{
			name:    "missing credentials",
			wantErr: "no credentials provided: set API_ACCESS_BEARER_TOKEN or API_ACCESS_USERNAME/API_ACCESS_PASSWORD",
		},
		{
			name:    "partial exchange credentials do not fall back to bearer",
			env:     map[string]string{"API_ACCESS_CAP_TOKEN": "test-cap", "API_ACCESS_BEARER_TOKEN": "test-token"},
			wantErr: "TokenExchangeURL and Token are required when ExchangerFunc is not set",
		},
		{
			name:    "missing URL",
			env:     map[string]string{"API_ACCESS_ROUTER_URL": ""},
			wantErr: "no url provided: set API_ACCESS_ROUTER_URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setKubeConfigEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			manifest := app.ManifestData{Group: "example.test"}
			// Both entry points must preserve these concrete authentication settings.
			for _, build := range []func(app.ManifestData) (*rest.Config, error){
				BuildKubeConfig,
				func(m app.ManifestData) (*rest.Config, error) {
					return BuildKubeConfigWithOptions(m, KubeConfigOptions{AdditionalTokenAudiences: []string{"router.test"}})
				},
			} {
				got, err := build(manifest)
				if tt.wantErr != "" {
					require.EqualError(t, err, tt.wantErr)
					continue
				}
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestBuildKubeConfigWithOptions_TokenExchange(t *testing.T) {
	tests := []struct {
		name          string
		additions     []string
		wantAudiences []string
		deny          bool
	}{
		{name: "manifest audiences", wantAudiences: []string{"example.test"}},
		{name: "additional audience", additions: []string{"router.test"}, wantAudiences: []string{"example.test", "router.test"}},
		{name: "unauthorized audience", additions: []string{"router.test"}, wantAudiences: []string{"example.test", "router.test"}, deny: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setKubeConfigEnv(t)
			var signerCalls atomic.Int32
			signer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				signerCalls.Add(1)
				assert.Equal(t, "Bearer test-cap", r.Header.Get("Authorization"))
				var request struct {
					Audiences []string `json:"audiences"`
					Namespace string   `json:"namespace"`
				}
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				assert.Equal(t, tt.wantAudiences, request.Audiences)
				// Additional audiences must not change the existing namespace scope.
				assert.Equal(t, "*", request.Namespace)
				if tt.deny {
					w.WriteHeader(http.StatusForbidden)
					_, _ = io.WriteString(w, `{"error":"audience not allowed"}`)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":{"token":"test-access-token"}}`)
			}))
			defer signer.Close()
			var resourceCalls atomic.Int32
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resourceCalls.Add(1)
				assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
				assert.Equal(t, "test-access-token", r.Header.Get("X-Access-Token"))
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()
			t.Setenv("API_ACCESS_ROUTER_URL", backend.URL)
			t.Setenv("API_ACCESS_TOKEN_EXCHANGE_URL", signer.URL)
			t.Setenv("API_ACCESS_CAP_TOKEN", "test-cap")

			cfg, err := BuildKubeConfigWithOptions(app.ManifestData{Group: "example.test"}, KubeConfigOptions{AdditionalTokenAudiences: tt.additions})
			require.NoError(t, err)
			client, err := rest.HTTPClientFor(cfg)
			require.NoError(t, err)
			response, err := client.Get(backend.URL + "/apis/example.test/v1/examples")
			assert.Equal(t, int32(1), signerCalls.Load())
			if tt.deny {
				require.ErrorContains(t, err, "audience not allowed")
				assert.Zero(t, resourceCalls.Load())
				return
			}
			require.NoError(t, err)
			defer func() { _ = response.Body.Close() }()
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, int32(1), resourceCalls.Load())
		})
	}
}

func TestTokenExchangeAudiences_CopiesAdditions(t *testing.T) {
	additions := []string{"router.test"}
	got := TokenExchangeAudiences(app.ManifestData{Group: "example.test"}, additions...)
	additions[0] = "changed.test"
	assert.Equal(t, []string{"example.test", "router.test"}, got)
}
