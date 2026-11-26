package operator

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestInformerRun_QueueCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, stop := context.WithCancel(t.Context())
		defer stop()
		source := &registeredInformer{Informer: &testInformer{}}
		go source.Run(ctx)
		synctest.Wait()
		run := source.run.Load()
		key := &deliveryOrder{key: deliveryKey{id: resource.Identifier{Name: "item"}}, uid: "original"}
		var calls []string
		delivery := func(order cleanupOrder, name string) deferredDelivery {
			owner, cancel := context.WithCancel(ctx)
			t.Cleanup(cancel)
			return deferredDelivery{
				order: order, cleanup: true, owner: owner, live: run.workContext(owner),
				dispatch: func(context.Context) { calls = append(calls, name) },
			}
		}

		firstRetry := make(chan struct{})
		reconciler := delivery(cleanupOrder{reconcilerCleanup, 0}, "reconciler")
		reconciler.running = []<-chan struct{}{firstRetry}
		require.True(t, run.queueCleanup(key, []deferredDelivery{reconciler}))
		go run.drainCleanup(key)
		synctest.Wait()

		// A later event retains an earlier watcher while the reconciler is
		// waiting. The watcher can start before the reconciler's retry ends.
		watcher := delivery(cleanupOrder{watcherCleanup, 0}, "watcher")
		release := make(chan struct{})
		dispatch := watcher.dispatch
		watcher.dispatch = func(ctx context.Context) {
			dispatch(ctx)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		require.False(t, run.queueCleanup(key, []deferredDelivery{watcher, reconciler}))
		synctest.Wait()
		require.Equal(t, []string{"watcher"}, calls)

		// The first retry ends and a new one starts while the watcher is in
		// its callback. Keep the original reconciler request and wait for both.
		close(firstRetry)
		secondRetry := make(chan struct{})
		reconciler.running = []<-chan struct{}{secondRetry}
		reconciler.dispatch = func(context.Context) { t.Error("redelivery replaced the original reconciler request") }
		watcher.dispatch = func(context.Context) { calls = append(calls, "watcher redelivery") }
		for range 10 {
			require.False(t, run.queueCleanup(key, []deferredDelivery{watcher, reconciler}), "only the first admission starts a worker")
		}
		close(release)
		synctest.Wait()
		require.Equal(t, []string{"watcher"}, calls, "the redelivered watcher waits for the current pass to finish")
		close(secondRetry)
		synctest.Wait()
		require.Equal(t, []string{"watcher", "reconciler", "watcher redelivery"}, calls)
		require.Empty(t, run.cleanups)

		// A last callback can race another admission while the pending list
		// is empty. After the worker retires, the next event needs a new one.
		watcher.dispatch = func(context.Context) {
			calls = append(calls, "last watcher")
			reconciler.dispatch = func(context.Context) { calls = append(calls, "final reconciler") }
			if run.queueCleanup(key, []deferredDelivery{reconciler}) {
				t.Error("started another worker during the last callback")
			}
		}
		require.True(t, run.queueCleanup(key, []deferredDelivery{watcher}))
		go run.drainCleanup(key)
		synctest.Wait()
		require.Equal(t, []string{"watcher", "reconciler", "watcher redelivery", "last watcher", "final reconciler"}, calls)
		require.Empty(t, run.cleanups)
	})
}
