package operator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"

	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/health"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/metrics"
)

// Runner runs an app.App as a standalone operator, capable of exposing admission (validation, mutation)
// and conversion as webhooks, and running a main control loop with reconcilers and watchers.
// It relies on the Kinds managed by the app.App already existing in the API server it talks to, either as CRD's
// or another type. It does not support certain advanced app.App functionality which is not natively supported by
// CRDs, such as arbitrary subresources (app.App.CallSubresource). It should be instantiated with NewRunner.
type Runner struct {
	config              RunnerConfig
	webhookServer       *webhookServerRunner
	metricsExporter     *metrics.Exporter
	healthCheck         health.Check
	metricsServer       *MetricsServer
	metricsServerRunner *app.SingletonRunner
	startMux            sync.Mutex
	running             bool
	runningWG           sync.WaitGroup
}

// NewRunner creates a new, properly-initialized instance of a Runner
func NewRunner(cfg RunnerConfig) (*Runner, error) {
	// Validate the KubeConfig by constructing a rest.RESTClient with it
	// TODO: this requires a GroupVersion, which gets set up based on the kind
	// _, err := rest.RESTClientFor(&cfg.KubeConfig)
	// if err != nil {
	// 	return nil, fmt.Errorf("invalid KubeConfig: %w", err)
	// }

	if cfg.MetricsConfig.Port <= 0 {
		cfg.MetricsConfig.Port = 9090
	}

	if cfg.MetricsConfig.HealthCheckInterval <= 0 {
		cfg.MetricsConfig.HealthCheckInterval = 1 * time.Minute
	}

	metricsServer := NewMetricsServer(cfg.MetricsConfig.MetricsServerConfig)
	op := Runner{
		config:              cfg,
		healthCheck:         cfg.HealthCheck,
		metricsServer:       metricsServer,
		metricsServerRunner: app.NewSingletonRunner(metricsServer, false),
	}

	if cfg.WebhookConfig.TLSConfig.CertPath != "" {
		ws, err := k8s.NewWebhookServer(k8s.WebhookServerConfig{
			Port: cfg.WebhookConfig.Port,
			TLSConfig: k8s.TLSConfig{
				CertPath: cfg.WebhookConfig.TLSConfig.CertPath,
				KeyPath:  cfg.WebhookConfig.TLSConfig.KeyPath,
			},
		})
		if err != nil {
			return nil, err
		}
		op.webhookServer = newWebhookServerRunner(ws)
	}
	if cfg.MetricsConfig.Enabled {
		op.metricsExporter = metrics.NewExporter(cfg.MetricsConfig.ExporterConfig)
	}

	return &op, nil
}

// RunnerConfig contains configuration information for the Runner.
type RunnerConfig struct {
	// WebhookConfig contains configuration information for exposing k8s webhooks and the App's
	// custom routes. The webhook server hosts both admission/conversion endpoints and the
	// /apis/... custom route paths on the same HTTPS port. This can be empty only if the App
	// has no admission, conversion, or custom route capabilities.
	WebhookConfig RunnerWebhookConfig
	// MetricsConfig contains the configuration for exposing prometheus metrics, if desired
	MetricsConfig RunnerMetricsConfig
	// Health checks for liveness and readiness
	HealthCheck health.Check
	// KubeConfig is the kubernetes rest.Config to use when communicating with the API server
	KubeConfig rest.Config
	// Filesystem is an fs.FS that can be used in lieu of the OS filesystem.
	// if empty, it defaults to os.DirFS(".")
	Filesystem fs.FS
}

// RunnerMetricsConfig contains configuration information for exposing prometheus metrics
type RunnerMetricsConfig struct {
	metrics.ExporterConfig
	MetricsServerConfig
	Namespace        string
	Enabled          bool
	ProfilingEnabled bool
}

type RunnerWebhookConfig struct {
	// Port is the port to open the webhook server on
	Port int
	// TLSConfig is the TLS Cert and Key to use for the HTTPS endpoints exposed for webhooks
	// and custom routes.
	TLSConfig k8s.TLSConfig
}

// Run runs the Runner for the app built from the provided app.AppProvider, until the provided context.Context is closed,
// or an unrecoverable error occurs. If an app.App cannot be instantiated from the app.AppProvider, an error will be returned.
// Webserver components of Run (such as webhooks and the prometheus exporter) will remain running so long as at least one Run() call is still active.
//
//nolint:funlen
func (s *Runner) Run(ctx context.Context, provider app.Provider) error {
	if provider == nil {
		return errors.New("provider cannot be nil")
	}

	// Get capabilities from manifest
	manifestData, err := s.getManifestData(provider)
	if err != nil {
		return fmt.Errorf("unable to get app manifest capabilities: %w", err)
	}
	appConfig := app.Config{
		KubeConfig:     s.config.KubeConfig,
		ManifestData:   *manifestData,
		SpecificConfig: provider.SpecificConfig(),
	}

	// Create the app
	a, err := provider.NewApp(appConfig)
	if err != nil {
		return err
	}

	s.runningWG.Add(1)
	defer s.runningWG.Done()

	err = func() error {
		s.startMux.Lock()
		defer s.startMux.Unlock()
		if !s.running {
			s.running = true
			go func() {
				s.runningWG.Wait()
				s.running = false
			}()
		}
		return nil
	}()
	if err != nil {
		return err
	}

	// Build the operator
	runner := app.NewMultiRunner()

	// Admission control and custom routes
	if k8s.AppNeedsWebhookHandler(manifestData) {
		if s.webhookServer == nil {
			return errors.New("app has capabilities that require the webhook server (admission, conversion, or custom routes), but no TLS config was provided")
		}
		if err := k8s.ConfigureWebhookHandler(s.webhookServer.server.WebhookHandler, a, manifestData); err != nil {
			return err
		}
		runner.AddRunnable(s.webhookServer)
	}

	// Main loop
	r := a.Runner()
	if r != nil {
		runner.AddRunnable(r)
	}

	// Metrics
	if s.metricsExporter != nil {
		if err := s.metricsExporter.RegisterCollectors(a.PrometheusCollectors()...); err != nil {
			return err
		}

		s.metricsServer.RegisterMetricsHandler(s.metricsExporter.HTTPHandler())
	}

	// Profiles
	if s.config.MetricsConfig.ProfilingEnabled {
		s.metricsServer.RegisterProfilingHandlers()
	}

	// Health
	s.metricsServer.RegisterHealthChecks(s)
	s.metricsServer.RegisterHealthChecks(runner.HealthChecks()...)
	s.metricsServer.RegisterHealthChecks(a.HealthChecks()...)
	runner.AddRunnable(s.metricsServerRunner)

	return runner.Run(ctx)
}

func (s *Runner) HealthCheck(_ context.Context) error {
	if s.running {
		return nil
	}
	return errors.New("app has not started yet")
}

func (*Runner) HealthCheckName() string {
	return "operator-runner"
}

func (s *Runner) getManifestData(provider app.Provider) (*app.ManifestData, error) {
	manifest := provider.Manifest()
	data := app.ManifestData{}
	switch manifest.Location.Type {
	case app.ManifestLocationEmbedded:
		if manifest.ManifestData == nil {
			return nil, errors.New("no ManifestData in Manifest")
		}
		data = *manifest.ManifestData
	case app.ManifestLocationFilePath:
		// TODO: more correct version?
		dir := s.config.Filesystem
		if dir == nil {
			dir = os.DirFS(".")
		}
		contents, err := fs.ReadFile(dir, manifest.Location.Path)
		if err != nil {
			return nil, fmt.Errorf("error reading manifest file from disk (path: %s): %w", manifest.Location.Path, err)
		}
		if err = json.Unmarshal(contents, &data); err != nil {
			return nil, fmt.Errorf("unable to unmarshal manifest data: %w", err)
		}
	case app.ManifestLocationAPIServerResource:
		// TODO: fetch from API server
		return nil, errors.New("apiserver location not supported yet")
	default:
	}
	return &data, nil
}

func newWebhookServerRunner(ws *k8s.WebhookServer) *webhookServerRunner {
	return &webhookServerRunner{
		server: ws,
		runner: app.NewSingletonRunner(&k8sRunnable{
			runner: ws,
		}, false),
	}
}

type webhookServerRunner struct {
	runner *app.SingletonRunner
	server *k8s.WebhookServer
}

func (s *webhookServerRunner) Run(ctx context.Context) error {
	return s.runner.Run(ctx)
}

type k8sRunner interface {
	Run(<-chan struct{}) error
}

type k8sRunnable struct {
	runner k8sRunner
}

func (k *k8sRunnable) Run(ctx context.Context) error {
	return k.runner.Run(ctx.Done())
}
