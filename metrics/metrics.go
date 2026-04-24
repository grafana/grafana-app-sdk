package metrics

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// LatencyBuckets is the collection of buckets to use for latency/request duration histograms.
	// If you wish to change this, this should be changed _before_ creating any objects which provide latency histograms,
	// as this variable is used at collector creation time.
	LatencyBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 25, 50, 100}
)

// NewExporter returns a new Exporter using the provided config.
// Empty values in the config apply default prometheus values.
func NewExporter(cfg ExporterConfig) *Exporter {
	if cfg.Registerer == nil {
		cfg.Registerer = prometheus.DefaultRegisterer
	}
	if cfg.Gatherer == nil {
		cfg.Gatherer = prometheus.DefaultGatherer
	}
	return &Exporter{
		Registerer: cfg.Registerer,
		Gatherer:   cfg.Gatherer,
	}
}

// Provider is an interface which describes any object which can provide the prometheus Collectors is uses for registration
type Provider interface {
	PrometheusCollectors() []prometheus.Collector
}

// Exporter exports prometheus metrics
type Exporter struct {
	Registerer prometheus.Registerer
	Gatherer   prometheus.Gatherer
}

// RegisterCollectors registers the provided collectors with the Exporter's Registerer.
// Already-registered collectors are silently skipped.
func (e *Exporter) RegisterCollectors(metrics ...prometheus.Collector) error {
	for _, m := range metrics {
		if err := e.Registerer.Register(m); err != nil {
			var already prometheus.AlreadyRegisteredError
			if errors.As(err, &already) {
				continue
			}
			return err
		}
	}
	return nil
}

func (e *Exporter) HTTPHandler() http.Handler {
	return promhttp.InstrumentMetricHandler(
		e.Registerer, promhttp.HandlerFor(e.Gatherer, promhttp.HandlerOpts{}),
	)
}
