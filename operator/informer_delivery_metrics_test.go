package operator

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/metrics"
)

func newTestDeliveryMetrics() *InformerDeliveryMetrics {
	return NewInformerDeliveryMetrics(InformerDeliveryMetricsConfig{
		MetricsConfig:       metrics.DefaultConfig("test"),
		ResourceVersionTime: func(rv int64) time.Time { return time.UnixMilli(rv) },
	})
}

// histogramSamples reads the sample count off one kind's series. CollectAndCount would only say whether the series
// exists, which a Recorder creates eagerly.
func histogramSamples(t *testing.T, vec *prometheus.HistogramVec) uint64 {
	t.Helper()
	metric, ok := vec.WithLabelValues(busKindLabels...).(prometheus.Metric)
	require.True(t, ok)
	var dtoMetric dto.Metric
	require.NoError(t, metric.Write(&dtoMetric))
	return dtoMetric.GetHistogram().GetSampleCount()
}

func deliveryMetricValue(m *InformerDeliveryMetrics, vec string, extra ...string) float64 {
	labels := append(append([]string(nil), busKindLabels...), extra...)
	if vec == "live_events" {
		return testutil.ToFloat64(m.liveEvents.WithLabelValues(labels...))
	}
	return testutil.ToFloat64(m.relistEvents.WithLabelValues(labels...))
}

// The watch-backed informer draws no live/re-list distinction of its own, so the meter derives it. Deriving it the
// same way is what keeps a kind's series stable when it moves between a watch and a change bus.
func TestDeliveryMeter_DerivesLiveFromRelist(t *testing.T) {
	m := newTestDeliveryMetrics()
	meter := deliveryMeter{metrics: newDeliveryKindMetrics(m, busTestKind)}
	recent := time.Now().Add(-time.Second).UnixMilli()

	meter.OnAdd(busObjRV("a", recent), true)
	meter.OnAdd(busObjRV("b", recent), false)
	meter.OnUpdate(busObjRV("a", recent), busObjRV("a", recent))
	meter.OnUpdate(busObjRV("a", recent), busObjRV("a", recent+1))
	meter.OnDelete(cache.DeletedFinalStateUnknown{Key: "default/a", Obj: busObjRV("a", recent)})
	meter.OnDelete(busObjRV("b", recent))

	assert.Equal(t, float64(1), deliveryMetricValue(m, "relist_events", deliveryVerbAdd), "the initial list")
	assert.Equal(t, float64(1), deliveryMetricValue(m, "live_events", deliveryVerbAdd))
	assert.Equal(t, float64(1), deliveryMetricValue(m, "relist_events", deliveryVerbUpdate), "a resync replay")
	assert.Equal(t, float64(1), deliveryMetricValue(m, "live_events", deliveryVerbUpdate))
	assert.Equal(t, float64(1), deliveryMetricValue(m, "relist_events", deliveryVerbDelete), "a missed delete")
	assert.Equal(t, float64(1), deliveryMetricValue(m, "live_events", deliveryVerbDelete))

	// Only a live event carries a version which dates the change; the three re-list deliveries above cannot.
	assert.Equal(t, uint64(3), histogramSamples(t, m.liveLatency))
	assert.Zero(t, histogramSamples(t, m.relistLatency))
}

// An object with no readable metadata must not take the meter down with it.
func TestDeliveryMeter_ToleratesAnUnreadableObject(t *testing.T) {
	m := newTestDeliveryMetrics()
	meter := deliveryMeter{metrics: newDeliveryKindMetrics(m, busTestKind)}

	meter.OnAdd("not an object", false)
	meter.OnDelete(cache.DeletedFinalStateUnknown{Key: "default/a"})

	assert.Equal(t, float64(1), deliveryMetricValue(m, "live_events", deliveryVerbAdd))
	assert.Zero(t, histogramSamples(t, m.liveLatency), "an unreadable version dates nothing")
}

// A bus informer's deliveries land on the same collectors, so a process can hand one set to every informer it runs.
func TestBusInformerMetrics_ShareTheDeliveryCollectors(t *testing.T) {
	delivery := newTestDeliveryMetrics()
	bus := NewBusInformerMetrics(BusInformerMetricsConfig{Delivery: delivery})

	assert.Same(t, delivery, bus.Delivery())
	assert.Subset(t, bus.PrometheusCollectors(), delivery.PrometheusCollectors())

	newKindMetrics(bus, busTestKind).observeLiveEvent(deliveryVerbAdd, 0)
	assert.Equal(t, float64(1), deliveryMetricValue(delivery, "live_events", deliveryVerbAdd))
}

// A nil set of collectors discards, so an informer given none needs no branch of its own.
func TestInformerDeliveryMetrics_NilDiscards(t *testing.T) {
	var m *InformerDeliveryMetrics
	assert.Nil(t, m.PrometheusCollectors())
	assert.Equal(t, RelistRecorder{}, m.RelistRecorder(busTestKind))

	k := newDeliveryKindMetrics(nil, busTestKind)
	k.observeLiveEvent(deliveryVerbAdd, 1)
	k.observeRelistEvent(deliveryVerbAdd, 1)
	k.observeDroppedEvent()
	k.observeReconnect()
	k.observeSubscriptionOpen()
}
