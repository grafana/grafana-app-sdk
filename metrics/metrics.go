package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

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
	if cfg.Port <= 0 {
		cfg.Port = 9090
	}
	return &Exporter{
		Registerer: cfg.Registerer,
		Gatherer:   cfg.Gatherer,
		Port:       cfg.Port,
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
	Port       int
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

// Run creates an HTTP server which exposes a /metrics endpoint on the configured port (if <=0, uses the default 9090)
func (e *Exporter) Run(stopCh <-chan struct{}) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.InstrumentMetricHandler(
		e.Registerer, promhttp.HandlerFor(e.Gatherer, promhttp.HandlerOpts{}),
	))
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", e.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	go func() {
		for range stopCh {
			// do nothing until closeCh is closed or receives a message
			break
		}
		ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second)
		defer cancelFunc()
		errCh <- server.Shutdown(ctx)
	}()
	err := <-errCh
	return err
}
