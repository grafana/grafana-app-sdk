package plugin

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	pluginapp "github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/plugin/runner"
)

var (
	_ backend.CheckHealthHandler  = (*Runner)(nil)
	_ backend.CallResourceHandler = (*Runner)(nil)
	_ backend.QueryDataHandler    = (*Runner)(nil)
	_ backend.AdmissionHandler    = (*Runner)(nil)
	_ backend.ConversionHandler   = (*Runner)(nil)
)

type RunnerConfig struct {
	// MetricsConfig contains the configuration for exposing prometheus metrics, if desired
	MetricsConfig RunnerMetricsConfig
	// KubeConfig is the kubernetes rest.Config to use when communicating with the API server
	KubeConfig rest.Config
	// Filesystem is an fs.FS that can be used in lieu of the OS filesystem.
	// if empty, it defaults to os.DirFS(".")
	Filesystem fs.FS
}

// RunnerMetricsConfig contains configuration information for exposing prometheus metrics
type RunnerMetricsConfig struct {
	metrics.ExporterConfig
	Enabled   bool
	Namespace string
}

// Runner runs an app.App as a Grafana Plugin, capable of exposing admission (validation, mutation)
// and conversion as webhooks, and running a main control loop with reconcilers and watchers.
// It relies on the Kinds managed by the app.App already existing in the API server it talks to, either as CRD's
// or another type. It does not support certain advanced app.App functionality which is not natively supported by
// CRDs, such as arbitrary subresources (app.App.CallSubresource). It should be instantiated with NewRunner.
type Runner struct {
	config        RunnerConfig
	pluginRunner  *runner.PluginRunner
	metricsServer *runner.MetricsServerRunner
}

// NewRunner creates a new, properly-initialized instance of a Runner
func NewRunner(cfg RunnerConfig) *Runner {
	op := Runner{
		config: cfg,
	}

	if cfg.MetricsConfig.Enabled {
		exporter := metrics.NewExporter(cfg.MetricsConfig.ExporterConfig)
		op.metricsServer = runner.NewMetricsServerRunner(exporter)
	}
	return &op
}

// Run runs the Runner for the app built from the provided app.AppProvider, until the provided context.Context is closed,
// or an unrecoverable error occurs. If an app.App cannot be instantiated from the app.AppProvider, an error will be returned.
func (r *Runner) Run(ctx context.Context, provider app.Provider) error {
	if provider == nil {
		return errors.New("provider cannot be nil")
	}

	// only embedded manifests are supported for now
	manifest := provider.Manifest()
	if manifest.ManifestData == nil {
		return fmt.Errorf("missing embeded app manifest data")
	}
	appConfig := app.Config{
		KubeConfig:     r.config.KubeConfig,
		ManifestData:   *manifest.ManifestData,
		SpecificConfig: provider.SpecificConfig(),
	}

	// Create the app
	a, err := provider.NewApp(appConfig)
	if err != nil {
		return err
	}

	r.pluginRunner = runner.NewPluginRunner(a)

	// Build the operator
	runner := app.NewMultiRunner()

	// Main loop
	mainRunner := a.Runner()
	if mainRunner != nil {
		runner.AddRunnable(mainRunner)
	}

	// Metrics
	if r.metricsServer != nil {
		err = r.metricsServer.RegisterCollectors(runner.PrometheusCollectors()...)
		if err != nil {
			return err
		}
		runner.AddRunnable(r.metricsServer)
	}

	return runner.Run(ctx)
}

func (r *Runner) GetInstanceFactoryFunc() pluginapp.InstanceFactoryFunc {
	return func(_ context.Context, _ backend.AppInstanceSettings) (instancemgmt.Instance, error) {
		return r, nil
	}
}

func (r *Runner) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	if r.pluginRunner == nil {
		return nil, errors.New("pluginRunner not initialized")
	}
	return r.pluginRunner.QueryData(ctx, req)
}

func (r *Runner) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if r.pluginRunner == nil {
		return nil, errors.New("pluginRunner not initialized")
	}
	return r.pluginRunner.CheckHealth(ctx, req)
}

func (r *Runner) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if r.pluginRunner == nil {
		return errors.New("pluginRunner not initialized")
	}
	return r.pluginRunner.CallResource(ctx, req, sender)
}

func (r *Runner) MutateAdmission(ctx context.Context, req *backend.AdmissionRequest) (*backend.MutationResponse, error) {
	if r.pluginRunner == nil {
		return nil, errors.New("pluginRunner not initialized")
	}
	return r.pluginRunner.MutateAdmission(ctx, req)
}

func (r *Runner) ValidateAdmission(ctx context.Context, req *backend.AdmissionRequest) (*backend.ValidationResponse, error) {
	if r.pluginRunner == nil {
		return nil, errors.New("pluginRunner not initialized")
	}
	return r.pluginRunner.ValidateAdmission(ctx, req)
}

func (r *Runner) ConvertObjects(ctx context.Context, req *backend.ConversionRequest) (*backend.ConversionResponse, error) {
	if r.pluginRunner == nil {
		return nil, errors.New("pluginRunner not initialized")
	}
	return r.pluginRunner.ConvertObjects(ctx, req)
}
