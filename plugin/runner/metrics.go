package runner

import (
	"context"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func NewMetricsServerRunner(exporter *metrics.Exporter) *MetricsServerRunner {
	return &MetricsServerRunner{
		server: exporter,
		runner: app.NewSingletonRunner(&k8sRunnable{
			runner: exporter,
		}, false),
	}
}

type MetricsServerRunner struct {
	runner *app.SingletonRunner
	server *metrics.Exporter
}

func (m *MetricsServerRunner) Run(ctx context.Context) error {
	return m.runner.Run(ctx)
}

func (m *MetricsServerRunner) RegisterCollectors(collectors ...prometheus.Collector) error {
	return m.server.RegisterCollectors(collectors...)
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
