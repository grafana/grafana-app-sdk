package k8s

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	authnlib "github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
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
	// When the context carries AuthInfo or a legacy caller ID token (see
	// IDTokenFromContext), exchange on behalf of that caller. Prefer AuthInfo's
	// access token over its ID token to preserve the delegation chain.
	//
	// Use this to bring your own authlib version or a custom token source:
	//
	//   exchanger, _ := authnlib.NewTokenExchangeClient(myConfig)
	//   creds := k8s.TokenExchangeCredentials{
	//       ExchangerFunc: func(ctx context.Context, audiences []string, namespace string) (string, error) {
	//           subjectToken, _ := k8s.IDTokenFromContext(ctx)
	//           if info, ok := types.AuthInfoFrom(ctx); ok && info != nil {
	//               subjectToken = info.GetAccessToken()
	//               if subjectToken == "" { subjectToken = info.GetIDToken() }
	//               if subjectToken == "" { return "", errors.New("caller has no signed token") }
	//           }
	//           resp, err := exchanger.Exchange(ctx, authnlib.TokenExchangeRequest{
	//               Audiences:    audiences,
	//               Namespace:    namespace,
	//               SubjectToken: subjectToken,
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
		return nil, errors.New("audiences are required")
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
		return nil, errors.New("audiences are required")
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
		// The caller's identity becomes part of the exchanged access token.
		subjectToken, _ := IDTokenFromContext(ctx)
		if info, ok := types.AuthInfoFrom(ctx); ok && info != nil {
			// Preserve the full delegation chain when invoked by an authenticated
			// plugin handler. Never downgrade a caller to service credentials.
			subjectToken = info.GetAccessToken()
			if subjectToken == "" {
				subjectToken = info.GetIDToken()
			}
			if subjectToken == "" {
				return "", errors.New("caller auth info has no access or ID token")
			}
		}
		resp, err := exchanger.Exchange(ctx, authnlib.TokenExchangeRequest{
			Audiences:    audiences,
			Namespace:    namespace,
			SubjectToken: subjectToken,
		})
		if err != nil {
			return "", err
		}
		return resp.Token, nil
	}, nil
}

type idTokenContextKey struct{}

// ContextWithIDToken returns a copy of ctx carrying the caller's Grafana ID token.
// An empty token returns ctx unchanged.
//
// Requests made with the returned context by a client from
// NewTokenExchangeRestConfig or NewTokenExchangeRemoteRestConfig exchange the ID
// token for an access token on behalf of the caller, so Grafana acts as the
// caller, limited to the permissions the access policy delegates, rather than
// as the service.
// This is deliberate: work done for a caller should not use the service's own
// permissions. Work that should act as the service, such as reconciling, should
// use a context without a caller. AuthInfo in the context takes precedence over
// this legacy ID-token value; its access token (or ID token) is exchanged instead.
func ContextWithIDToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, idTokenContextKey{}, token)
}

// IDTokenFromContext returns the caller's Grafana ID token, if ctx carries one.
func IDTokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(idTokenContextKey{}).(string)
	return token, ok && token != ""
}

// tokenExchangeTransport injects an X-Access-Token header by exchanging
// credentials before each request, on behalf of the caller when the request
// context carries AuthInfo or a legacy ID token (see ContextWithIDToken).
// Follows the same transport wrapper
// pattern as streamErrorTransport.
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
