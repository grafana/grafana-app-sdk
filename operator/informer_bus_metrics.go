package operator

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// Verb label values for the bus informer's event metrics.
const (
	busVerbAdd    = "add"
	busVerbUpdate = "update"
	busVerbDelete = "delete"
)

// ListProjection is what a server returned for a re-list.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type ListProjection string

const (
	// ProjectionIdentities is a projection carrying namespace, name and resource version only.
	ProjectionIdentities ListProjection = "identities"
	// ProjectionObjects is a full list of objects.
	ProjectionObjects ListProjection = "objects"
)

// BusInformerMetrics are the prometheus collectors shared by every BusInformer and PeriodicInformer in a process.
// Kinds are told apart by labels, so the collectors are built once and handed to each informer: building them per
// informer would register the same metric names more than once.
//
// Live events and re-list events are separate families on purpose. A live event is a delivery; a re-list add or
// delete is a change the live stream did not deliver to this replica, so counting them together would hide exactly
// the thing the pair is there to measure.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusInformerMetrics struct {
	liveEvents       *prometheus.CounterVec
	relistEvents     *prometheus.CounterVec
	liveLatency      *prometheus.HistogramVec
	relistLatency    *prometheus.HistogramVec
	reconnects       *prometheus.CounterVec
	liveSubscription *prometheus.GaugeVec
	relistRequests   *prometheus.CounterVec
	relistProjection *prometheus.CounterVec
	droppedEvents    *prometheus.CounterVec

	resourceVersionTime func(int64) time.Time
}

// BusInformerMetricsConfig configures the bus informer metrics.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusInformerMetricsConfig struct {
	// MetricsConfig controls the namespace and native histogram settings of the collectors.
	MetricsConfig metrics.Config
	// ResourceVersionTime decodes the timestamp a resource version embeds, which is what dates a change for the
	// latency histograms. Resource versions are opaque in the kubernetes API contract, so there is no portable
	// decoding: when this is nil, or the version is not numeric, no latency is observed and the counters are
	// unaffected.
	ResourceVersionTime func(int64) time.Time
}

// NewBusInformerMetrics builds the delivery metrics for every bus informer in a process.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewBusInformerMetrics(cfg BusInformerMetricsConfig) *BusInformerMetrics {
	histogram := func(name, help string) prometheus.HistogramOpts {
		return prometheus.HistogramOpts{
			Namespace:                      cfg.MetricsConfig.Namespace,
			Name:                           name,
			Help:                           help,
			Buckets:                        metrics.LatencyBuckets,
			NativeHistogramBucketFactor:    cfg.MetricsConfig.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber: cfg.MetricsConfig.NativeHistogramMaxBucketNumber,
		}
	}
	return &BusInformerMetrics{
		liveEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_bus_live_events_total",
			Help:      "Events delivered to informer handlers from the live subscription, by group, version, kind and verb.",
		}, []string{"group", "version", "kind", "verb"}),
		relistEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_bus_relist_events_total",
			Help:      "Events delivered to informer handlers by the periodic re-list, by group, version, kind and verb. Adds and deletes are changes the live stream did not deliver here; updates are re-deliveries of unchanged objects.",
		}, []string{"group", "version", "kind", "verb"}),
		liveLatency: prometheus.NewHistogramVec(histogram(
			"informer_bus_live_event_latency_seconds",
			"Time from a change's resource version being issued to its live event reaching the informer handlers.",
		), []string{"group", "version", "kind"}),
		relistLatency: prometheus.NewHistogramVec(histogram(
			"informer_bus_relist_event_latency_seconds",
			"Time from a change's resource version being issued to its recovery by a re-list, for adds the live stream did not deliver. The initial list and re-deliveries of unchanged objects carry no latency.",
		), []string{"group", "version", "kind"}),
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
		relistRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_bus_relist_requests_total",
			Help:      "List requests the re-list issued, one per page followed. Divided by the number of re-lists it is the average page count per snapshot; a rising ratio means the kind is outgrowing a single page.",
		}, []string{"group", "version", "kind"}),
		droppedEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_bus_dropped_events_total",
			Help:      "Live events dropped because the informer's queue was full, meaning it is not applying events as fast as the bus delivers them. Each one forces a re-list, so the change is recovered rather than lost, at the cost of a full list.",
		}, []string{"group", "version", "kind"}),
		relistProjection: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_bus_relist_projection_total",
			Help:      "Re-lists by what the server returned: identities is a projection carrying namespace, name and resource version only, objects is a full list. Shows whether an identity projection took effect, and the fallback when a server does not serve one.",
		}, []string{"group", "version", "kind", "projection"}),
		resourceVersionTime: cfg.ResourceVersionTime,
	}
}

// PrometheusCollectors implements metrics.Provider.
func (m *BusInformerMetrics) PrometheusCollectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return []prometheus.Collector{
		m.liveEvents, m.relistEvents, m.liveLatency, m.relistLatency,
		m.reconnects, m.liveSubscription, m.relistRequests, m.relistProjection, m.droppedEvents,
	}
}

// RelistRecorder binds the re-list metrics to one kind, for a ResourceLister to record what it issued and what the
// server served. A nil BusInformerMetrics returns a recorder which discards.
func (m *BusInformerMetrics) RelistRecorder(kind resource.Kind) RelistRecorder {
	if m == nil {
		return RelistRecorder{}
	}
	gvk := kind.GroupVersionKind()
	return RelistRecorder{
		requests:   m.relistRequests.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind),
		identities: m.relistProjection.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind, string(ProjectionIdentities)),
		objects:    m.relistProjection.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind, string(ProjectionObjects)),
	}
}

// RelistRecorder records one kind's re-list outcomes. Its zero value discards, so a lister needs no nil checks.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type RelistRecorder struct {
	requests   prometheus.Counter
	identities prometheus.Counter
	objects    prometheus.Counter
}

// Request records one list request, which for a paginated list is one page.
func (r RelistRecorder) Request() {
	if r.requests != nil {
		r.requests.Inc()
	}
}

// Projection records which projection served one re-list.
func (r RelistRecorder) Projection(projection ListProjection) {
	counter := r.objects
	if projection == ProjectionIdentities {
		counter = r.identities
	}
	if counter != nil {
		counter.Inc()
	}
}

// kindMetrics binds the delivery metrics to one kind. Its zero value discards.
type kindMetrics struct {
	metrics *BusInformerMetrics
	group   string
	version string
	kind    string
}

// newKindMetrics resolves one kind's label values once.
func newKindMetrics(collectors *BusInformerMetrics, kind resource.Kind) kindMetrics {
	gvk := kind.GroupVersionKind()
	return kindMetrics{metrics: collectors, group: gvk.Group, version: gvk.Version, kind: gvk.Kind}
}

func (k kindMetrics) observeLiveEvent(verb string, rv int64) {
	if k.metrics == nil {
		return
	}
	k.metrics.liveEvents.WithLabelValues(k.group, k.version, k.kind, verb).Inc()
	k.observeLatency(k.metrics.liveLatency, rv)
}

func (k kindMetrics) observeRelistEvent(verb string, rv int64) {
	if k.metrics == nil {
		return
	}
	k.metrics.relistEvents.WithLabelValues(k.group, k.version, k.kind, verb).Inc()
	k.observeLatency(k.metrics.relistLatency, rv)
}

func (k kindMetrics) observeDroppedEvent() {
	if k.metrics == nil {
		return
	}
	k.metrics.droppedEvents.WithLabelValues(k.group, k.version, k.kind).Inc()
}

func (k kindMetrics) observeReconnect() {
	if k.metrics == nil {
		return
	}
	k.metrics.reconnects.WithLabelValues(k.group, k.version, k.kind).Inc()
}

func (k kindMetrics) observeSubscriptionOpen() { k.setLiveSubscription(1) }

func (k kindMetrics) observeSubscriptionClosed() { k.setLiveSubscription(0) }

func (k kindMetrics) setLiveSubscription(value float64) {
	if k.metrics == nil {
		return
	}
	k.metrics.liveSubscription.WithLabelValues(k.group, k.version, k.kind).Set(value)
}

func (k kindMetrics) observeLatency(latency *prometheus.HistogramVec, rv int64) {
	if rv <= 0 || k.metrics.resourceVersionTime == nil {
		return
	}
	// A negative result is clock skew, not delivery.
	if seconds := time.Since(k.metrics.resourceVersionTime(rv)).Seconds(); seconds > 0 {
		latency.WithLabelValues(k.group, k.version, k.kind).Observe(seconds)
	}
}
