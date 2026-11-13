package metrics

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/grafana/grafana-app-sdk/logging"
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
			if _, ok := errors.AsType[prometheus.AlreadyRegisteredError](err); ok {
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

// Register registers c with reg and returns it, or returns the equivalent collector already registered there.
//
// It is for collectors more than one component may build with the same descriptors: prometheus rejects the second
// registration, and a component which ignores that keeps updating a collector nothing exports — which looks like a
// metric that stopped rather than a wiring mistake. Returning the registered one instead means every component
// updates the series actually scraped.
//
// A nil Registerer returns c unregistered, which is the SDK's default: a component exposes its collectors through
// Provider and the app's Exporter registers them.
func Register[T prometheus.Collector](reg prometheus.Registerer, c T) T {
	if reg == nil {
		return c
	}
	err := reg.Register(c)
	if err == nil {
		return c
	}
	if already, ok := errors.AsType[prometheus.AlreadyRegisteredError](err); ok {
		if existing, ok := already.ExistingCollector.(T); ok {
			return existing
		}
	}
	// Anything else is a descriptor conflict rather than a duplicate, which no caller can act on from here. The
	// collector is still returned, so the component works and exports nothing, rather than taking the process down.
	logging.DefaultLogger.Error("could not register a prometheus collector", "error", err)
	return c
}
