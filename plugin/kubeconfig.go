package plugin

import (
	"errors"

	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/logging"
)

const (
	grafanaAudience = "grafana"
)

// KubeConfigOptions configures token exchange for a plugin's resource client.
// The zero value preserves BuildKubeConfig's authentication behavior.
type KubeConfigOptions struct {
	// AdditionalTokenAudiences are added to the audiences derived from the manifest.
	// Empty additions and duplicates are ignored. The Cloud Access Policy (CAP) must allow all
	// requested audiences. These values are unused with bearer authentication.
	AdditionalTokenAudiences []string
}

// BuildKubeConfig builds the rest.Config from environment variables.
//
// Picks up configuration for API access from:
// - GF_API_ACCESS_ROUTER_URL
// - GF_API_ACCESS_CA_FILE (optional)
// - GF_API_ACCESS_INSECURE_TLS (optional)
//
// Configures token exchange if the following are set:
// - GF_API_ACCESS_CAP_TOKEN
// - GF_API_ACCESS_TOKEN_EXCHANGE_URL
//
// With token exchange, a request whose context carries verified AuthInfo or a
// legacy ID token (see k8s.ContextWithIDToken) is made as that caller, limited to
// the permissions the access policy delegates. Plugin protocol v3 handlers, such as admission,
// conversion and custom route handlers, get this by passing on the context
// they were called with, provided the plugin authenticates requests (see
// grpcplugin.ServeOpts.Authenticator). Other requests act as the plugin.
//
// Otherwise falls back to standard auth, using:
// - GF_API_ACCESS_BEARER_TOKEN (deprecated, logs a warning when set)
//
// The legacy names without the GF_ prefix are read when the new names are unset.
func BuildKubeConfig(manifestData app.ManifestData) (*rest.Config, error) {
	return BuildKubeConfigWithOptions(manifestData, KubeConfigOptions{})
}

// BuildKubeConfigWithOptions builds the same environment-based configuration as
// BuildKubeConfig, with optional additional token-exchange audiences.
func BuildKubeConfigWithOptions(manifestData app.ManifestData, options KubeConfigOptions) (*rest.Config, error) {
	routerURL := getEnv(EnvVarAPIAccessRouterURL)
	if routerURL == "" {
		return nil, errors.New("no url provided: set " + EnvVarAPIAccessRouterURL)
	}

	bearerToken := getEnv(EnvVarAPIAccessBearerToken)
	if bearerToken != "" {
		logging.DefaultLogger.Warn(EnvVarAPIAccessBearerToken + " is deprecated: set " + EnvVarAPIAccessTokenExchangeURL + " and " + EnvVarAPIAccessCAPToken + " instead")
	}

	// Check if token exchange is configured.
	tokenExchangeURL := getEnv(EnvVarAPIAccessTokenExchangeURL)
	capToken := getEnv(EnvVarAPIAccessCAPToken)
	if tokenExchangeURL != "" || capToken != "" {
		return k8s.NewTokenExchangeRestConfig(
			k8s.TokenExchangeCredentials{
				Token:            capToken,
				TokenExchangeURL: tokenExchangeURL,
			},
			k8s.RemoteServiceTarget{
				Host:        routerURL,
				Audiences:   TokenExchangeAudiences(manifestData, options.AdditionalTokenAudiences...),
				InsecureTLS: getEnv(EnvVarAPIAccessInsecureTLS) == "true",
				CAFile:      getEnv(EnvVarAPIAccessCAFile),
			},
		)
	}

	// Fall back to auth without token exchange: a bearer token.
	if bearerToken != "" {
		return &rest.Config{
			Host:        routerURL,
			APIPath:     "/apis",
			BearerToken: bearerToken,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: getEnv(EnvVarAPIAccessInsecureTLS) == "true",
				CAFile:   getEnv(EnvVarAPIAccessCAFile),
			},
		}, nil
	}

	return nil, errors.New("no credentials provided: set " + EnvVarAPIAccessTokenExchangeURL + "+" + EnvVarAPIAccessCAPToken + " or " + EnvVarAPIAccessBearerToken)
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
