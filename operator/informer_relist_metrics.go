package operator

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// Verb label values for the relist informer's event metrics.
const (
	relistVerbAdd    = "add"
	relistVerbUpdate = "update"
	relistVerbDelete = "delete"
)

// ListProjection is what a server returned for a re-list.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type ListProjection string

const (
	// ProjectionIdentities is a projection carrying namespace, name and resource version only.
	ProjectionIdentities ListProjection = "identities"
	// ProjectionObjects is a full list of objects.
	ProjectionObjects ListProjection = "objects"
)

// RelistInformerMetrics are the prometheus collectors shared by every RelistInformer in a process.
// Kinds are told apart by labels, so the collectors are built once and handed to each informer: building them per
// informer would register the same metric names more than once.
//
// Live events and re-list events are separate families on purpose. A live event is a delivery; a re-list add or
// delete is a change the live stream did not deliver to this replica, so counting them together would hide exactly
// the thing the pair is there to measure.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type RelistInformerMetrics struct {
	liveEvents       *prometheus.CounterVec
	relistEvents     *prometheus.CounterVec
	liveLatency      *prometheus.HistogramVec
	relistLatency    *prometheus.HistogramVec
	rewatches        *prometheus.CounterVec
	liveWatch        *prometheus.GaugeVec
	relistRequests   *prometheus.CounterVec
	relistProjection *prometheus.CounterVec

	resourceVersionTime func(int64) time.Time
}

// RelistInformerMetricsConfig configures the relist informer metrics.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type RelistInformerMetricsConfig struct {
	// MetricsConfig controls the namespace and native histogram settings of the collectors.
	MetricsConfig metrics.Config
	// ResourceVersionTime decodes the timestamp a resource version embeds, which is what dates a change for the
	// latency histograms. Resource versions are opaque in the kubernetes API contract, so there is no portable
	// decoding: when this is nil, or the version is not numeric, no latency is observed and the counters are
	// unaffected.
	ResourceVersionTime func(int64) time.Time
}

// NewRelistInformerMetrics builds the delivery metrics for every relist informer in a process.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
func NewRelistInformerMetrics(cfg RelistInformerMetricsConfig) *RelistInformerMetrics {
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
	return &RelistInformerMetrics{
		liveEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "relist_informer_live_events_total",
			Help:      "Events delivered to informer handlers from the live watch, by kind and verb.",
		}, []string{"kind", "verb"}),
		relistEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "relist_informer_relist_events_total",
			Help:      "Events delivered to informer handlers by the periodic re-list, by kind and verb. Adds and deletes are changes the live stream did not deliver here; updates are re-deliveries of unchanged objects.",
		}, []string{"kind", "verb"}),
		liveLatency: prometheus.NewHistogramVec(histogram(
			"relist_informer_live_event_latency_seconds",
			"Time from a change's resource version being issued to its live event reaching the informer handlers.",
		), []string{"kind"}),
		relistLatency: prometheus.NewHistogramVec(histogram(
			"relist_informer_relist_event_latency_seconds",
			"Time from a change's resource version being issued to its recovery by a re-list, for adds the live stream did not deliver. The initial list and re-deliveries of unchanged objects carry no latency.",
		), []string{"kind"}),
		rewatches: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "relist_informer_rewatches_total",
			Help:      "Times an informer's live watch was (re)opened after a gap. Changes made during the gap never reach this replica live; the informer forces a re-list to recover them.",
		}, []string{"kind"}),
		liveWatch: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "relist_informer_live_watch",
			Help:      "Whether an informer currently holds an open live watch (1) or is running re-list-only (0): before its watch first opens, between watches, or in degraded-start mode.",
		}, []string{"kind"}),
		relistRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "relist_informer_list_requests_total",
			Help:      "List requests the re-list issued, one per page followed. Divided by the number of re-lists it is the average page count per snapshot; a rising ratio means the kind is outgrowing a single page.",
		}, []string{"kind"}),
		relistProjection: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "relist_informer_list_projection_total",
			Help:      "Re-lists by what the server returned: identities is a projection carrying namespace, name and resource version only, objects is a full list. Shows whether an identity projection took effect, and the fallback when a server does not serve one.",
		}, []string{"kind", "projection"}),
		resourceVersionTime: cfg.ResourceVersionTime,
	}
}

// PrometheusCollectors implements metrics.Provider.
func (m *RelistInformerMetrics) PrometheusCollectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return []prometheus.Collector{
		m.liveEvents, m.relistEvents, m.liveLatency, m.relistLatency,
		m.rewatches, m.liveWatch, m.relistRequests, m.relistProjection,
	}
}

// RelistRecorder binds the re-list metrics to one kind, for a ResourceLister to record what it issued and what the
// server served. A nil RelistInformerMetrics returns a recorder which discards.
func (m *RelistInformerMetrics) RelistRecorder(kind resource.Kind) RelistRecorder {
	if m == nil {
		return RelistRecorder{}
	}
	label := kind.GroupVersionKind().String()
	return RelistRecorder{
		requests:   m.relistRequests.WithLabelValues(label),
		identities: m.relistProjection.WithLabelValues(label, string(ProjectionIdentities)),
		objects:    m.relistProjection.WithLabelValues(label, string(ProjectionObjects)),
	}
}

// RelistRecorder records one kind's re-list outcomes. Its zero value discards, so a lister needs no nil checks.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
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
	metrics *RelistInformerMetrics
	kind    string
}

func (k kindMetrics) observeLiveEvent(verb string, rv int64) {
	if k.metrics == nil {
		return
	}
	k.metrics.liveEvents.WithLabelValues(k.kind, verb).Inc()
	k.observeLatency(k.metrics.liveLatency, rv)
}

func (k kindMetrics) observeRelistEvent(verb string, rv int64) {
	if k.metrics == nil {
		return
	}
	k.metrics.relistEvents.WithLabelValues(k.kind, verb).Inc()
	k.observeLatency(k.metrics.relistLatency, rv)
}

func (k kindMetrics) observeRewatch() {
	if k.metrics == nil {
		return
	}
	k.metrics.rewatches.WithLabelValues(k.kind).Inc()
}

func (k kindMetrics) observeWatchOpen() { k.setLiveWatch(1) }

func (k kindMetrics) observeWatchClosed() { k.setLiveWatch(0) }

func (k kindMetrics) setLiveWatch(value float64) {
	if k.metrics == nil {
		return
	}
	k.metrics.liveWatch.WithLabelValues(k.kind).Set(value)
}

func (k kindMetrics) observeLatency(latency *prometheus.HistogramVec, rv int64) {
	if rv <= 0 || k.metrics.resourceVersionTime == nil {
		return
	}
	// A negative result is clock skew, not delivery.
	if seconds := time.Since(k.metrics.resourceVersionTime(rv)).Seconds(); seconds > 0 {
		latency.WithLabelValues(k.kind).Observe(seconds)
	}
}
