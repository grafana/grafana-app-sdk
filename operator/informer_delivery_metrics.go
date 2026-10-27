package operator

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// Verb label values for the informer delivery metrics.
const (
	deliveryVerbAdd    = "add"
	deliveryVerbUpdate = "update"
	deliveryVerbDelete = "delete"
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

// InformerDeliveryMetrics are the prometheus collectors every informer in a process reports its deliveries on,
// whatever its source:
//
//	informer_live_events_total{group,version,kind,verb}
//	informer_relist_events_total{group,version,kind,verb}
//	informer_live_event_latency_seconds{group,version,kind}
//	informer_relist_event_latency_seconds{group,version,kind}
//	informer_relist_requests_total{group,version,kind}
//	informer_relist_projection_total{group,version,kind,projection}
//
// Kinds are told apart by labels, so the collectors are built once and handed to each informer: building them per
// informer would register the same metric names more than once. Sharing one instance across the bus, periodic and
// watch-backed informers is also what keeps the series stable when a kind moves between them — a migration shows up
// as a shift between the live and re-list families, not as one set of series disappearing and another appearing.
//
// Live events and re-list events are separate families on purpose. A live event is a delivery; a re-list add or
// delete is a change the live stream did not deliver to this replica, so counting them together would hide exactly
// the thing the pair is there to measure.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type InformerDeliveryMetrics struct {
	liveEvents       *prometheus.CounterVec
	relistEvents     *prometheus.CounterVec
	liveLatency      *prometheus.HistogramVec
	relistLatency    *prometheus.HistogramVec
	relistRequests   *prometheus.CounterVec
	relistProjection *prometheus.CounterVec

	resourceVersionTime func(int64) time.Time
}

// InformerDeliveryMetricsConfig configures the informer delivery metrics.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type InformerDeliveryMetricsConfig struct {
	// MetricsConfig controls the namespace and native histogram settings of the collectors.
	MetricsConfig metrics.Config
	// ResourceVersionTime decodes the timestamp a resource version embeds, which is what dates a change for the
	// latency histograms. Resource versions are opaque in the kubernetes API contract, so there is no portable
	// decoding: when this is nil, or the version is not numeric, no latency is observed and the counters are
	// unaffected.
	ResourceVersionTime func(int64) time.Time
}

// NewInformerDeliveryMetrics builds the delivery metrics for every informer in a process.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewInformerDeliveryMetrics(cfg InformerDeliveryMetricsConfig) *InformerDeliveryMetrics {
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
	return &InformerDeliveryMetrics{
		liveEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_live_events_total",
			Help:      "Events delivered to informer handlers from the live stream — a change bus subscription or a watch — by group, version, kind and verb.",
		}, []string{"group", "version", "kind", "verb"}),
		relistEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_relist_events_total",
			Help:      "Events delivered to informer handlers by a list rather than the live stream, by group, version, kind and verb. Adds and deletes are changes the live stream did not deliver here; updates are re-deliveries of unchanged objects.",
		}, []string{"group", "version", "kind", "verb"}),
		liveLatency: prometheus.NewHistogramVec(histogram(
			"informer_live_event_latency_seconds",
			"Time from a change's resource version being issued to its live event reaching the informer handlers.",
		), []string{"group", "version", "kind"}),
		relistLatency: prometheus.NewHistogramVec(histogram(
			"informer_relist_event_latency_seconds",
			"Time from a change's resource version being issued to its recovery by a re-list, for adds the live stream did not deliver. The initial list and re-deliveries of unchanged objects carry no latency.",
		), []string{"group", "version", "kind"}),
		relistRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_relist_requests_total",
			Help:      "List requests a re-list issued, one per page followed. Divided by the number of re-lists it is the average page count per snapshot; a rising ratio means the kind is outgrowing a single page.",
		}, []string{"group", "version", "kind"}),
		relistProjection: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_relist_projection_total",
			Help:      "Re-lists by what the server returned: identities is a projection carrying namespace, name and resource version only, objects is a full list. Shows whether an identity projection took effect, and the fallback when a server does not serve one.",
		}, []string{"group", "version", "kind", "projection"}),
		resourceVersionTime: cfg.ResourceVersionTime,
	}
}

// PrometheusCollectors implements metrics.Provider.
func (m *InformerDeliveryMetrics) PrometheusCollectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return []prometheus.Collector{
		m.liveEvents, m.relistEvents, m.liveLatency, m.relistLatency, m.relistRequests, m.relistProjection,
	}
}

// RelistRecorder binds the re-list metrics to one kind, for a ResourceLister to record what it issued and what the
// server served. A nil InformerDeliveryMetrics returns a recorder which discards.
func (m *InformerDeliveryMetrics) RelistRecorder(kind resource.Kind) RelistRecorder {
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

// kindMetrics binds an informer's metrics to one kind, resolving the label values once. Either set of collectors
// may be absent — a watch-backed informer has no bus collectors at all — so each observation checks its own.
type kindMetrics struct {
	delivery *InformerDeliveryMetrics
	bus      *BusInformerMetrics
	group    string
	version  string
	kind     string
}

// newKindMetrics binds a bus informer's metrics, which carry the shared delivery collectors with them.
func newKindMetrics(bus *BusInformerMetrics, kind resource.Kind) kindMetrics {
	k := newDeliveryKindMetrics(bus.Delivery(), kind)
	k.bus = bus
	return k
}

// newDeliveryKindMetrics binds the delivery collectors alone, for an informer with no bus-specific metrics.
func newDeliveryKindMetrics(delivery *InformerDeliveryMetrics, kind resource.Kind) kindMetrics {
	gvk := kind.GroupVersionKind()
	return kindMetrics{delivery: delivery, group: gvk.Group, version: gvk.Version, kind: gvk.Kind}
}

func (k kindMetrics) observeLiveEvent(verb string, rv int64) {
	if k.delivery == nil {
		return
	}
	k.delivery.liveEvents.WithLabelValues(k.group, k.version, k.kind, verb).Inc()
	k.observeLatency(k.delivery.liveLatency, rv)
}

func (k kindMetrics) observeRelistEvent(verb string, rv int64) {
	if k.delivery == nil {
		return
	}
	k.delivery.relistEvents.WithLabelValues(k.group, k.version, k.kind, verb).Inc()
	k.observeLatency(k.delivery.relistLatency, rv)
}

func (k kindMetrics) observeLatency(latency *prometheus.HistogramVec, rv int64) {
	if rv <= 0 || k.delivery.resourceVersionTime == nil {
		return
	}
	// A negative result is clock skew, not delivery.
	if seconds := time.Since(k.delivery.resourceVersionTime(rv)).Seconds(); seconds > 0 {
		latency.WithLabelValues(k.group, k.version, k.kind).Observe(seconds)
	}
}

func (k kindMetrics) observeDroppedEvent() {
	if k.bus == nil {
		return
	}
	k.bus.droppedEvents.WithLabelValues(k.group, k.version, k.kind).Inc()
}

func (k kindMetrics) observeSupersededEvent() {
	if k.bus == nil {
		return
	}
	k.bus.supersededEvents.WithLabelValues(k.group, k.version, k.kind).Inc()
}

func (k kindMetrics) observeReconnect() {
	if k.bus == nil {
		return
	}
	k.bus.reconnects.WithLabelValues(k.group, k.version, k.kind).Inc()
}

func (k kindMetrics) observeSubscriptionOpen() { k.setLiveSubscription(1) }

func (k kindMetrics) observeSubscriptionClosed() { k.setLiveSubscription(0) }

func (k kindMetrics) setLiveSubscription(value float64) {
	if k.bus == nil {
		return
	}
	k.bus.liveSubscription.WithLabelValues(k.group, k.version, k.kind).Set(value)
}

// deliveryMeter reports a client-go SharedIndexInformer's deliveries on the shared delivery metrics. The informer
// draws no live/re-list distinction of its own, so the meter derives it: initial-list adds, resync re-deliveries
// (the same version on both sides of an update) and re-list-detected deletes (a DeletedFinalStateUnknown tombstone)
// are re-list deliveries, and everything else came from the watch.
//
// Register it as one extra event handler, so each event is counted once however many handlers the informer has.
type deliveryMeter struct {
	metrics kindMetrics
}

var _ cache.ResourceEventHandler = deliveryMeter{}

// nolint:revive // isInInitialList is client-go's cache.ResourceEventHandler signature.
func (h deliveryMeter) OnAdd(obj any, isInInitialList bool) {
	if isInInitialList {
		// An initial-list object may be arbitrarily old, so there is no delay to attribute to delivery.
		h.metrics.observeRelistEvent(deliveryVerbAdd, 0)
		return
	}
	h.metrics.observeLiveEvent(deliveryVerbAdd, objectResourceVersion(obj))
}

func (h deliveryMeter) OnUpdate(oldObj, newObj any) {
	// A cache resync replays the store's object against itself; a live update always carries a new version.
	if objectResourceVersionString(oldObj) == objectResourceVersionString(newObj) {
		h.metrics.observeRelistEvent(deliveryVerbUpdate, 0)
		return
	}
	h.metrics.observeLiveEvent(deliveryVerbUpdate, objectResourceVersion(newObj))
}

func (h deliveryMeter) OnDelete(obj any) {
	if _, missed := obj.(cache.DeletedFinalStateUnknown); missed {
		// The delete was detected by a re-list; the object the tombstone carries predates it, so it cannot date it.
		h.metrics.observeRelistEvent(deliveryVerbDelete, 0)
		return
	}
	h.metrics.observeLiveEvent(deliveryVerbDelete, objectResourceVersion(obj))
}

func objectResourceVersionString(obj any) string {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return ""
	}
	return accessor.GetResourceVersion()
}

func objectResourceVersion(obj any) int64 {
	return parseResourceVersion(objectResourceVersionString(obj))
}
