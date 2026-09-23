package k8s

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// capturingRoundTripper records the last request it received so tests
// can inspect the headers that were set by the transport wrapper.
type capturingRoundTripper struct {
	lastReq *http.Request
	resp    *http.Response
	err     error
}

func (c *capturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.lastReq = req
	if c.resp != nil {
		return c.resp, c.err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
	}, c.err
}

// staticExchangeFunc returns an ExchangerFunc that always succeeds with token "tok".
// Use in tests that only need credentials wiring, not audience/namespace assertions.
func staticExchangeFunc() func(context.Context, []string, string) (string, error) {
	return func(context.Context, []string, string) (string, error) {
		return "tok", nil
	}
}

func TestNewTokenExchangeRestConfig(t *testing.T) {
	tests := []struct {
		name        string
		creds       TokenExchangeCredentials
		target      RemoteServiceTarget
		wantErr     bool
		errContains string
		checkCfg    func(t *testing.T, cfg *rest.Config)
	}{
		{
			name: "returns correct host and APIPath",
			creds: TokenExchangeCredentials{
				ExchangerFunc: staticExchangeFunc(),
			},
			target: RemoteServiceTarget{
				Host:      "https://apiext.example.com",
				Audiences: []string{"apiextensions.k8s.io"},
			},
			checkCfg: func(t *testing.T, cfg *rest.Config) {
				assert.Equal(t, "https://apiext.example.com", cfg.Host)
				assert.Equal(t, "/apis", cfg.APIPath)
			},
		},
		{
			name: "sets InsecureTLS",
			creds: TokenExchangeCredentials{
				ExchangerFunc: staticExchangeFunc(),
			},
			target: RemoteServiceTarget{
				Host:        "https://host",
				Audiences:   []string{"aud"},
				InsecureTLS: true,
			},
			checkCfg: func(t *testing.T, cfg *rest.Config) {
				assert.True(t, cfg.TLSClientConfig.Insecure)
			},
		},
		{
			name: "sets CAFile",
			creds: TokenExchangeCredentials{
				ExchangerFunc: staticExchangeFunc(),
			},
			target: RemoteServiceTarget{
				Host:      "https://host",
				Audiences: []string{"aud"},
				CAFile:    "/etc/ssl/ca.crt",
			},
			checkCfg: func(t *testing.T, cfg *rest.Config) {
				assert.Equal(t, "/etc/ssl/ca.crt", cfg.TLSClientConfig.CAFile)
			},
		},
		{
			name:  "error when URL and Token are empty without ExchangerFunc",
			creds: TokenExchangeCredentials{},
			target: RemoteServiceTarget{
				Host:      "https://host",
				Audiences: []string{"aud"},
			},
			wantErr:     true,
			errContains: "TokenExchangeURL and Token are required",
		},
		{
			name: "error when only URL is set without ExchangerFunc",
			creds: TokenExchangeCredentials{
				TokenExchangeURL: "https://exchange.example.com",
			},
			target: RemoteServiceTarget{
				Host:      "https://host",
				Audiences: []string{"aud"},
			},
			wantErr:     true,
			errContains: "TokenExchangeURL and Token are required",
		},
		{
			name: "error when only Token is set without ExchangerFunc",
			creds: TokenExchangeCredentials{
				Token: "my-token",
			},
			target: RemoteServiceTarget{
				Host:      "https://host",
				Audiences: []string{"aud"},
			},
			wantErr:     true,
			errContains: "TokenExchangeURL and Token are required",
		},
		{
			name: "with real URL and Token creates authlib client",
			creds: TokenExchangeCredentials{
				TokenExchangeURL: "https://exchange.example.com/token",
				Token:            "my-cap-token",
			},
			target: RemoteServiceTarget{
				Host:      "https://host",
				Audiences: []string{"aud"},
			},
			checkCfg: func(t *testing.T, cfg *rest.Config) {
				assert.NotNil(t, cfg.WrapTransport)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewTokenExchangeRestConfig(tt.creds, tt.target)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			if tt.checkCfg != nil {
				tt.checkCfg(t, cfg)
			}
		})
	}
}

func TestNewTokenExchangeRemoteRestConfig(t *testing.T) {
	tests := []struct {
		name        string
		creds       TokenExchangeCredentials
		target      RemoteServiceTarget
		wantErr     bool
		errContains string
		checkCfg    func(t *testing.T, cfg *RemoteRestConfig)
	}{
		{
			name: "returns correct host with OverrideAuth true",
			creds: TokenExchangeCredentials{
				ExchangerFunc: staticExchangeFunc(),
			},
			target: RemoteServiceTarget{
				Host:      "https://iam.example.com",
				Audiences: []string{"iam.grafana.app"},
			},
			checkCfg: func(t *testing.T, cfg *RemoteRestConfig) {
				assert.Equal(t, "https://iam.example.com", cfg.Host)
				assert.True(t, cfg.OverrideAuth)
				assert.NotNil(t, cfg.WrapTransport)
			},
		},
		{
			name: "sets TLS config",
			creds: TokenExchangeCredentials{
				ExchangerFunc: staticExchangeFunc(),
			},
			target: RemoteServiceTarget{
				Host:        "https://host",
				Audiences:   []string{"aud"},
				InsecureTLS: true,
				CAFile:      "/ca.pem",
			},
			checkCfg: func(t *testing.T, cfg *RemoteRestConfig) {
				assert.True(t, cfg.TLSClientConfig.Insecure)
				assert.Equal(t, "/ca.pem", cfg.TLSClientConfig.CAFile)
			},
		},
		{
			name:  "error when credentials are missing",
			creds: TokenExchangeCredentials{},
			target: RemoteServiceTarget{
				Host:      "https://host",
				Audiences: []string{"aud"},
			},
			wantErr:     true,
			errContains: "TokenExchangeURL and Token are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewTokenExchangeRemoteRestConfig(tt.creds, tt.target)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			if tt.checkCfg != nil {
				tt.checkCfg(t, cfg)
			}
		})
	}
}

func TestTokenExchangeNamespaceDefault(t *testing.T) {
	exchFunc := func(_ context.Context, _ []string, namespace string) (string, error) {
		return namespace, nil
	}

	tests := []struct {
		name      string
		namespace string
		wantNS    string
	}{
		{
			name:      "empty namespace defaults to wildcard",
			namespace: "",
			wantNS:    "*",
		},
		{
			name:      "explicit namespace is preserved",
			namespace: "my-ns",
			wantNS:    "my-ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewTokenExchangeRestConfig(
				TokenExchangeCredentials{ExchangerFunc: exchFunc},
				RemoteServiceTarget{
					Host:      "https://host",
					Audiences: []string{"aud"},
					Namespace: tt.namespace,
				},
			)
			require.NoError(t, err)

			base := &capturingRoundTripper{}
			transport := cfg.WrapTransport(base)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://host/apis", nil)
			require.NoError(t, err)

			_, err = transport.RoundTrip(req)
			require.NoError(t, err)

			// The exchFunc returns the namespace as the token, so we can
			// inspect X-Access-Token to verify the namespace value used.
			assert.Equal(t, tt.wantNS, base.lastReq.Header.Get("X-Access-Token"))
		})
	}
}

func TestTokenExchangeTransportRoundTrip(t *testing.T) {
	tests := []struct {
		name         string
		exchangeFunc func(ctx context.Context, audiences []string, namespace string) (string, error)
		audiences    []string
		namespace    string
		wantErr      bool
		errContains  string
		wantHeader   string
	}{
		{
			name: "injects X-Access-Token header",
			exchangeFunc: func(_ context.Context, _ []string, _ string) (string, error) {
				return "signed-jwt-token", nil
			},
			audiences:  []string{"apiextensions.k8s.io"},
			wantHeader: "signed-jwt-token",
		},
		{
			name: "propagates exchange error",
			exchangeFunc: func(_ context.Context, _ []string, _ string) (string, error) {
				return "", errors.New("exchange service unavailable")
			},
			audiences:   []string{"aud"},
			wantErr:     true,
			errContains: "token exchange failed",
		},
		{
			name: "passes correct single audience and namespace",
			exchangeFunc: func(_ context.Context, audiences []string, namespace string) (string, error) {
				return strings.Join(audiences, ",") + ":" + namespace, nil
			},
			audiences:  []string{"apiextensions.k8s.io"},
			namespace:  "my-ns",
			wantHeader: "apiextensions.k8s.io:my-ns",
		},
		{
			name: "passes multiple audiences",
			exchangeFunc: func(_ context.Context, audiences []string, namespace string) (string, error) {
				return strings.Join(audiences, ",") + ":" + namespace, nil
			},
			audiences:  []string{"dashboard.grafana.app", "provisioning.grafana.app"},
			namespace:  "*",
			wantHeader: "dashboard.grafana.app,provisioning.grafana.app:*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audiences := tt.audiences
			if audiences == nil {
				audiences = []string{"apiextensions.k8s.io"}
			}
			namespace := tt.namespace
			if namespace == "" {
				namespace = "*"
			}

			base := &capturingRoundTripper{}
			transport := &tokenExchangeTransport{
				exchangeFunc: tt.exchangeFunc,
				base:         base,
				audiences:    audiences,
				namespace:    namespace,
			}

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://host/apis", nil)
			require.NoError(t, err)
			req.Header.Set("Existing-Header", "keep-me")

			_, err = transport.RoundTrip(req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, base.lastReq)
			assert.Equal(t, tt.wantHeader, base.lastReq.Header.Get("X-Access-Token"))
			assert.Equal(t, "keep-me", base.lastReq.Header.Get("Existing-Header"))
		})
	}
}

func TestTokenExchangeTransportClonesRequest(t *testing.T) {
	base := &capturingRoundTripper{}
	transport := &tokenExchangeTransport{
		exchangeFunc: staticExchangeFunc(),
		base:         base,
		audiences:    []string{"aud"},
		namespace:    "*",
	}

	original, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://host/apis", nil)
	require.NoError(t, err)

	_, err = transport.RoundTrip(original)
	require.NoError(t, err)

	// The original request should not have the token header set.
	assert.Empty(t, original.Header.Get("X-Access-Token"))
	// The cloned request sent to base should have the header.
	assert.Equal(t, "tok", base.lastReq.Header.Get("X-Access-Token"))
}
