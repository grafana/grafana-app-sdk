package operator

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// BusInformerMetrics are the prometheus collectors for what only a bus informer has: a live subscription which can
// drop, reconnect and fall behind. Its deliveries are reported on the shared InformerDeliveryMetrics, which it
// carries so that every informer in a process can share one set.
//
// Kinds are told apart by labels, so the collectors are built once and handed to each informer: building them per
// informer would register the same metric names more than once.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusInformerMetrics struct {
	delivery *InformerDeliveryMetrics

	reconnects       *prometheus.CounterVec
	liveSubscription *prometheus.GaugeVec
	droppedEvents    *prometheus.CounterVec
	supersededEvents *prometheus.CounterVec
}

// BusInformerMetricsConfig configures the bus informer metrics.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusInformerMetricsConfig struct {
	// MetricsConfig controls the namespace and native histogram settings of the collectors.
	MetricsConfig metrics.Config
	// ResourceVersionTime decodes the timestamp a resource version embeds, for the delivery latency histograms.
	// It is ignored when Delivery is set, which carries its own. See InformerDeliveryMetricsConfig.
	ResourceVersionTime func(int64) time.Time
	// Delivery are the shared delivery collectors the informers report their events on. Set it to the same instance
	// the process's watch-backed and periodic informers use, so that a kind which moves between informers keeps its
	// series. When nil, one set is built from MetricsConfig and ResourceVersionTime.
	Delivery *InformerDeliveryMetrics
}

// NewBusInformerMetrics builds the metrics for every bus informer in a process.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewBusInformerMetrics(cfg BusInformerMetricsConfig) *BusInformerMetrics {
	delivery := cfg.Delivery
	if delivery == nil {
		delivery = NewInformerDeliveryMetrics(InformerDeliveryMetricsConfig{
			MetricsConfig:       cfg.MetricsConfig,
			ResourceVersionTime: cfg.ResourceVersionTime,
		})
	}
	return &BusInformerMetrics{
		delivery: delivery,
		reconnects: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_bus_reconnects_total",
			Help:      "Times an informer's live subscription was (re)established after a gap. Events published during the gap never reach this replica; the informer forces a re-list to recover them.",
		}, []string{"group", "version", "kind"}),
		liveSubscription: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_bus_live_subscription",
			Help:      "Whether an informer currently holds an open live subscription (1) or is running re-list-only (0), before its subscription first opens or in degraded-start mode.",
		}, []string{"group", "version", "kind"}),
		droppedEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_bus_dropped_events_total",
			Help:      "Live events dropped because the informer's queue was full, meaning it is not applying events as fast as the bus delivers them. Each one forces a re-list, so the change is recovered rather than lost, at the cost of a full list.",
		}, []string{"group", "version", "kind"}),
		supersededEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_bus_superseded_events_total",
			Help:      "Live events dropped because a re-list completed while they waited in the queue and read a newer version of the resource, so the snapshot already accounts for them. Expected in small numbers under load; sustained growth means events are queued for longer than the re-list cadence.",
		}, []string{"group", "version", "kind"}),
	}
}

// Delivery returns the shared delivery collectors these metrics report events on, for handing to the process's
// other informers. A nil BusInformerMetrics returns nil, which every consumer treats as "discard".
func (m *BusInformerMetrics) Delivery() *InformerDeliveryMetrics {
	if m == nil {
		return nil
	}
	return m.delivery
}

// PrometheusCollectors implements metrics.Provider. It includes the shared delivery collectors, which another
// informer may already have registered: registering an already-registered collector is expected here and tolerated.
func (m *BusInformerMetrics) PrometheusCollectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return append(m.delivery.PrometheusCollectors(),
		m.reconnects, m.liveSubscription, m.droppedEvents, m.supersededEvents)
}

// RelistRecorder binds the re-list metrics to one kind, for a ResourceLister to record what it issued and what the
// server served. A nil BusInformerMetrics returns a recorder which discards.
func (m *BusInformerMetrics) RelistRecorder(kind resource.Kind) RelistRecorder {
	return m.Delivery().RelistRecorder(kind)
}
