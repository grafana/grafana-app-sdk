package plugin

import (
	"context"
	"errors"
	"sync"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	backendapp "github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	backendlog "github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/plugin/appadapter"
)

// Run is a convinience entry point for plugin backends to use, when they have
// implement App SDK functionality. It wraps plugin Manage().
//
// In the simplest case, no options are needed:
//
//	func main() {
//		if err := plugin.Run(myapp.Provider()); err != nil {
//			backendlog.DefaultLogger.Error(err.Error())
//			os.Exit(1)
//		}
//	}
//
// If the App needs to talk to the Kubernetes API server, the rest.Config
// can be overridden via WithKubeConfig:
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

	appConfig := app.Config{
		KubeConfig:     cfg.kubeConfig,
		ManifestData:   *manifestData,
		SpecificConfig: provider.SpecificConfig(),
	}
	a, err := provider.NewApp(appConfig)
	if err != nil {
		return err
	}

	// If the pluginID was not given, we can assume it from the manifest.
	if cfg.pluginID == "" {
		cfg.pluginID = manifestData.AppName
	}

	// If a standard plugin backend app was not given, use our stub one.
	if cfg.appFunc == nil {
		cfg.appFunc = newStubAppInstance
	}

	// Set ExtraPlugins to handle the plugin v3 interfaces.
	if len(cfg.manageOpts.ExtraPlugins) > 0 {
		return errors.New("ExtraPlugins cannot be overridden")
	}
	cfg.manageOpts.ExtraPlugins = appadapter.New(a)

	// Start any background operations that the App requires.
	runner := a.Runner()
	var runnerWait sync.WaitGroup
	runnerCtx, runnerCancel := context.WithCancel(context.Background())
	defer func() {
		runnerCancel()
		runnerWait.Wait()
	}()
	runnerWait.Add(1)
	go func() {
		defer runnerWait.Done()

		err := runner.Run(runnerCtx)
		if err != nil && !errors.Is(err, context.Canceled) {
			backendlog.DefaultLogger.Error(err.Error())
		}
	}()

	return manage(cfg.pluginID, cfg.appFunc, cfg.manageOpts)
}

// manage is backendapp.Manage, indirected so that tests can run Run without a plugin host.
var manage = backendapp.Manage

// WithKubeConfig sets the rest.Config used to communicate with the Kubernetes API server.
func WithKubeConfig(kubeConfig rest.Config) RunOption {
	return func(cfg *runConfig) {
		cfg.kubeConfig = kubeConfig
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

type RunOption func(cfg *runConfig)

type runConfig struct {
	kubeConfig rest.Config
	pluginID   string
	appFunc    backendapp.InstanceFactoryFunc
	manageOpts backendapp.ManageOpts
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
