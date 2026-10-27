package operator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

func freshID(name string) resource.Identifier {
	return resource.Identifier{Namespace: busTestNamespace, Name: name}
}

func newFreshnessMetrics() *FreshnessMetrics {
	return NewFreshnessMetrics(FreshnessMetricsConfig{
		MetricsConfig:       metrics.DefaultConfig("test"),
		ResourceVersionTime: func(rv int64) time.Time { return time.UnixMilli(rv) },
	})
}

func freshnessCount(m *FreshnessMetrics, outcome string) float64 {
	gvk := busTestKind.GroupVersionKind()
	return testutil.ToFloat64(m.reads.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind, outcome))
}

// lagSamples is the number of observations in the lag histogram. CollectAndCount counts series, and Recorder
// creates the series eagerly, so it cannot tell "no lag recorded" from "none observed".
func lagSamples(t *testing.T, m *FreshnessMetrics) uint64 {
	t.Helper()
	gvk := busTestKind.GroupVersionKind()
	observer := m.lag.WithLabelValues(gvk.Group, gvk.Version, gvk.Kind)
	metric, ok := observer.(prometheus.Metric)
	require.True(t, ok)
	var dtoMetric dto.Metric
	require.NoError(t, metric.Write(&dtoMetric))
	return dtoMetric.GetHistogram().GetSampleCount()
}

func notFoundErr(name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Resource: "widgets"}, name)
}

func TestFreshnessRecorder_Observe(t *testing.T) {
	ctx := context.Background()

	t.Run("a read at or past the announced version is current", func(t *testing.T) {
		m := newFreshnessMetrics()
		r := m.Recorder(busTestKind)

		r.Observe(ctx, freshID("x"), 100, busObjRV("x", 100), nil)
		r.Observe(ctx, freshID("x"), 100, busObjRV("x", 200), nil)

		assert.Equal(t, float64(2), freshnessCount(m, freshnessCurrent))
		assert.Zero(t, freshnessCount(m, freshnessBehind))
	})

	t.Run("a read below it is behind, and its lag is recorded", func(t *testing.T) {
		m := newFreshnessMetrics()

		m.Recorder(busTestKind).Observe(ctx, freshID("x"), 5000, busObjRV("x", 2000), nil)

		assert.Equal(t, float64(1), freshnessCount(m, freshnessBehind))
		assert.Equal(t, uint64(1), lagSamples(t, m), "the gap between the two versions")
	})

	// A read already in flight when a newer write commits may legitimately return the earlier state, so nothing is
	// judged unless an announcement was outstanding when the read was issued.
	t.Run("a read with nothing announced is not counted", func(t *testing.T) {
		m := newFreshnessMetrics()

		m.Recorder(busTestKind).Observe(ctx, freshID("x"), 0, busObjRV("x", 1), nil)

		assert.Zero(t, freshnessCount(m, freshnessCurrent))
		assert.Zero(t, freshnessCount(m, freshnessBehind))
		assert.Zero(t, freshnessCount(m, freshnessNotFound))
	})

	// An object deleted after its announcement is correctly absent, and under a shared group the delete proving it
	// may have gone to another replica — so a NotFound is its own outcome and never "behind".
	t.Run("a NotFound is counted apart from behind", func(t *testing.T) {
		m := newFreshnessMetrics()

		m.Recorder(busTestKind).Observe(ctx, freshID("x"), 100, nil, notFoundErr("x"))

		assert.Equal(t, float64(1), freshnessCount(m, freshnessNotFound))
		assert.Zero(t, freshnessCount(m, freshnessBehind))
	})

	t.Run("any other failure is not counted", func(t *testing.T) {
		m := newFreshnessMetrics()

		m.Recorder(busTestKind).Observe(ctx, freshID("x"), 100, nil, errors.New("api unavailable"))

		assert.Zero(t, freshnessCount(m, freshnessNotFound))
		assert.Zero(t, freshnessCount(m, freshnessCurrent))
	})

	t.Run("an opaque observed version cannot be compared", func(t *testing.T) {
		m := newFreshnessMetrics()
		opaque := busObj("x")
		opaque.ResourceVersion = "opaque"

		m.Recorder(busTestKind).Observe(ctx, freshID("x"), 100, opaque, nil)

		assert.Zero(t, freshnessCount(m, freshnessBehind))
		assert.Zero(t, freshnessCount(m, freshnessCurrent))
	})

	// Without a decoder there is no portable way to turn a version difference into seconds, so the counter still
	// moves and the histogram stays empty.
	t.Run("no version decoder still counts, without a lag", func(t *testing.T) {
		m := NewFreshnessMetrics(FreshnessMetricsConfig{MetricsConfig: metrics.DefaultConfig("test")})

		m.Recorder(busTestKind).Observe(ctx, freshID("x"), 5000, busObjRV("x", 2000), nil)

		assert.Equal(t, float64(1), freshnessCount(m, freshnessBehind))
		assert.Zero(t, lagSamples(t, m))
	})

	t.Run("the zero recorder discards", func(t *testing.T) {
		var r FreshnessRecorder
		r.Observe(ctx, freshID("x"), 100, busObjRV("x", 1), nil)
		assert.Equal(t, FreshnessRecorder{}, (*FreshnessMetrics)(nil).Recorder(busTestKind))
	})
}

func TestFreshnessMetrics_PrometheusCollectors(t *testing.T) {
	assert.Len(t, newFreshnessMetrics().PrometheusCollectors(), 2)
	assert.Nil(t, (*FreshnessMetrics)(nil).PrometheusCollectors())
}

func TestAnnouncementTracker(t *testing.T) {
	t.Run("keeps the newest version announced", func(t *testing.T) {
		tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{})

		tracker.Announce(freshID("x"), "200")
		tracker.Announce(freshID("x"), "100")

		assert.Equal(t, int64(200), tracker.Announced(freshID("x")), "an out-of-order older event must not lower it")
		assert.Zero(t, tracker.Announced(freshID("absent")))
	})

	t.Run("an unparseable version is ignored", func(t *testing.T) {
		tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{})
		tracker.Announce(freshID("x"), "opaque")
		assert.Zero(t, tracker.Announced(freshID("x")), "it cannot be compared with anything a read returns")
	})

	// Events for one key can arrive out of order, so a delete older than the outstanding announcement must not
	// erase the newer version and stop counting the reads it was there to judge.
	t.Run("a stale delete does not forget a newer announcement", func(t *testing.T) {
		tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{})
		tracker.Announce(freshID("x"), "300")

		tracker.Forget(freshID("x"), "200")
		assert.Equal(t, int64(300), tracker.Announced(freshID("x")))

		tracker.Forget(freshID("x"), "400")
		assert.Zero(t, tracker.Announced(freshID("x")))
	})

	t.Run("an undatable delete forgets the key", func(t *testing.T) {
		tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{})
		tracker.Announce(freshID("x"), "300")
		tracker.Forget(freshID("x"), "")
		assert.Zero(t, tracker.Announced(freshID("x")), "under-counting beats judging a read against a stale version")
	})

	t.Run("entries expire", func(t *testing.T) {
		now := time.Now()
		tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{
			TTL: time.Minute,
			Now: func() time.Time { return now },
		})
		tracker.Announce(freshID("x"), "100")
		require.Equal(t, int64(100), tracker.Announced(freshID("x")))

		now = now.Add(2 * time.Minute)
		assert.Zero(t, tracker.Announced(freshID("x")), "a read this long after says nothing about visibility")
		assert.Zero(t, tracker.Len(), "and the entry is dropped rather than kept")
	})

	t.Run("a sweep drops what the read path never touches", func(t *testing.T) {
		now := time.Now()
		tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{
			TTL: time.Minute,
			Now: func() time.Time { return now },
		})
		for _, name := range []string{"a", "b", "c"} {
			tracker.Announce(freshID(name), "100")
		}
		require.Equal(t, 3, tracker.Len())

		now = now.Add(2 * time.Minute)
		tracker.Announce(freshID("fresh"), "200")

		assert.Equal(t, 1, tracker.Len(), "the expired keys are swept on the next announcement")
	})

	// A storage layer whose versions are not directly comparable — two encodings across a migration — normalizes
	// them into one space before they are compared.
	t.Run("versions are normalized before comparison", func(t *testing.T) {
		tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{
			NormalizeResourceVersion: func(rv int64) int64 { return rv * 1000 },
		})
		tracker.Announce(freshID("x"), "5")
		assert.Equal(t, int64(5000), tracker.Announced(freshID("x")))
	})
}

// The tracker feeds itself from an informer's events, using the delivery origin and announced version the informer
// puts on the dispatch context.
func TestAnnouncementTracker_Watcher(t *testing.T) {
	tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{})
	inf := newBusInformer(t, resourcebus.NewFake(), staticLister(busObjRV("listed", 50)), nil, BusInformerOptions{})
	require.NoError(t, inf.AddEventHandler(tracker.Watcher()))

	t.Run("a live event announces", func(t *testing.T) {
		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "x", "100"))
		assert.Equal(t, int64(100), tracker.Announced(freshID("x")))

		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbModified, "x", "200"))
		assert.Equal(t, int64(200), tracker.Announced(freshID("x")))
	})

	t.Run("a live delete forgets", func(t *testing.T) {
		inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "x", "300"))
		assert.Zero(t, tracker.Announced(freshID("x")), "the object is gone, so a NotFound is the truth")
	})

	// A re-list delivery was caused by no announcement — its version came from the lister — so counting it would
	// leave the metric reporting mostly "current" while measuring nothing.
	t.Run("a re-list delivery announces nothing", func(t *testing.T) {
		require.NoError(t, inf.relist(context.Background(), initialRelist))
		assert.Zero(t, tracker.Announced(freshID("listed")))
	})
}

// ObserveRead is the sample-before-read wrapper, so a consumer writes one line rather than its own getter type.
func TestObserveRead(t *testing.T) {
	id := resource.Identifier{Namespace: busTestNamespace, Name: "a"}
	obj := busObjRV("a", 10)

	t.Run("records the read against the version announced before it was issued", func(t *testing.T) {
		m := newFreshnessMetrics()
		tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{})
		tracker.Announce(id, "20")

		read, err := ObserveRead(context.Background(), tracker, m.Recorder(busTestKind), id,
			func(context.Context) (*resource.UntypedObject, error) { return obj, nil })

		require.NoError(t, err)
		assert.Same(t, obj, read, "the result passes through untouched")
		assert.Equal(t, float64(1), freshnessCount(m, freshnessBehind))
	})

	t.Run("passes a failure through and records nothing for it", func(t *testing.T) {
		m := newFreshnessMetrics()
		tracker := NewAnnouncementTracker(AnnouncementTrackerOptions{})
		tracker.Announce(id, "20")
		wanted := errors.New("api is down")

		_, err := ObserveRead(context.Background(), tracker, m.Recorder(busTestKind), id,
			func(context.Context) (*resource.UntypedObject, error) { return nil, wanted })

		assert.ErrorIs(t, err, wanted)
		assert.Zero(t, freshnessCount(m, freshnessBehind))
		assert.Zero(t, freshnessCount(m, freshnessNotFound))
	})

	// A deployment with read-freshness switched off reads through the same call site.
	t.Run("a nil tracker reads without observing", func(t *testing.T) {
		var calls int
		read, err := ObserveRead(context.Background(), nil, FreshnessRecorder{}, id,
			func(context.Context) (*resource.UntypedObject, error) {
				calls++
				return obj, nil
			})

		require.NoError(t, err)
		assert.Same(t, obj, read)
		assert.Equal(t, 1, calls)
	})
}
