package plugin

import (
	"context"

	"github.com/grafana/grafana-plugin-sdk-go/config"
	"k8s.io/client-go/rest"
)

// serviceAccountKubeConfig builds a rest.Config from the plugin's provisioned
// service account, if available; returns nil if not.
//
// Note:
//   - Looks for GF_APP_URL and GF_PLUGIN_APP_CLIENT_SECRET.
//   - Requires externalServiceAccounts feature toggle and
//     [auth.managed_service_accounts] enabled=true
//     (or GF_AUTH_MANAGED_SERVICE_ACCOUNTS_ENABLED=true)
//   - Requires an "iam" block set in plugin.json with necessary permissions.
func serviceAccountKubeConfig() *rest.Config {
	// We explicitly don't pass a context here, as we _only_ want
	// configuration to be collected from environment variables.
	gcfg := config.GrafanaConfigFromContext(context.Background())

	appURL, err := gcfg.AppURL()
	if err != nil {
		return nil
	}

	secret, err := gcfg.PluginAppClientSecret()
	if err != nil {
		return nil
	}

	return &rest.Config{
		Host:        appURL,
		APIPath:     "/apis",
		BearerToken: secret,
	}
}
