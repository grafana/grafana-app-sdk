package operator

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

func TestNewConcurrentWatcher(t *testing.T) {
	t.Run("nil args", func(t *testing.T) {
		cw, err := newConcurrentWatcher(nil, 0, nil)
		assert.Nil(t, cw)
		assert.EqualError(t, err, "resource watcher cannot be nil")

		cw, err = newConcurrentWatcher(&SimpleWatcher{}, 0, nil)
		assert.Nil(t, cw)
		assert.EqualError(t, err, "initial worker pool size needs to be greater than 0")

		// In case of a nil errorHandler, we create a ConcurrentWatcher with DefaultErrorHandler
		cw, err = newConcurrentWatcher(&SimpleWatcher{}, 1, nil)
		assert.NoError(t, err)
		assert.NotNil(t, cw)
	})

	t.Run("success", func(t *testing.T) {
		var size uint64 = 2
		cw, err := newConcurrentWatcher(&SimpleWatcher{}, size, DefaultErrorHandler)
		assert.NoError(t, err)
		assert.NotNil(t, cw)
		assert.Len(t, cw.workers, int(size))
	})
}

func TestConcurrentWatcher_Add(t *testing.T) {
	ex := &resource.TypedSpecObject[string]{}
	schema := resource.NewSimpleSchema("group", "version", ex, &resource.TypedList[*resource.TypedSpecObject[string]]{})

	t.Run("successful add with single worker", func(t *testing.T) {
		mock := &mockWatcher{}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 1, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj := schema.ZeroValue()
		err = cw.Add(t.Context(), obj)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(1), mock.addAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())
	})

	t.Run("error handler should be called in case of an error", func(t *testing.T) {
		mock := &mockWatcher{}
		mock.AddFunc = func(ctx context.Context, o resource.Object) error {
			return fmt.Errorf("IT'S-A ME, ERRORIO!")
		}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 1, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj := schema.ZeroValue()
		err = cw.Add(t.Context(), obj)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(1), mock.addAttempts.Load())
		assert.Equal(t, int64(1), errCount.Load())
	})

	t.Run("successful adds with multiple workers", func(t *testing.T) {
		mock := &mockWatcher{}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 3, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj1 := schema.ZeroValue()
		obj1.SetName("one")
		obj2 := schema.ZeroValue()
		obj2.SetName("two")
		obj3 := schema.ZeroValue()
		obj3.SetName("three")
		err = cw.Add(t.Context(), obj1)
		assert.Nil(t, err)
		err = cw.Add(t.Context(), obj2)
		assert.Nil(t, err)
		err = cw.Add(t.Context(), obj3)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(3), mock.addAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())
	})
}

func TestConcurrentWatcher_Update(t *testing.T) {
	ex := &resource.TypedSpecObject[string]{}
	schema := resource.NewSimpleSchema("group", "version", ex, &resource.TypedList[*resource.TypedSpecObject[string]]{})

	t.Run("successful update with single worker", func(t *testing.T) {
		mock := &mockWatcher{}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 1, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj := schema.ZeroValue()
		err = cw.Update(t.Context(), obj, obj)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(1), mock.updateAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())
	})

	t.Run("error handler should be called in case of an error", func(t *testing.T) {
		mock := &mockWatcher{}
		mock.UpdateFunc = func(_ context.Context, _, _ resource.Object) error {
			return fmt.Errorf("IT'S-A ME, ERRORIO!")
		}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 1, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj := schema.ZeroValue()
		err = cw.Update(t.Context(), obj, obj)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(1), mock.updateAttempts.Load())
		assert.Equal(t, int64(1), errCount.Load())
	})

	t.Run("successful updates with multiple workers", func(t *testing.T) {
		mock := &mockWatcher{}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 3, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj1 := schema.ZeroValue()
		obj1.SetName("one")
		obj2 := schema.ZeroValue()
		obj2.SetName("two")
		obj3 := schema.ZeroValue()
		obj3.SetName("three")
		err = cw.Update(t.Context(), obj1, obj1)
		assert.Nil(t, err)
		err = cw.Update(t.Context(), obj2, obj2)
		assert.Nil(t, err)
		err = cw.Update(t.Context(), obj3, obj3)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(3), mock.updateAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())
	})
}

func TestConcurrentWatcher_Delete(t *testing.T) {
	ex := &resource.TypedSpecObject[string]{}
	schema := resource.NewSimpleSchema("group", "version", ex, &resource.TypedList[*resource.TypedSpecObject[string]]{})

	t.Run("successful delete with single worker", func(t *testing.T) {
		mock := &mockWatcher{}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 1, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj := schema.ZeroValue()
		err = cw.Delete(t.Context(), obj)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(1), mock.deleteAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())
	})

	t.Run("error handler should be called in case of an error", func(t *testing.T) {
		mock := &mockWatcher{}
		mock.DeleteFunc = func(_ context.Context, _ resource.Object) error {
			return fmt.Errorf("IT'S-A ME, ERRORIO!")
		}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 1, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj := schema.ZeroValue()
		err = cw.Delete(t.Context(), obj)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(1), mock.deleteAttempts.Load())
		assert.Equal(t, int64(1), errCount.Load())
	})

	t.Run("successful deletes with multiple workers", func(t *testing.T) {
		mock := &mockWatcher{}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 3, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj1 := schema.ZeroValue()
		obj1.SetName("one")
		obj2 := schema.ZeroValue()
		obj2.SetName("two")
		obj3 := schema.ZeroValue()
		obj3.SetName("three")
		err = cw.Delete(t.Context(), obj1)
		assert.Nil(t, err)
		err = cw.Delete(t.Context(), obj2)
		assert.Nil(t, err)
		err = cw.Delete(t.Context(), obj3)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(3), mock.deleteAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())
	})
}

func TestConcurrentWatcher(t *testing.T) {
	ex := &resource.TypedSpecObject[string]{}
	schema := resource.NewSimpleSchema("group", "version", ex, &resource.TypedList[*resource.TypedSpecObject[string]]{})

	t.Run("successfully trigger appropriate handler methods with single worker", func(t *testing.T) {
		mock := &mockWatcher{}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 1, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		obj := schema.ZeroValue()
		err = cw.Add(t.Context(), obj)
		assert.Nil(t, err)
		err = cw.Update(t.Context(), obj, obj)
		assert.Nil(t, err)
		err = cw.Update(t.Context(), obj, obj)
		assert.Nil(t, err)
		err = cw.Delete(t.Context(), obj)
		assert.Nil(t, err)
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(1), mock.addAttempts.Load())
		assert.Equal(t, int64(2), mock.updateAttempts.Load())
		assert.Equal(t, int64(1), mock.deleteAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())
	})

	t.Run("successfully trigger appropriate handler methods with multiple workers", func(t *testing.T) {
		mock := &mockWatcher{}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 3, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		for i := 0; i < 3; i++ {
			obj := schema.ZeroValue()
			obj.SetName(strconv.Itoa(i))
			err = cw.Add(t.Context(), obj)
			assert.Nil(t, err)
			err = cw.Update(t.Context(), obj, obj)
			assert.Nil(t, err)
			err = cw.Update(t.Context(), obj, obj)
			assert.Nil(t, err)
			err = cw.Delete(t.Context(), obj)
			assert.Nil(t, err)
		}
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(1*3), mock.addAttempts.Load())
		assert.Equal(t, int64(2*3), mock.updateAttempts.Load())
		assert.Equal(t, int64(1*3), mock.deleteAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())
	})

	t.Run("event for the same object should be processed sequentially (ie by the same worker)", func(t *testing.T) {
		events := make([]string, 0)
		mock := &mockWatcher{}
		mock.AddFunc = func(ctx context.Context, o resource.Object) error {
			events = append(events, "add")
			return nil
		}
		mock.UpdateFunc = func(ctx context.Context, _, _ resource.Object) error {
			events = append(events, "update")
			return nil
		}
		mock.DeleteFunc = func(ctx context.Context, o resource.Object) error {
			events = append(events, "delete")
			return nil
		}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 4, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)
		go cw.Run(t.Context())
		{
			obj := schema.ZeroValue()
			obj.SetName("one")
			err = cw.Add(t.Context(), obj)
			assert.Nil(t, err)
			err = cw.Update(t.Context(), obj, obj)
			assert.Nil(t, err)
			err = cw.Update(t.Context(), obj, obj)
			assert.Nil(t, err)
			err = cw.Delete(t.Context(), obj)
			assert.Nil(t, err)
		}
		// this should be enough for the workers to process the event from queue.
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, int64(1), mock.addAttempts.Load())
		assert.Equal(t, int64(2), mock.updateAttempts.Load())
		assert.Equal(t, int64(1), mock.deleteAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())
		// Events received should be in the same order of events triggered always.
		assert.Equal(t, []string{"add", "update", "update", "delete"}, events)
	})

	t.Run("events should be processed concurrently with multiple workers", func(t *testing.T) {
		mock := &mockWatcher{}
		mock.AddFunc = func(ctx context.Context, o resource.Object) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		}
		var errCount atomic.Int64
		cw, err := newConcurrentWatcher(mock, 3, func(ctx context.Context, err error) { errCount.Add(1) })
		assert.Nil(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		runReturned := make(chan struct{})
		go func() {
			cw.Run(ctx)
			close(runReturned)
		}()

		for i := 0; i < 90; i++ {
			obj := schema.ZeroValue()
			obj.SetName(strconv.Itoa(i))
			err = cw.Add(t.Context(), obj)
			assert.Nil(t, err)
		}
		// Assuming an event distribution within 3 workers, 90 events (each taking 100ms) should take ~3 seconds
		// to get processed concurrently. With some margin, waiting for 4s should be enough to test if the events
		// are being processed concurrently, as otherwise (sequentially) it would take ~9 seconds.
		time.Sleep(4 * time.Second)
		assert.Equal(t, int64(90), mock.addAttempts.Load())
		assert.Equal(t, int64(0), errCount.Load())

		cancel()
		select {
		case <-runReturned:
		case <-time.After(1 * time.Second):
			t.Fatal("Run did not return in time")
		}
	})
}

type mockWatcher struct {
	addAttempts    atomic.Int64
	updateAttempts atomic.Int64
	deleteAttempts atomic.Int64

	SimpleWatcher
}

func (mw *mockWatcher) Add(ctx context.Context, obj resource.Object) error {
	mw.addAttempts.Add(1)
	return mw.SimpleWatcher.Add(ctx, obj)
}

func (mw *mockWatcher) Update(ctx context.Context, src, tgt resource.Object) error {
	mw.updateAttempts.Add(1)
	return mw.SimpleWatcher.Update(ctx, src, tgt)
}

func (mw *mockWatcher) Delete(ctx context.Context, obj resource.Object) error {
	mw.deleteAttempts.Add(1)
	return mw.SimpleWatcher.Delete(ctx, obj)
}

func TestConcurrentInformer_PrometheusCollectors(t *testing.T) {
	t.Run("implements metrics.Provider", func(t *testing.T) {
		ci, err := NewConcurrentInformerFromOptions(&noopInformer{}, InformerOptions{
			MetricsConfig: metrics.DefaultConfig("test"),
			ResourceKind:  "test.example.com/v1, Kind=TestKind",
		})
		require.NoError(t, err)

		var provider metrics.Provider = ci
		collectors := provider.PrometheusCollectors()
		assert.Len(t, collectors, 1)
	})

	t.Run("queue_depth reflects pending events", func(t *testing.T) {
		blockCh := make(chan struct{})
		mock := &mockWatcher{}
		mock.AddFunc = func(ctx context.Context, o resource.Object) error {
			<-blockCh
			return nil
		}

		ci, err := NewConcurrentInformerFromOptions(&noopInformer{}, InformerOptions{
			MaxConcurrentWorkers: 2,
			MetricsConfig:        metrics.DefaultConfig("test"),
		})
		require.NoError(t, err)

		err = ci.AddEventHandler(mock)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go ci.Run(ctx)

		assert.Equal(t, float64(0), ci.sumQueueDepth())

		ex := &resource.TypedSpecObject[string]{}
		schema := resource.NewSimpleSchema("group", "version", ex, &resource.TypedList[*resource.TypedSpecObject[string]]{})

		obj1 := schema.ZeroValue()
		obj1.SetName("obj1")
		obj2 := schema.ZeroValue()
		obj2.SetName("obj2")

		ci.watchers[0].Add(ctx, obj1)
		ci.watchers[0].Add(ctx, obj2)

		assert.Eventually(t, func() bool {
			return ci.sumQueueDepth() > 0
		}, time.Second, 10*time.Millisecond)

		close(blockCh)

		assert.Eventually(t, func() bool {
			return ci.sumQueueDepth() == 0
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("multiple informers with different kinds register without collision", func(t *testing.T) {
		reg := prometheus.NewRegistry()

		ci1, err := NewConcurrentInformerFromOptions(&noopInformer{}, InformerOptions{
			MetricsConfig: metrics.DefaultConfig("test"),
			ResourceKind:  "group1/v1, Kind=KindA",
		})
		require.NoError(t, err)
		for _, c := range ci1.PrometheusCollectors() {
			require.NoError(t, reg.Register(c))
		}

		ci2, err := NewConcurrentInformerFromOptions(&noopInformer{}, InformerOptions{
			MetricsConfig: metrics.DefaultConfig("test"),
			ResourceKind:  "group2/v1, Kind=KindB",
		})
		require.NoError(t, err)
		for _, c := range ci2.PrometheusCollectors() {
			require.NoError(t, reg.Register(c))
		}

		mfs, err := reg.Gather()
		require.NoError(t, err)
		var found int
		for _, mf := range mfs {
			if mf.GetName() == "test_concurrent_watcher_queue_depth" {
				found = len(mf.GetMetric())
			}
		}
		assert.Equal(t, 2, found)
	})

	t.Run("deprecated constructor also creates metrics", func(t *testing.T) {
		ci, err := NewConcurrentInformer(&noopInformer{}, ConcurrentInformerOptions{
			MetricsConfig: metrics.DefaultConfig("test"),
		})
		require.NoError(t, err)

		collectors := ci.PrometheusCollectors()
		assert.Len(t, collectors, 1)
	})
}

type noopInformer struct{}

func (m *noopInformer) AddEventHandler(_ ResourceWatcher) error { return nil }
func (m *noopInformer) Run(ctx context.Context) error           { <-ctx.Done(); return nil }
func (m *noopInformer) WaitForSync(ctx context.Context) error   { return nil }
