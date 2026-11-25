package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
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
// WithAuthenticator or, without one, from GF_JWKS_URL or
// GF_JWKS. Tokens must have the plugin ID or the manifest's
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
// An App that implements app.NamedRunnerProvider (simple.App does, see
// simple.App.AddNamedRunnable) can run just some of its named runnables, for
// example to split its informers from its other background work across
// deployments. Select them with WithTarget or GF_APP_TARGET, as a
// comma-separated list of names. The default, "all", runs the whole App runner,
// and "none" runs no runnables. The plugin server, including the API and
// diagnostics such as metrics, always runs. For example, to split an App across
// two deployments:
//
//	GF_APP_TARGET=none ./my-plugin       # serves the API only
//	GF_APP_TARGET=informers ./my-plugin  # also runs the informers (reconcilers and watchers)
//
// simple.App registers its informers as "informers". Any other name is one the
// App registered itself, for example with AddNamedRunnable("my-task", runnable),
// and several names can be selected with GF_APP_TARGET=informers,my-task.
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

	if len(cfg.targets) == 0 {
		cfg.targets = parseTargets(os.Getenv(EnvVarTarget))
	}
	runnerTargets, err := resolveTargets(cfg.targets)
	if err != nil {
		return err
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

	runner, err := selectRunner(a, runnerTargets)
	if err != nil {
		return err
	}

	// Start any background operations that the App requires, if it can reach
	// the API server. Without a kube config, they would only fail repeatedly.
	if cfg.kubeConfig != nil && runner != nil {
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
	} else if runner != nil {
		logging.DefaultLogger.Info("not starting the app runner: no kube config for api access", "pluginId", cfg.pluginID)
	}

	return manage(cfg.pluginID, cfg.appFunc, cfg.manageOpts)
}

const (
	// TargetAll is the target which runs all of the App's runnables. It is the default.
	TargetAll = "all"
	// TargetNone is the target which runs none of the App's runnables.
	TargetNone = "none"
)

// resolveTargets validates targets and returns the named runnable targets: nil for the App's
// whole Runner(), and empty for no runnables. TargetAll and TargetNone cannot be combined with other targets.
func resolveTargets(targets []string) ([]string, error) {
	for _, t := range targets {
		if (t == TargetAll || t == TargetNone) && len(targets) > 1 {
			return nil, fmt.Errorf("target %q cannot be combined with other targets", t)
		}
	}
	switch {
	case len(targets) == 0, targets[0] == TargetAll:
		return nil, nil
	case targets[0] == TargetNone:
		return []string{}, nil
	}
	return targets, nil
}

// selectRunner returns the Runnable for the named runnable targets from resolveTargets: the App's
// whole Runner() for nil, nil for none, or else the named runnables from app.NamedRunnerProvider.
func selectRunner(a app.App, targets []string) (app.Runnable, error) {
	if targets == nil {
		return a.Runner(), nil
	}
	if len(targets) == 0 {
		return nil, nil
	}
	provider, ok := a.(app.NamedRunnerProvider)
	if !ok {
		return nil, fmt.Errorf("targets %v requested, but the app does not provide named runners", targets)
	}
	named := provider.NamedRunners()
	multi := app.NewMultiRunner()
	for _, t := range targets {
		r, ok := named[t]
		if !ok {
			names := make([]string, 0, len(named))
			for n := range named {
				names = append(names, n)
			}
			sort.Strings(names)
			return nil, fmt.Errorf("unknown target %q: must be %q, %q, or one or more of [%s]", t, TargetAll, TargetNone, strings.Join(names, ", "))
		}
		multi.AddRunnable(r)
	}
	return multi, nil
}

// parseTargets splits a comma-separated target list, dropping blanks and duplicates.
func parseTargets(v string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range strings.Split(v, ",") {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
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

// WithTarget selects which of the App's named runnables (see app.NamedRunnerProvider) Run starts,
// instead of the whole App runner. "all" (the default) selects everything and "none" selects nothing,
// and neither can be combined with other names. The plugin server, including the API, always runs.
// Without WithTarget, the targets are read from the comma-separated GF_APP_TARGET.
// Run returns an error if a target does not exist.
func WithTarget(targets ...string) RunOption {
	return func(cfg *runConfig) {
		cfg.targets = targets
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
// instead of building one from GF_JWKS_URL or GF_JWKS.
func WithAuthenticator(authenticator authn.Authenticator) RunOption {
	return func(cfg *runConfig) {
		cfg.authenticator = authenticator
	}
}

// WithInsecureSkipAuthentication serves plugin protocol v3 requests without
// verifying them when no authenticator is set or configured by
// GF_JWKS_URL or GF_JWKS. A request's access token is parsed without
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
	targets                    []string
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
