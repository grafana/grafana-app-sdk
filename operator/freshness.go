package operator

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// Read-freshness measures whether a read taken because of a live event observes the version that event announced.
// It answers one question an informer's own metrics cannot: a change source can announce a write to this replica
// before the read path can serve it, and a reconcile which reads a version older than the one it was woken for is
// acting on state the system already knows is stale.
//
// It is instrumentation. Nothing here feeds a retry, a cache decision or a dispatch; an adopter wires it around its
// own reads and the results pass through untouched.
//
// Two legal executions are excluded by construction, so a non-zero behind count cannot be dismissed as ordinary
// concurrency:
//
//   - the announcement is sampled before the read is issued, because a read already in flight when a newer write
//     commits may legitimately return the earlier state;
//   - a NotFound is never counted as behind, because an object deleted after its announcement is correctly absent,
//     and under a shared delivery group the delete proving it may have gone to another replica.

// Outcome label values. "behind" is the one a storage contract says cannot happen; "not_found" is ambiguous and is
// kept apart from it rather than folded in.
const (
	freshnessCurrent  = "current"
	freshnessBehind   = "behind"
	freshnessNotFound = "not_found"
)

const (
	// DefaultAnnouncementTTL is how long an announcement stays outstanding when none is configured. Only a read
	// taken soon after an announcement says anything about visibility, so entries whose next event lands on another
	// replica age out instead of accumulating.
	DefaultAnnouncementTTL = 15 * time.Minute
	// defaultAnnouncementSweep is how often expiry is swept, amortized over the event stream rather than paid per
	// call.
	defaultAnnouncementSweep = time.Minute
)

// FreshnessMetricsConfig configures the read-freshness metrics.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type FreshnessMetricsConfig struct {
	// MetricsConfig controls the namespace and native histogram settings of the collectors.
	MetricsConfig metrics.Config
	// ResourceVersionTime decodes the timestamp a resource version embeds, which is what turns a version difference
	// into a lag in seconds. Resource versions are opaque in the kubernetes API contract, so there is no portable
	// decoding: when this is nil the lag histogram stays empty and the counters are unaffected.
	ResourceVersionTime func(int64) time.Time
	// Registerer registers the collectors as they are built. See InformerDeliveryMetricsConfig.Registerer.
	Registerer prometheus.Registerer
}

// FreshnessMetrics reports how reads taken because of a live event compare against the version that event announced:
//
//	informer_read_freshness_total{group,version,kind,outcome}
//	informer_read_version_lag_seconds{group,version,kind}
//
// Only reads with an announcement outstanding are counted, so the three outcomes sum to that population and
// behind/(current+behind) is the rate at which such a read missed the announced write.
//
// Build it once per process and share it: the collectors are told apart by label, so building them per kind would
// register the same names twice.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type FreshnessMetrics struct {
	reads *prometheus.CounterVec
	lag   *prometheus.HistogramVec

	resourceVersionTime func(int64) time.Time
}

// NewFreshnessMetrics builds the read-freshness metrics.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewFreshnessMetrics(cfg FreshnessMetricsConfig) *FreshnessMetrics {
	return &FreshnessMetrics{
		reads: metrics.Register(cfg.Registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.MetricsConfig.Namespace,
			Name:      "informer_read_freshness_total",
			Help:      "Reads taken while a live event for the same object was outstanding, by outcome: current (at or above the announced resource version), behind (below it — the read missed a write already announced to this replica), not_found (absent, which is also correct for an object deleted after the announcement).",
		}, []string{"group", "version", "kind", "outcome"})),
		lag: metrics.Register(cfg.Registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                      cfg.MetricsConfig.Namespace,
			Name:                           "informer_read_version_lag_seconds",
			Help:                           "How far behind a read came back: the time between the announced resource version and the older one the read observed.",
			Buckets:                        prometheus.ExponentialBuckets(0.005, 4, 8),
			NativeHistogramBucketFactor:    cfg.MetricsConfig.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber: cfg.MetricsConfig.NativeHistogramMaxBucketNumber,
		}, []string{"group", "version", "kind"})),
		resourceVersionTime: cfg.ResourceVersionTime,
	}
}

// PrometheusCollectors implements metrics.Provider.
func (m *FreshnessMetrics) PrometheusCollectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return []prometheus.Collector{m.reads, m.lag}
}

// Recorder binds the metrics to one kind, creating all three outcome series at zero so that "nothing came back
// behind" reads as a flat zero rather than as an absent series. A nil FreshnessMetrics returns a recorder which
// discards.
func (m *FreshnessMetrics) Recorder(kind resource.Kind) FreshnessRecorder {
	if m == nil {
		return FreshnessRecorder{}
	}
	gvk := kind.GroupVersionKind()
	return FreshnessRecorder{
		kind:     gvk.String(),
		current:  m.reads.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind, freshnessCurrent),
		behind:   m.reads.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind, freshnessBehind),
		notFound: m.reads.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind, freshnessNotFound),
		lag:      m.lag.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind),

		resourceVersionTime: m.resourceVersionTime,
	}
}

// FreshnessRecorder records one kind's read-freshness observations. Its zero value discards, so a caller needs no
// nil checks.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type FreshnessRecorder struct {
	kind     string
	current  prometheus.Counter
	behind   prometheus.Counter
	notFound prometheus.Counter
	lag      prometheus.Observer

	resourceVersionTime func(int64) time.Time
}

// Observe records one read against the version announced before it was issued, which a caller takes from
// AnnouncementTracker.Announced or AnnouncedVersionFromContext. A zero announced version means none was
// outstanding, so there is nothing to judge the read against and nothing is recorded.
//
// Failures other than NotFound are not recorded either: a read which did not complete says nothing about what the
// read path can serve.
//
// The object is taken as a metav1.Object rather than a resource.Object because all that is read off it is the
// resource version: a kind read through a generated clientset, or an unstructured one, is judged without being
// wrapped in something the SDK recognizes.
func (r FreshnessRecorder) Observe(ctx context.Context, id resource.Identifier, announced int64, obj metav1.Object, err error) {
	if r.current == nil || announced <= 0 {
		return
	}
	logger := logging.FromContext(ctx).With("kind", r.kind, "namespace", id.Namespace, "name", id.Name)
	switch {
	case apierrors.IsNotFound(err):
		r.notFound.Inc()
		// A consumer reports a missing object too, but cannot say whether an event for it was outstanding, so this
		// is the only line carrying the version one announced — the shape a read which outran its own write would
		// take. It is not proof of one: the object may simply have been deleted after the announcement.
		logger.Info("read found no object while a live event for it was outstanding", "announced", announced)
	case err != nil:
		return
	default:
		observed := parseResourceVersion(obj.GetResourceVersion())
		if observed == 0 {
			return
		}
		if observed >= announced {
			r.current.Inc()
			return
		}
		r.behind.Inc()
		// The counter says the premise was contradicted; the log says which write, so it can be matched against
		// the storage side.
		logger = logger.With("announced", announced, "observed", observed)
		if r.resourceVersionTime == nil {
			logger.Warn("read observed an older version than the live event which announced it")
			return
		}
		// Zero is included rather than dropped: two versions from the same millisecond may differ only below the
		// timestamp, and dropping those would leave the histogram empty while the counter climbed.
		lag := r.resourceVersionTime(announced).Sub(r.resourceVersionTime(observed)).Seconds()
		if lag >= 0 {
			r.lag.Observe(lag)
		}
		logger.Warn("read observed an older version than the live event which announced it", "lagSeconds", lag)
	}
}

// AnnouncementTrackerOptions configures an AnnouncementTracker.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type AnnouncementTrackerOptions struct {
	// TTL is how long an announcement stays outstanding. A non-positive value uses DefaultAnnouncementTTL.
	TTL time.Duration
	// NormalizeResourceVersion maps a parsed resource version into the space versions are compared in, for a
	// storage layer whose versions are not directly comparable — two encodings in use across a migration, say.
	// Nil compares the parsed values as they are.
	NormalizeResourceVersion func(int64) int64
	// Now is the clock, for tests. Nil uses time.Now.
	Now func() time.Time
}

// AnnouncementTracker remembers, per object, the newest version a live event announced to this replica, so that a
// read taken later — by a work queue's worker, say, long after the event was dispatched — can be judged against it.
//
// A consumer which reads inside the dispatch itself does not need this: the version is on the context, from
// AnnouncedVersionFromContext.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type AnnouncementTracker struct {
	ttl       time.Duration
	normalize func(int64) int64
	now       func() time.Time

	mu        sync.Mutex
	seen      map[string]trackedAnnouncement
	lastSweep time.Time
}

type trackedAnnouncement struct {
	version int64
	seen    time.Time
}

// NewAnnouncementTracker returns an empty tracker.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewAnnouncementTracker(opts AnnouncementTrackerOptions) *AnnouncementTracker {
	if opts.TTL <= 0 {
		opts.TTL = DefaultAnnouncementTTL
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.NormalizeResourceVersion == nil {
		opts.NormalizeResourceVersion = func(rv int64) int64 { return rv }
	}
	return &AnnouncementTracker{
		ttl:       opts.TTL,
		normalize: opts.NormalizeResourceVersion,
		now:       opts.Now,
		seen:      map[string]trackedAnnouncement{},
	}
}

// Announce records that a live event announced a write to the object at resourceVersion, keeping the newest version
// seen and refreshing the entry's age. A version which does not parse is ignored: it cannot be compared with
// anything a read returns.
func (t *AnnouncementTracker) Announce(id resource.Identifier, resourceVersion string) {
	version := parseResourceVersion(resourceVersion)
	if version <= 0 {
		return
	}
	version = t.normalize(version)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()
	key := busStoreKey(id.Namespace, id.Name)
	entry := t.seen[key]
	if version > entry.version {
		entry.version = version
	}
	entry.seen = t.now()
	t.seen[key] = entry
}

// Forget drops the object's announcement, for a delete: the object is gone, so no read is expected to reach a
// version for it, and a correct NotFound must not be judged against one.
//
// A delete older than the outstanding announcement is kept out, because events for one key can arrive out of order:
// a delete can land after the re-create which superseded it, and erasing the newer version would stop counting the
// reads the announcement was there to judge. A delete whose version does not parse drops the key, which can only
// under-count.
func (t *AnnouncementTracker) Forget(id resource.Identifier, resourceVersion string) {
	version := parseResourceVersion(resourceVersion)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()
	key := busStoreKey(id.Namespace, id.Name)
	if entry, ok := t.seen[key]; ok && version > 0 && entry.version > t.normalize(version) {
		return
	}
	delete(t.seen, key)
}

// Announced returns the newest version announced for the object, or 0 when none is outstanding. Expiry is enforced
// here too, so an entry cannot outlive the TTL on a stream too quiet to sweep.
func (t *AnnouncementTracker) Announced(id resource.Identifier) int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := busStoreKey(id.Namespace, id.Name)
	entry, ok := t.seen[key]
	if !ok {
		return 0
	}
	if t.now().Sub(entry.seen) > t.ttl {
		delete(t.seen, key)
		return 0
	}
	return entry.version
}

// Len reports how many announcements are outstanding, for a test or a gauge.
func (t *AnnouncementTracker) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.seen)
}

func (t *AnnouncementTracker) sweepLocked() {
	now := t.now()
	if now.Sub(t.lastSweep) < defaultAnnouncementSweep {
		return
	}
	t.lastSweep = now
	for key, entry := range t.seen {
		if now.Sub(entry.seen) > t.ttl {
			delete(t.seen, key)
		}
	}
}

// Watcher returns a ResourceWatcher which feeds the tracker from an informer's events. Register it before the
// watchers which act on those events, since informers dispatch in registration order: a consumer which enqueues a
// key synchronously must find the announcement already recorded when its worker picks the key up.
//
// Only live events are recorded. A re-list delivery was caused by no announcement — its version came from the
// lister — so counting it would leave the metric reporting mostly "current" while measuring nothing.
func (t *AnnouncementTracker) Watcher() ResourceWatcher {
	return announcementWatcher{tracker: t}
}

type announcementWatcher struct {
	tracker *AnnouncementTracker
}

var _ ResourceWatcher = announcementWatcher{}

func (w announcementWatcher) Add(ctx context.Context, obj resource.Object) error {
	w.record(ctx, obj)
	return nil
}

func (w announcementWatcher) Update(ctx context.Context, _, tgt resource.Object) error {
	w.record(ctx, tgt)
	return nil
}

// Delete forgets the key rather than announcing it: the object is gone from storage by the time a delete is
// delivered, so the NotFound a later read returns is the truth and not a lagging read.
func (w announcementWatcher) Delete(ctx context.Context, obj resource.Object) error {
	if DeliveryFromContext(ctx) != DeliveryLive {
		return nil
	}
	w.tracker.Forget(identifierOf(obj), AnnouncedVersionFromContext(ctx))
	return nil
}

func (w announcementWatcher) record(ctx context.Context, obj resource.Object) {
	if DeliveryFromContext(ctx) != DeliveryLive {
		return
	}
	w.tracker.Announce(identifierOf(obj), AnnouncedVersionFromContext(ctx))
}

func identifierOf(obj resource.Object) resource.Identifier {
	return resource.Identifier{Namespace: obj.GetNamespace(), Name: obj.GetName()}
}

// ObserveRead runs a read and records how its result compared to the version a live event announced for the object,
// which is the whole of what a consumer has to do to wire read-freshness around its own reads: sample the
// announcement before issuing the read, then observe the pair.
//
// The result passes through untouched, error and all, so it reads as the read it wraps:
//
//	repo, err := operator.ObserveRead(ctx, tracker, recorder, id, func(ctx context.Context) (*v1.Repository, error) {
//	    return client.Repositories(id.Namespace).Get(ctx, id.Name, metav1.GetOptions{})
//	})
//
// A nil tracker skips the observation, so a deployment with read-freshness switched off needs no second code path.
//
// T is any object carrying kubernetes metadata — a generated clientset's type, an unstructured one, or a
// resource.Object — since the version is all that is read off it.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func ObserveRead[T metav1.Object](
	ctx context.Context, tracker *AnnouncementTracker, recorder FreshnessRecorder,
	id resource.Identifier, read func(context.Context) (T, error),
) (T, error) {
	if tracker == nil {
		return read(ctx)
	}
	// Sampled before the read is issued; see the top of the file.
	announced := tracker.Announced(id)
	obj, err := read(ctx)
	recorder.Observe(ctx, id, announced, obj, err)
	return obj, err
}
