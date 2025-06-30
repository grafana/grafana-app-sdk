package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/simple"

	"github.com/grafana/issue-tracker-project/pkg/app"
)

func main() {
	// Configure the default logger to use slog
	logging.DefaultLogger = logging.NewSLogLogger(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	//Load the config from the environment
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		logging.DefaultLogger.With("error", err).Error("Unable to load config from environment")
		panic(err)
	}

	// Load the kube config
	kubeConfig, err := LoadInClusterConfig()
	if err != nil {
		logging.DefaultLogger.With("error", err).Error("Unable to load kubernetes configuration")
		panic(err)
	}

	// Set up tracing
	if cfg.OTelConfig.Host != "" {
		simple.SetTraceProvider(simple.OpenTelemetryConfig{
			Host:        cfg.OTelConfig.Host,
			Port:        cfg.OTelConfig.Port,
			ConnType:    simple.OTelConnType(cfg.OTelConfig.ConnType),
			ServiceName: cfg.OTelConfig.ServiceName,
		})
	}

	// Create the operator config and the runner
	operatorConfig := operator.RunnerConfig{
		KubeConfig: kubeConfig.RestConfig,
		MetricsConfig: operator.RunnerMetricsConfig{
			Enabled: true,
		},
	}

	// Only configure webhooks if TLS certificates are provided
	if cfg.WebhookServer.TLSCertPath != "" && cfg.WebhookServer.TLSKeyPath != "" {
		operatorConfig.WebhookConfig = operator.RunnerWebhookConfig{
			Port: cfg.WebhookServer.Port,
			TLSConfig: k8s.TLSConfig{
				CertPath: cfg.WebhookServer.TLSCertPath,
				KeyPath:  cfg.WebhookServer.TLSKeyPath,
			},
		}
		logging.DefaultLogger.Info("Webhook server enabled", "port", cfg.WebhookServer.Port)
	} else {
		logging.DefaultLogger.Info("Webhook server disabled - no TLS certificates provided")
	}
	runner, err := operator.NewRunner(operatorConfig)
	if err != nil {
		logging.DefaultLogger.With("error", err).Error("Unable to create operator runner")
		panic(err)
	}

	// Context and cancel for the operator's Run method
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	// Run
	logging.DefaultLogger.Info("Starting operator")
	err = runner.Run(ctx, app.Provider(cfg))
	if err != nil {
		logging.DefaultLogger.With("error", err).Error("Operator exited with error")
		panic(err)
	}
	logging.DefaultLogger.Info("Normal operator exit")

}
