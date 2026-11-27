package operator

import (
	"context"
	"slices"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// InformerEventMetricsConfig controls the shared informer event collectors.
type InformerEventMetricsConfig struct {
	Namespace string
	// ResourceVersionTime decodes a provider's resource version. Return a
	// zero time for a version without a timestamp.
	ResourceVersionTime func(string) time.Time
}

// InformerEventMetrics collects events from bus and periodic LIST informers.
// Construct one per application and pass it to every informer in that app.
type InformerEventMetrics struct {
	collectors    []prometheus.Collector
	deliveries    *prometheus.CounterVec
	drops         *prometheus.CounterVec
	reconnects    *prometheus.CounterVec
	ready         *prometheus.GaugeVec
	lists         *prometheus.CounterVec
	pages         *prometheus.CounterVec
	latency       *prometheus.HistogramVec
	decodeVersion func(string) time.Time
}

// NewInformerEventMetrics creates a collector set for the caller's registry.
func NewInformerEventMetrics(cfg InformerEventMetricsConfig) *InformerEventMetrics {
	kind := []string{"group", "version", "kind"}
	counter := func(name, help string, labels ...string) *prometheus.CounterVec {
		return prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: cfg.Namespace, Name: name, Help: help},
			append(slices.Clone(kind), labels...))
	}
	m := &InformerEventMetrics{
		deliveries: counter("informer_event_deliveries_total", "Informer events presented to handlers by source and action.", "source", "action"),
		drops:      counter("informer_bus_events_dropped_total", "Bus notifications discarded by the informer.", "reason"),
		reconnects: counter("informer_bus_reconnects_total", "Bus subscriptions that reported a reconnect."),
		ready: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: cfg.Namespace, Name: "informer_bus_ready_subscriptions",
			Help: "Open subscription handles that completed readiness."}, kind),
		lists:         counter("informer_list_attempts_total", "Complete LIST attempts by outcome.", "outcome"),
		pages:         counter("informer_list_page_requests_total", "Individual page requests from listers that expose paging."),
		decodeVersion: cfg.ResourceVersionTime,
	}
	m.collectors = []prometheus.Collector{m.deliveries, m.drops, m.reconnects, m.ready, m.lists, m.pages}
	if m.decodeVersion != nil {
		m.latency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.Namespace, Name: "informer_event_delivery_latency_seconds",
			Help:    "Time from a published resource version to delivery. LIST samples are for objects first seen after startup.",
			Buckets: metrics.LatencyBuckets,
		}, append(slices.Clone(kind), "source"))
		m.collectors = append(m.collectors, m.latency)
	}
	return m
}

// PrometheusCollectors returns the shared set for registration with Prometheus.
func (m *InformerEventMetrics) PrometheusCollectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return slices.Clone(m.collectors)
}

func metricKindLabels(kind resource.Kind) []string {
	return []string{kind.Group(), kind.Version(), kind.Kind()}
}

type informerEventRecorder struct {
	metrics *InformerEventMetrics
	labels  []string
}

func eventRecorder(m *InformerEventMetrics, kind resource.Kind) informerEventRecorder {
	if m == nil {
		return informerEventRecorder{}
	}
	return informerEventRecorder{metrics: m, labels: metricKindLabels(kind)}
}

func (r informerEventRecorder) delivery(source Delivery, action, version string) {
	if r.metrics == nil {
		return
	}
	r.metrics.deliveries.WithLabelValues(r.labels[0], r.labels[1], r.labels[2], string(source), action).Inc()
	if r.metrics.decodeVersion == nil || version == "" {
		return
	}
	if at := r.metrics.decodeVersion(version); !at.IsZero() && !at.After(time.Now()) {
		r.metrics.latency.WithLabelValues(r.labels[0], r.labels[1], r.labels[2], string(source)).Observe(time.Since(at).Seconds())
	}
}

func (r informerEventRecorder) drop(reason string) {
	if r.metrics != nil {
		r.metrics.drops.WithLabelValues(r.labels[0], r.labels[1], r.labels[2], reason).Inc()
	}
}

func (r informerEventRecorder) reconnect() {
	if r.metrics != nil {
		r.metrics.reconnects.WithLabelValues(r.labels...).Inc()
	}
}

func (r informerEventRecorder) ready(delta float64) {
	if r.metrics != nil {
		r.metrics.ready.WithLabelValues(r.labels...).Add(delta)
	}
}

func (r informerEventRecorder) listOutcome(err error) {
	if r.metrics == nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	r.metrics.lists.WithLabelValues(r.labels[0], r.labels[1], r.labels[2], outcome).Inc()
}

type listMetricsContextKey struct{}

func withListMetrics(ctx context.Context, m *InformerEventMetrics, kind resource.Kind) context.Context {
	if m == nil {
		return ctx
	}
	return context.WithValue(ctx, listMetricsContextKey{}, eventRecorder(m, kind))
}

// RecordInformerListPage counts one request from a paginated ResourceLister or
// ResourceKeyLister. Call it at the page request boundary, including failures.
func RecordInformerListPage(ctx context.Context) {
	if r, ok := ctx.Value(listMetricsContextKey{}).(informerEventRecorder); ok {
		r.metrics.pages.WithLabelValues(r.labels...).Inc()
	}
}
