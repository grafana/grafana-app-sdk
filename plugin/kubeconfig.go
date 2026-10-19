package plugin

import (
	"errors"
	"os"

	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
)

const (
	grafanaAudience = "grafana"
)

// KubeConfigOptions configures token exchange for a plugin's resource client.
// The zero value preserves BuildKubeConfig's authentication behavior.
type KubeConfigOptions struct {
	// AdditionalTokenAudiences are added to the audiences derived from the manifest,
	// and are kept when a token is narrowed to the request's API group (see
	// k8s.RemoteServiceTarget.BaseAudiences). Empty additions and duplicates are
	// ignored. The Cloud Access Policy (CAP) must allow all requested audiences. These values are unused with bearer authentication.
	AdditionalTokenAudiences []string
}

// BuildKubeConfig builds the rest.Config from environment variables.
//
// Picks up configuration for API access from:
// - API_ACCESS_ROUTER_URL
// - API_ACCESS_CA_FILE (optional)
// - API_ACCESS_INSECURE_TLS (optional)
//
// Configures token exchange if the following are set:
// - API_ACCESS_CAP_TOKEN
// - API_ACCESS_TOKEN_EXCHANGE_URL
//
// With token exchange, a request whose context carries verified AuthInfo or a
// legacy ID token (see k8s.ContextWithIDToken) is made as that caller, limited to
// the permissions the access policy delegates. Plugin protocol v3 handlers, such as admission,
// conversion and custom route handlers, get this by passing on the context
// they were called with, provided the plugin authenticates requests (see
// grpcplugin.ServeOpts.Authenticator). Other requests act as the plugin.
//
// Otherwise falls back to standard auth, using:
// - API_ACCESS_BEARER_TOKEN
func BuildKubeConfig(manifestData app.ManifestData) (*rest.Config, error) {
	return BuildKubeConfigWithOptions(manifestData, KubeConfigOptions{})
}

// BuildKubeConfigWithOptions builds the same environment-based configuration as
// BuildKubeConfig, with optional additional token-exchange audiences.
func BuildKubeConfigWithOptions(manifestData app.ManifestData, options KubeConfigOptions) (*rest.Config, error) {
	routerURL := os.Getenv("API_ACCESS_ROUTER_URL")
	if routerURL == "" {
		return nil, errors.New("no url provided: set API_ACCESS_ROUTER_URL")
	}

	// Check if token exchange is configured.
	tokenExchangeURL := os.Getenv("API_ACCESS_TOKEN_EXCHANGE_URL")
	capToken := os.Getenv("API_ACCESS_CAP_TOKEN")
	if tokenExchangeURL != "" || capToken != "" {
		return k8s.NewTokenExchangeRestConfig(
			k8s.TokenExchangeCredentials{
				Token:            capToken,
				TokenExchangeURL: tokenExchangeURL,
			},
			k8s.RemoteServiceTarget{
				Host:      routerURL,
				Audiences: TokenExchangeAudiences(manifestData, options.AdditionalTokenAudiences...),
				// A token narrowed to the request's API group keeps grafana and
				// the additional audiences, which deployments may still verify.
				BaseAudiences: append([]string{grafanaAudience}, options.AdditionalTokenAudiences...),
				InsecureTLS:   os.Getenv("API_ACCESS_INSECURE_TLS") == "true",
				CAFile:        os.Getenv("API_ACCESS_CA_FILE"),
			},
		)
	}

	// Fall back to auth without token exchange: a bearer token.
	if bearerToken := os.Getenv("API_ACCESS_BEARER_TOKEN"); bearerToken != "" {
		return &rest.Config{
			Host:        routerURL,
			APIPath:     "/apis",
			BearerToken: bearerToken,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: os.Getenv("API_ACCESS_INSECURE_TLS") == "true",
				CAFile:   os.Getenv("API_ACCESS_CA_FILE"),
			},
		}, nil
	}

	return nil, errors.New("no credentials provided: set API_ACCESS_TOKEN_EXCHANGE_URL+API_ACCESS_CAP_TOKEN or API_ACCESS_BEARER_TOKEN")
}

// TokenExchangeAudiences returns the manifest's API-group audiences followed by
// additional audiences, with duplicates removed. Empty additions are ignored;
// manifest groups retain their existing behavior, including empty values.
// The returned slice is independent of the inputs. Custom transports can use this
// helper to share BuildKubeConfigWithOptions' audience selection.
func TokenExchangeAudiences(manifestData app.ManifestData, additionalAudiences ...string) []string {
	seen := map[string]bool{manifestData.Group: true}
	audiences := []string{manifestData.Group}

	// Always include grafana as an audience.
	seen[grafanaAudience] = true
	audiences = append(audiences, grafanaAudience)

	if manifestData.ExtraPermissions != nil {
		for _, accessKind := range manifestData.ExtraPermissions.AccessKinds {
			if !seen[accessKind.Group] {
				seen[accessKind.Group] = true
				audiences = append(audiences, accessKind.Group)
			}
		}
	}
	for _, audience := range additionalAudiences {
		if audience == "" || seen[audience] {
			continue
		}
		seen[audience] = true
		audiences = append(audiences, audience)
	}
	return audiences
}
