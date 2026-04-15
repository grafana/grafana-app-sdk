package k8s

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	authnlib "github.com/grafana/authlib/authn"
	"k8s.io/client-go/rest"
)

// TokenExchangeCredentials holds the shared auth-signer credentials used to obtain
// signed JWTs for authenticating with Grafana API services.
// These credentials are typically the same across all services.
type TokenExchangeCredentials struct {
	// TokenExchangeURL is the auth-signer endpoint.
	// Ignored if ExchangerFunc is set.
	TokenExchangeURL string

	// Token is the static CAP token the signer recognizes.
	// Ignored if ExchangerFunc is set.
	Token string

	// ExchangerFunc, when set, is called to obtain access tokens instead of
	// creating an internal authlib TokenExchangeClient from TokenExchangeURL/Token.
	// The function receives the request context plus the audiences and namespace
	// from the RemoteServiceTarget, and should return a valid signed access token.
	//
	// Use this to bring your own authlib version or a custom token source:
	//
	//   exchanger, _ := authnlib.NewTokenExchangeClient(myConfig)
	//   creds := k8s.TokenExchangeCredentials{
	//       ExchangerFunc: func(ctx context.Context, audiences []string, namespace string) (string, error) {
	//           resp, err := exchanger.Exchange(ctx, authnlib.TokenExchangeRequest{
	//               Audiences: audiences,
	//               Namespace: namespace,
	//           })
	//           if err != nil { return "", err }
	//           return resp.Token, nil
	//       },
	//   }
	ExchangerFunc func(ctx context.Context, audiences []string, namespace string) (string, error)
}

// RemoteServiceTarget describes a remote Grafana API service to connect to.
type RemoteServiceTarget struct {
	// Host is the direct URL of the target API server.
	Host string
	// Audiences is the set of intended audiences for the token
	// (e.g. []string{"apiextensions.k8s.io"} or []string{"dashboard.grafana.app", "provisioning.grafana.app"}).
	Audiences []string
	// Namespace is the token exchange namespace.
	// Defaults to "*" (all namespaces) if empty.
	Namespace string
	// InsecureTLS skips TLS verification.
	InsecureTLS bool
	// CAFile is an optional path to a CA bundle file.
	CAFile string
}

// NewTokenExchangeRestConfig returns a *rest.Config that authenticates requests via
// token exchange. Use this when the token-exchanged service replaces the entire
// KubeConfig (e.g. an external operator connecting directly to apiextensions-apiserver).
func NewTokenExchangeRestConfig(creds TokenExchangeCredentials, target RemoteServiceTarget) (*rest.Config, error) {
	exchangeFunc, err := newTokenExchangeFunc(creds)
	if err != nil {
		return nil, err
	}

	ns := target.Namespace
	if ns == "" {
		ns = "*"
	}

	if len(target.Audiences) == 0 {
		return nil, errors.New("Audiences are required")
	}

	return &rest.Config{
		Host:    target.Host,
		APIPath: "/apis",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: target.InsecureTLS,
			CAFile:   target.CAFile,
		},
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return &tokenExchangeTransport{
				exchangeFunc: exchangeFunc,
				base:         rt,
				audiences:    target.Audiences,
				namespace:    ns,
			}
		},
	}, nil
}

// NewTokenExchangeRemoteRestConfig returns a *RemoteRestConfig suitable for
// per-group routing via NewClientConfigWithExternalClients. The returned config
// sets OverrideAuth to true so inherited bearer tokens are cleared.
func NewTokenExchangeRemoteRestConfig(creds TokenExchangeCredentials, target RemoteServiceTarget) (*RemoteRestConfig, error) {
	exchangeFunc, err := newTokenExchangeFunc(creds)
	if err != nil {
		return nil, err
	}

	ns := target.Namespace
	if ns == "" {
		ns = "*"
	}

	if len(target.Audiences) == 0 {
		return nil, errors.New("Audiences are required")
	}

	return &RemoteRestConfig{
		Host: target.Host,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: target.InsecureTLS,
			CAFile:   target.CAFile,
		},
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return &tokenExchangeTransport{
				exchangeFunc: exchangeFunc,
				base:         rt,
				audiences:    target.Audiences,
				namespace:    ns,
			}
		},
		OverrideAuth: true,
	}, nil
}

func newTokenExchangeFunc(creds TokenExchangeCredentials) (func(ctx context.Context, audiences []string, namespace string) (string, error), error) {
	if creds.ExchangerFunc != nil {
		return creds.ExchangerFunc, nil
	}
	if creds.TokenExchangeURL == "" || creds.Token == "" {
		return nil, errors.New("TokenExchangeURL and Token are required when ExchangerFunc is not set")
	}
	exchanger, err := authnlib.NewTokenExchangeClient(authnlib.TokenExchangeConfig{
		Token:            creds.Token,
		TokenExchangeURL: creds.TokenExchangeURL,
	})
	if err != nil {
		return nil, fmt.Errorf("creating token exchange client: %w", err)
	}

	return func(ctx context.Context, audiences []string, namespace string) (string, error) {
		resp, err := exchanger.Exchange(ctx, authnlib.TokenExchangeRequest{
			Audiences: audiences,
			Namespace: namespace,
		})
		if err != nil {
			return "", err
		}
		return resp.Token, nil
	}, nil
}

// tokenExchangeTransport injects an X-Access-Token header by exchanging
// credentials before each request. Follows the same transport wrapper pattern
// as streamErrorTransport.
type tokenExchangeTransport struct {
	exchangeFunc func(ctx context.Context, audiences []string, namespace string) (string, error)
	base         http.RoundTripper
	audiences    []string
	namespace    string
}

func (t *tokenExchangeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.exchangeFunc(req.Context(), t.audiences, t.namespace)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	req = req.Clone(req.Context())
	req.Header.Set("X-Access-Token", token)
	req.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(req)
}
