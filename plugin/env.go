package plugin

import "github.com/grafana/grafana-plugin-sdk-go/backend/auth"

// Environment variables read by a plugin process. Grafana sets them when it
// starts the plugin. They are defined by the plugin SDK (see the
// grafana-plugin-sdk-go backend/auth package), and repeated here for
// compatibility.
const (
	// EnvVarAPIAccessRouterURL names the environment variable holding the URL of the API server
	// the plugin's resource clients talk to.
	EnvVarAPIAccessRouterURL = auth.EnvVarAPIAccessRouterURL
	// EnvVarAPIAccessCAFile names the environment variable holding the path of a CA file
	// used to verify the API server's certificate.
	EnvVarAPIAccessCAFile = auth.EnvVarAPIAccessCAFile
	// EnvVarAPIAccessInsecureTLS names the environment variable that, when "true",
	// skips verification of the API server's certificate.
	EnvVarAPIAccessInsecureTLS = auth.EnvVarAPIAccessInsecureTLS
	// EnvVarAPIAccessCAPToken names the environment variable holding the Cloud Access Policy (CAP)
	// token exchanged for access tokens.
	EnvVarAPIAccessCAPToken = auth.EnvVarAPIAccessCAPToken
	// EnvVarAPIAccessTokenExchangeURL names the environment variable holding the URL of the
	// token exchange service.
	EnvVarAPIAccessTokenExchangeURL = auth.EnvVarAPIAccessTokenExchangeURL
	// EnvVarAPIAccessBearerToken names the environment variable holding a bearer token used
	// when token exchange is not configured.
	//
	// Deprecated: configure token exchange with EnvVarAPIAccessTokenExchangeURL and
	// EnvVarAPIAccessCAPToken instead.
	EnvVarAPIAccessBearerToken = auth.EnvVarAPIAccessBearerToken //nolint:staticcheck // the deprecated name is still read

	// EnvVarGrafanaJWKSURL names the environment variable holding the
	// URL of the JWKS used to verify access tokens. Keys are downloaded on demand.
	EnvVarGrafanaJWKSURL = auth.EnvVarGrafanaJWKSURL
	// EnvVarGrafanaJWKS names the environment variable holding the
	// JWKS used to verify access tokens, as a JSON document.
	EnvVarGrafanaJWKS = auth.EnvVarGrafanaJWKS

	// EnvVarInsecureSkipAuthentication names the environment variable that, when
	// "true", makes Run serve requests without verifying them if no signing
	// keys are configured (see WithInsecureSkipAuthentication). Grafana sets it
	// from insecure_skip_authentication = true in the plugin's [plugin.<id>]
	// settings. Use it only for local development.
	EnvVarInsecureSkipAuthentication = auth.EnvVarInsecureSkipAuthentication

	// EnvVarTarget names the environment variable that selects which named runnables
	// of the App Run starts (see WithTarget): a comma-separated list of names, "all",
	// or "none". Unset, empty, or "all" runs everything.
	EnvVarTarget = "GF_APP_TARGET"
)

// getEnv returns the value of the environment variable name, falling back to
// the legacy name it replaces when it is empty.
func getEnv(name string) string {
	return auth.GetEnv(name)
}
