package plugin

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/grafana/authlib/authn"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	backendapp "github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	backendlog "github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/build/buildinfo"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/plugin/appadapter"
)

// Run is a convinience entry point for plugin backends to use, when they have
// implement App SDK functionality. It wraps plugin Manage().
//
// Plugin protocol v3 requests are authenticated with the authenticator from
// WithAuthenticator or, without one, from GRAFANA_JWKS_URL or
// GRAFANA_JWKS. Tokens must have the plugin ID or the manifest's
// API group as an audience. If neither is available, every plugin protocol v3
// request is rejected, unless authentication is skipped for local development
// with WithInsecureSkipAuthentication or GF_PLUGIN_INSECURE_SKIP_AUTHENTICATION=true,
// which Grafana sets from insecure_skip_authentication = true in the plugin's
// [plugin.<id>] settings. The plugin still starts, so a host that sends it no
// plugin protocol v3 requests needs none of these. When the host sets any of
// them, no options are needed:
//
//	func main() {
//		if err := plugin.Run(myapp.Provider()); err != nil {
//			backendlog.DefaultLogger.Error(err.Error())
//			os.Exit(1)
//		}
//	}
//
// If the App needs to talk to the Kubernetes API server, the rest.Config
// can be overridden via WithKubeConfig. Without one from WithKubeConfig or
// BuildKubeConfig, the App's runner is not started, since its informers and
// other runnables have no API server to talk to:
//
//	err = plugin.Run(myapp.Provider(), plugin.WithKubeConfig(kubeConfig))
//
// Existing plugins that already have a plugin ID and an
// backendapp.InstanceFactoryFunc (e.g. one built with app.New from an
// existing datasource or app plugin) can keep using them via WithPluginID
// and WithAppFunc, instead of relying on the manifest-derived ID and stub
// instance:
//
//	err := plugin.Run(
//		myapp.Provider(),
//		plugin.WithPluginID("my-existing-plugin-id"),
//		plugin.WithAppFunc(app.NewInstanceFactoryFunc(existingApp)),
//	)
//
// WithManageOpts allows further customization of how the plugin backend is
// managed, such as enabling GRPC settings or tracing opts. Note that
// ManageOpts.ExtraPlugins is reserved for appadapter and cannot be set:
//
//	err := plugin.Run(
//		myapp.Provider(),
//		plugin.WithManageOpts(backendapp.ManageOpts{
//			TracingOpts: tracing.Opts{CustomAttributes: []attribute.KeyValue{
//				attribute.String("plugin", "my-plugin"),
//			}},
//		}),
//	)
func Run(provider app.Provider, opts ...RunOption) error {
	var cfg runConfig
	for _, o := range opts {
		o(&cfg)
	}

	// Forward logs into the plugin logger stream so they appear cleanly in Grafana logs.
	logging.DefaultLogger = NewLogger(backendlog.DefaultLogger)

	if provider == nil {
		return errors.New("provider cannot be nil")
	}
	manifestData := provider.Manifest().ManifestData
	if manifestData == nil {
		return errors.New("embedded manifest required")
	}

	// If the pluginID was not given, use the plugin.json ID that the plugin SDK
	// build compiles in, or else the manifest's app name. The authenticator
	// accepts it as a token audience, and Grafana sets that to the plugin.json ID.
	if cfg.pluginID == "" {
		if info, err := buildinfo.GetBuildInfo(); err == nil && info.PluginID != "" {
			cfg.pluginID = info.PluginID
		} else {
			cfg.pluginID = manifestData.AppName
		}
	}

	if os.Getenv(EnvVarInsecureSkipAuthentication) == "true" {
		cfg.insecureSkipAuthentication = true
	}

	if cfg.authenticator == nil {
		authenticator, err := buildAuthenticator(cfg.pluginID, manifestData)
		switch {
		case err == nil:
			cfg.authenticator = authenticator
		case !errors.Is(err, ErrNoSigningKeys):
			return err
		case !cfg.insecureSkipAuthentication:
			// Without an authenticator, the plugin v3 servers reject every request.
			// Start anyway: hosts that never send plugin v3 requests, such as a
			// Grafana that routes the App's API elsewhere, need not configure one.
			logging.DefaultLogger.Warn("no authenticator configured: plugin protocol v3 requests will be rejected; set "+EnvVarGrafanaJWKSURL+" or "+EnvVarGrafanaJWKS+", or use WithAuthenticator; for local development, use WithInsecureSkipAuthentication or set "+EnvVarInsecureSkipAuthentication+"=true",
				"pluginId", cfg.pluginID)
		default:
			// Authentication is explicitly skipped. Anything that can reach the
			// plugin can then claim any identity, so make that visible.
			logging.DefaultLogger.Warn("plugin protocol v3 requests are not verified: callers can claim any identity; use this only for local development",
				"pluginId", cfg.pluginID)
		}
	}

	if cfg.kubeConfig == nil {
		kubeConfig, err := BuildKubeConfig(*manifestData)
		if err != nil {
			logging.DefaultLogger.Warn("no kube config for api access", "error", err)
		} else {
			cfg.kubeConfig = kubeConfig
		}
	}

	var kubeConfig rest.Config
	if cfg.kubeConfig != nil {
		kubeConfig = *cfg.kubeConfig
	}

	appConfig := app.Config{
		KubeConfig:     kubeConfig,
		ManifestData:   *manifestData,
		SpecificConfig: provider.SpecificConfig(),
	}
	a, err := provider.NewApp(appConfig)
	if err != nil {
		return err
	}

	// If a standard plugin backend app was not given, use our stub one.
	if cfg.appFunc == nil {
		cfg.appFunc = newStubAppInstance
	}

	// Set ExtraPlugins to handle the plugin v3 interfaces.
	if len(cfg.manageOpts.ExtraPlugins) > 0 {
		return errors.New("ExtraPlugins cannot be overridden")
	}
	serveOpts := appadapter.ServeOpts(a)
	serveOpts.Authenticator = cfg.authenticator
	serveOpts.PluginID = cfg.pluginID
	serveOpts.InsecureSkipAuthentication = cfg.insecureSkipAuthentication
	cfg.manageOpts.ExtraPlugins = serveOpts.PluginSet()

	// Start any background operations that the App requires, if it can reach
	// the API server. Without a kube config, they would only fail repeatedly.
	if cfg.kubeConfig != nil {
		runner := a.Runner()
		var runnerWait sync.WaitGroup
		runnerCtx, runnerCancel := context.WithCancel(context.Background())
		defer func() {
			runnerCancel()
			runnerWait.Wait()
		}()
		runnerWait.Go(func() {
			err := runner.Run(runnerCtx)
			if err != nil && !errors.Is(err, context.Canceled) {
				backendlog.DefaultLogger.Error(err.Error())
			}
		})
	} else {
		logging.DefaultLogger.Info("not starting the app runner: no kube config for api access", "pluginId", cfg.pluginID)
	}

	return manage(cfg.pluginID, cfg.appFunc, cfg.manageOpts)
}

// manage is backendapp.Manage, indirected so that tests can run Run without a plugin host.
var manage = backendapp.Manage

// WithKubeConfig sets the rest.Config used to communicate with the Kubernetes API server.
// If not provided, Run will attempt to build one via BuildKubeConfig, and
// without either, the App's runner is not started.
func WithKubeConfig(kubeConfig rest.Config) RunOption {
	return func(cfg *runConfig) {
		cfg.kubeConfig = &kubeConfig
	}
}

// WithPluginID overrides the plugin ID, which otherwise defaults to the App's manifest AppName.
func WithPluginID(pluginID string) RunOption {
	return func(cfg *runConfig) {
		cfg.pluginID = pluginID
	}
}

// WithAppFunc overrides the backend.InstanceFactoryFunc used to construct plugin instances,
// which otherwise defaults to a stub instance that only answers health checks.
func WithAppFunc(appFunc backendapp.InstanceFactoryFunc) RunOption {
	return func(cfg *runConfig) {
		cfg.appFunc = appFunc
	}
}

// WithManageOpts sets the backendapp.ManageOpts used when managing the plugin backend.
func WithManageOpts(manageOpts backendapp.ManageOpts) RunOption {
	return func(cfg *runConfig) {
		cfg.manageOpts = manageOpts
	}
}

// WithAuthenticator sets the authenticator that verifies the access token on
// each plugin protocol v3 request (see grpcplugin.ServeOpts.Authenticator),
// instead of building one from GRAFANA_JWKS_URL or GRAFANA_JWKS.
func WithAuthenticator(authenticator authn.Authenticator) RunOption {
	return func(cfg *runConfig) {
		cfg.authenticator = authenticator
	}
}

// WithInsecureSkipAuthentication serves plugin protocol v3 requests without
// verifying them when no authenticator is set or configured by
// GRAFANA_JWKS_URL or GRAFANA_JWKS. A request's access token is parsed without
// checking its signature, so handlers get the identity it claims; requests
// without one have no identity, and their outbound requests act as the plugin
// (see grpcplugin.ServeOpts.InsecureSkipAuthentication). Use it only for local development.
func WithInsecureSkipAuthentication() RunOption {
	return func(cfg *runConfig) {
		cfg.insecureSkipAuthentication = true
	}
}

type RunOption func(cfg *runConfig)

type runConfig struct {
	kubeConfig                 *rest.Config
	pluginID                   string
	appFunc                    backendapp.InstanceFactoryFunc
	manageOpts                 backendapp.ManageOpts
	authenticator              authn.Authenticator
	insecureSkipAuthentication bool
}

type stubInstance struct{}

func (stubInstance) CheckHealth(_ context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	// TODO: Should we use the healthchecks from app.App?
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "ok",
	}, nil
}

func newStubAppInstance(_ context.Context, _ backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	return stubInstance{}, nil
}
