package plugin

import (
	"context"
	"errors"
	"os"

	"github.com/grafana/grafana-plugin-sdk-go/config"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/logging"
)

// KubeConfigOptions configures token exchange for a plugin's resource client.
// The zero value preserves BuildKubeConfig's authentication behavior.
type KubeConfigOptions struct {
	// AdditionalTokenAudiences are added to the audiences derived from the manifest.
	// Empty additions and duplicates are ignored. The Cloud Access Policy (CAP) must allow all
	// requested audiences. These values are unused with basic or bearer authentication.
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
// Otherwise falls back to standard auth, using either:
// - API_ACCESS_BEARER_TOKEN or
// - API_ACCESS_USERNAME/API_ACCESS_PASSWORD
func BuildKubeConfig(manifestData app.ManifestData) (*rest.Config, error) {
	return BuildKubeConfigWithOptions(manifestData, KubeConfigOptions{})
}

// BuildKubeConfigWithOptions builds the same environment-based configuration as
// BuildKubeConfig, with optional additional token-exchange audiences.
func BuildKubeConfigWithOptions(manifestData app.ManifestData, options KubeConfigOptions) (*rest.Config, error) {
	// Check if managed service accounts are configured.
	// We explicitly don't pass a context here, as we _only_ want
	// configuration to be collected from environment variables.
	gcfg := config.GrafanaConfigFromContext(context.Background())
	appURL, _ := gcfg.AppURL()
	secret, _ := gcfg.PluginAppClientSecret()
	if appURL != "" && secret != "" {
		logging.DefaultLogger.Info("authenticating using grafana provided secret", "app_url", appURL)

		return &rest.Config{
			Host:        appURL,
			APIPath:     "/apis",
			BearerToken: secret,
		}, nil
	}

	routerURL := os.Getenv("API_ACCESS_ROUTER_URL")
	if routerURL == "" {
		return nil, errors.New("no url provided: set API_ACCESS_ROUTER_URL")
	}

	// Check if token exchange is configured.
	tokenExchangeURL := os.Getenv("API_ACCESS_TOKEN_EXCHANGE_URL")
	capToken := os.Getenv("API_ACCESS_CAP_TOKEN")
	if tokenExchangeURL != "" || capToken != "" {
		logging.DefaultLogger.Info("authenticating using token exchange",
			"router_url", routerURL, "token_exchange_url", tokenExchangeURL)

		return k8s.NewTokenExchangeRestConfig(
			k8s.TokenExchangeCredentials{
				Token:            capToken,
				TokenExchangeURL: tokenExchangeURL,
			},
			k8s.RemoteServiceTarget{
				Host:        routerURL,
				Audiences:   TokenExchangeAudiences(manifestData, options.AdditionalTokenAudiences...),
				InsecureTLS: os.Getenv("API_ACCESS_INSECURE_TLS") == "true",
				CAFile:      os.Getenv("API_ACCESS_CA_FILE"),
			},
		)
	}

	// Fall back to auth without token exchange: either a bearer token or basic auth.
	cfg := &rest.Config{
		Host:    routerURL,
		APIPath: "/apis",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: os.Getenv("API_ACCESS_INSECURE_TLS") == "true",
			CAFile:   os.Getenv("API_ACCESS_CA_FILE"),
		},
	}

	if bearerToken := os.Getenv("API_ACCESS_BEARER_TOKEN"); bearerToken != "" {
		logging.DefaultLogger.Info("authenticating using bearer token",
			"router_url", routerURL)

		cfg.BearerToken = bearerToken
		return cfg, nil
	}

	username := os.Getenv("API_ACCESS_USERNAME")
	password := os.Getenv("API_ACCESS_PASSWORD")
	if username != "" && password != "" {
		logging.DefaultLogger.Info("authenticating using basic auth",
			"router_url", routerURL, "user", username)

		cfg.Username = username
		cfg.Password = password
		return cfg, nil
	}

	return nil, errors.New("no credentials provided: set API_ACCESS_BEARER_TOKEN or API_ACCESS_USERNAME/API_ACCESS_PASSWORD")
}

// TokenExchangeAudiences returns the manifest's API-group audiences followed by
// additional audiences, with duplicates removed. Empty additions are ignored;
// manifest groups retain their existing behavior, including empty values.
// The returned slice is independent of the inputs. Custom transports can use this
// helper to share BuildKubeConfigWithOptions' audience selection.
func TokenExchangeAudiences(manifestData app.ManifestData, additionalAudiences ...string) []string {
	seen := map[string]bool{manifestData.Group: true}
	audiences := []string{manifestData.Group}
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
