package k8s

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

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

	// IsServiceIdentity reports whether ctx carries an in-process service
	// identity, which has no signed token to exchange. Such requests act as the
	// service. Grafana passes identity.IsServiceIdentity. If nil, a request
	// whose AuthInfo has no signed token fails rather than act as the service.
	// Ignored if ExchangerFunc is set.
	IsServiceIdentity func(ctx context.Context) bool

	// ExchangerFunc, when set, is called to obtain access tokens instead of
	// creating an internal authlib TokenExchangeClient from TokenExchangeURL/Token.
	// The function receives the request context plus the audiences and namespace
	// from the RemoteServiceTarget, and should return a valid signed access token.
	// The audiences are narrowed to the request's API group and the target's
	// BaseAudiences when that group is one of the target's Audiences (see
	// RemoteServiceTarget.Audiences).
	// When the context carries AuthInfo or a legacy caller ID token (see
	// IDTokenFromContext), exchange on behalf of that caller. Prefer AuthInfo's
	// access token over its ID token to preserve the delegation chain, and act
	// as the service only for in-process service identities.
	// Custom exchangers must also replace a wildcard namespace with AuthInfo's
	// concrete namespace when exchanging on behalf of a caller, as shown below.
	//
	// Use this to bring your own authlib version or a custom token source:
	//
	//   exchanger, _ := authnlib.NewTokenExchangeClient(myConfig)
	//   creds := k8s.TokenExchangeCredentials{
	//       ExchangerFunc: func(ctx context.Context, audiences []string, namespace string) (string, error) {
	//           subjectToken, _ := k8s.IDTokenFromContext(ctx)
	//           if identity.IsServiceIdentity(ctx) {
	//               subjectToken = ""
	//           } else if info, ok := types.AuthInfoFrom(ctx); ok && info != nil {
	//               subjectToken = info.GetAccessToken()
	//               if subjectToken == "" { subjectToken = info.GetIDToken() }
	//               if subjectToken == "" { return "", errors.New("caller has no signed token") }
	//               if namespace == "*" {
	//                   if callerNamespace := info.GetNamespace(); callerNamespace != "" && callerNamespace != "*" {
	//                       namespace = callerNamespace
	//                   }
	//               }
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
	// A request to /apis/<group>/... whose group is one of Audiences gets a
	// token for that group and BaseAudiences only, so the token is not valid
	// on the target's other groups. Other requests, such as discovery, get a
	// token for all of Audiences.
	Audiences []string
	// BaseAudiences are added to the token for a request to one of Audiences'
	// groups (e.g. []string{"grafana"} while verifiers still require it).
	// They should also be in Audiences.
	BaseAudiences []string
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
				exchangeFunc:  exchangeFunc,
				base:          rt,
				audiences:     target.Audiences,
				baseAudiences: target.BaseAudiences,
				namespace:     ns,
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
				exchangeFunc:  exchangeFunc,
				base:          rt,
				audiences:     target.Audiences,
				baseAudiences: target.BaseAudiences,
				namespace:     ns,
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
		if creds.IsServiceIdentity != nil && creds.IsServiceIdentity(ctx) {
			// The service acts as itself, as it has no caller to delegate.
			subjectToken = ""
		} else if info, ok := types.AuthInfoFrom(ctx); ok && info != nil {
			// Preserve the full delegation chain when invoked by an authenticated
			// plugin handler. Never downgrade a caller to service credentials.
			subjectToken = info.GetAccessToken()
			if subjectToken == "" {
				subjectToken = info.GetIDToken()
			}
			if subjectToken == "" {
				return "", errors.New("caller auth info has no access or ID token; set TokenExchangeCredentials.IsServiceIdentity to let service identities act as the service")
			}
			if namespace == "*" {
				if callerNamespace := info.GetNamespace(); callerNamespace != "" && callerNamespace != "*" {
					namespace = callerNamespace
				}
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
// use a context without a caller, or with a service identity recognized by
// TokenExchangeCredentials.IsServiceIdentity. AuthInfo in the context takes precedence over
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
	// baseAudiences are added to a token narrowed to the request's group.
	baseAudiences []string
	namespace     string
}

func (t *tokenExchangeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.exchangeFunc(req.Context(), t.audiencesFor(req), t.namespace)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("token exchange returned an empty access token")
	}
	req = req.Clone(req.Context())
	// The new access token carries the delegated identity. Remove old identity
	// and access credentials, including non-canonical header map keys.
	for name := range req.Header {
		if strings.EqualFold(name, "X-Grafana-Id") || strings.EqualFold(name, "X-Access-Token") || strings.EqualFold(name, "Authorization") {
			delete(req.Header, name)
		}
	}
	req.Header.Set("X-Access-Token", token)
	req.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(req)
}

// audiencesFor returns the audiences to request a token for req. A request to
// /apis/<group>/... whose group is one of the configured audiences gets a
// token for that group and the base audiences, so the token is not valid on
// the other groups. Any other request, such as discovery or a group not
// configured, gets every configured audience.
func (t *tokenExchangeTransport) audiencesFor(req *http.Request) []string {
	group := authnlib.APIGroupFromRequest(req)
	if group == "" || !slices.Contains(t.audiences, group) {
		return t.audiences
	}
	audiences := make([]string, 0, 1+len(t.baseAudiences))
	audiences = append(audiences, group)
	for _, audience := range t.baseAudiences {
		if audience != "" && !slices.Contains(audiences, audience) {
			audiences = append(audiences, audience)
		}
	}
	return audiences
}
