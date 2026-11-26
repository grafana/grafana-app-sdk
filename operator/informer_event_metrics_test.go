package operator

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

func TestEventMetricsSharedAcrossSubscriptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		metrics := NewInformerEventMetrics(InformerEventMetricsConfig{Namespace: "sdk_test"})
		var published []func(context.Context, resourcebus.Event)
		subscriber := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, publish func(context.Context, resourcebus.Event), _ ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			published = append(published, publish)
			return &testBusSubscription{}, nil
		})
		newInformer := func() *busInformer {
			b, err := newBusInformer(listerTestKind, subscriber, BusInformerOptions{Metrics: metrics})
			require.NoError(t, err)
			require.NoError(t, b.AddEventHandler(&SimpleWatcher{}))
			return b
		}
		b1, b2 := newInformer(), newInformer()
		registry := prometheus.NewRegistry()
		controller := NewInformerController(InformerControllerConfig{})
		require.NoError(t, controller.AddInformer(b1, "first"))
		require.NoError(t, controller.AddInformer(b2, "second"))
		registry.MustRegister(controller.PrometheusCollectors()...)
		require.Equal(t, b1.PrometheusCollectors(), b2.PrometheusCollectors())
		const family = "sdk_test_informer_bus_ready_subscriptions"
		require.NoError(t, testutil.GatherAndCompare(registry, strings.NewReader(`# HELP sdk_test_informer_bus_ready_subscriptions Open subscription handles that completed readiness.
# TYPE sdk_test_informer_bus_ready_subscriptions gauge
sdk_test_informer_bus_ready_subscriptions{group="example.grafana.app",kind="Widget",version="v1"} 0
`), family))

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 2)
		go func() { done <- b1.Run(ctx) }()
		require.NoError(t, b1.WaitForSync(ctx))
		go func() { done <- b2.Run(ctx) }()
		require.NoError(t, b2.WaitForSync(ctx))
		for _, publish := range published {
			publish(ctx, resourcebus.Event{Verb: resourcebus.VerbAdded, Name: "item"})
		}
		synctest.Wait()
		labels := metricKindLabels(listerTestKind)
		// Constructing another informer must leave other ready handles counted.
		_ = newInformer()
		require.Equal(t, float64(2), testutil.ToFloat64(metrics.ready.WithLabelValues(labels...)))
		require.Equal(t, float64(2), testutil.ToFloat64(metrics.deliveries.WithLabelValues(append(labels, "live", "add")...)))
		cancel()
		require.NoError(t, <-done)
		require.NoError(t, <-done)
		require.Zero(t, testutil.ToFloat64(metrics.ready.WithLabelValues(labels...)))
	})
}

func TestEventMetricsCountNotificationQueueOverflow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		metrics := NewInformerEventMetrics(InformerEventMetricsConfig{Namespace: "sdk_test"})
		var publish func(context.Context, resourcebus.Event)
		subscriber := busSubscribeFunc(func(_ context.Context, _ schema.GroupVersionResource, _ string, handler func(context.Context, resourcebus.Event), _ ...resourcebus.SubscribeOption) (resourcebus.Subscription, error) {
			publish = handler
			return &testBusSubscription{ready: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			}}, nil
		})
		inf, err := newBusInformer(listerTestKind, subscriber, BusInformerOptions{Metrics: metrics})
		require.NoError(t, err)
		registry := prometheus.NewRegistry()
		registry.MustRegister(inf.PrometheusCollectors()...)
		done := make(chan error, 1)
		go func() { done <- inf.Run(ctx) }()
		synctest.Wait()
		require.NotNil(t, publish)

		for range busQueueSize + 2 {
			publish(ctx, resourcebus.Event{Verb: resourcebus.VerbAdded, Name: "item"})
		}
		const family = "sdk_test_informer_bus_events_dropped_total"
		require.NoError(t, testutil.GatherAndCompare(registry, strings.NewReader(`# HELP sdk_test_informer_bus_events_dropped_total Bus notifications discarded by the informer.
# TYPE sdk_test_informer_bus_events_dropped_total counter
sdk_test_informer_bus_events_dropped_total{group="example.grafana.app",kind="Widget",reason="queue_full",version="v1"} 2
`), family))
		cancel()
		require.NoError(t, <-done)
	})
}

func TestEventMetricsCountActualPages(t *testing.T) {
	m := NewInformerEventMetrics(InformerEventMetricsConfig{})
	client := mockListWatchClient{ListIntoFunc: func(_ context.Context, _ string, opts resource.ListOptions, into resource.ListObject) error {
		if opts.Continue == "" {
			into.SetContinue("next")
		}
		return nil
	}}
	lister, err := NewClientLister(client, listerTestKind, ClientListerOptions{})
	require.NoError(t, err)
	_, _, err = lister.List(withListMetrics(t.Context(), m, listerTestKind))
	require.NoError(t, err)
	require.Equal(t, float64(2), testutil.ToFloat64(m.pages.WithLabelValues(metricKindLabels(listerTestKind)...)))
}

func TestEventMetricsCountNativeListPages(t *testing.T) {
	m := NewInformerEventMetrics(InformerEventMetricsConfig{})
	lister, err := NewPagedLister(listerTestKind, func(_ context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		page := &resource.UntypedList{}
		if opts.Continue == "" {
			page.SetContinue("next")
		}
		return page, nil
	}, PagedListerOptions{})
	require.NoError(t, err)
	_, _, err = lister.List(withListMetrics(t.Context(), m, listerTestKind))
	require.NoError(t, err)
	require.Equal(t, float64(2), testutil.ToFloat64(m.pages.WithLabelValues(metricKindLabels(listerTestKind)...)))
}
