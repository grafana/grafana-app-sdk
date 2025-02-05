package metrics

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// LatencyBuckets is the collection of buckets to use for latency/request duration histograms.
	// If you wish to change this, this should be changed _before_ creating any objects which provide latency histograms,
	// as this variable is used at collector creation time.
	LatencyBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 25, 50, 100}
)

// NewExporter returns a new Exporter using the provided config.
// Empty values in the config apply default prometheus values.
func NewExporter(mux *http.ServeMux, cfg ExporterConfig) (*Exporter, error) {
	if mux == nil {
		return nil, errors.New("mux cannot be nil")
	}

	if cfg.Registerer == nil {
		cfg.Registerer = prometheus.DefaultRegisterer
	}
	if cfg.Gatherer == nil {
		cfg.Gatherer = prometheus.DefaultGatherer
	}
	return &Exporter{
		Registerer: cfg.Registerer,
		Gatherer:   cfg.Gatherer,
		Mux:        mux,
	}, nil
}

// Provider is an interface which describes any object which can provide the prometheus Collectors is uses for registration
type Provider interface {
	PrometheusCollectors() []prometheus.Collector
}

// Exporter exports prometheus metrics
type Exporter struct {
	Registerer prometheus.Registerer
	Gatherer   prometheus.Gatherer
	Mux        *http.ServeMux
}

// RegisterCollectors registers the provided collectors with the Exporter's Registerer.
// If there is an error registering any of the provided collectors, registration is halted and an error is returned.
func (e *Exporter) RegisterCollectors(metrics ...prometheus.Collector) error {
	for _, m := range metrics {
		err := e.Registerer.Register(m)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Exporter) RegisterMetricsHandler() {
	e.Mux.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("true"))
	}))

	/*
		promhttp.InstrumentMetricHandler(
				e.Registerer, promhttp.HandlerFor(e.Gatherer, promhttp.HandlerOpts{}),
			)
	*/
}
