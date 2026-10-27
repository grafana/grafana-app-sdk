package operator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestNewPeriodicInformer_Validation(t *testing.T) {
	_, err := NewPeriodicInformer(resource.Kind{}, staticLister(), PeriodicInformerOptions{})
	assert.Error(t, err, "an empty kind is rejected")

	_, err = NewPeriodicInformer(relistTestKind, nil, PeriodicInformerOptions{})
	assert.Error(t, err, "a nil lister is rejected")

	inf, err := NewPeriodicInformer(relistTestKind, staticLister(), PeriodicInformerOptions{})
	require.NoError(t, err)
	assert.Error(t, inf.AddEventHandler(nil))
	assert.Equal(t, relistTestKind.Schema, inf.Schema())
}

// Every pass delivers every listed object as an add, unchanged objects included: re-delivery is what re-triggers
// cadence-driven work.
func TestPeriodicInformer_RelistsOnCadence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		watcher := &recordingWatcher{}
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			calls.Add(1)
			return []resource.Object{relistObj("a")}, "", nil
		})
		inf, err := NewPeriodicInformer(relistTestKind, lister, PeriodicInformerOptions{ResyncInterval: time.Minute})
		require.NoError(t, err)
		require.NoError(t, inf.AddEventHandler(watcher))

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.True(t, inf.HasSynced())
		require.NoError(t, inf.HealthCheck(ctx))
		assert.Equal(t, []string{"a"}, watcher.addedNames())

		// The jitter spreads each interval by up to 10%, so sleeping past the longest possible one guarantees
		// exactly one more pass.
		time.Sleep(time.Minute + 7*time.Second)
		synctest.Wait()
		assert.Equal(t, []string{"a", "a"}, watcher.addedNames())
		assert.Equal(t, int64(2), calls.Load())
	})
}

// A failing initial list is retried, and the informer does not report synced until one succeeds.
func TestPeriodicInformer_RetriesInitialList(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		watcher := &recordingWatcher{}
		var calls atomic.Int64
		lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			if calls.Add(1) < 3 {
				return nil, "", errors.New("api unavailable")
			}
			return []resource.Object{relistObj("a")}, "", nil
		})
		inf, err := NewPeriodicInformer(relistTestKind, lister, PeriodicInformerOptions{
			ResyncInterval: time.Hour,
			ErrorHandler:   func(context.Context, error) {},
		})
		require.NoError(t, err)
		require.NoError(t, inf.AddEventHandler(watcher))

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); _ = inf.Run(ctx) }()
		defer func() { cancel(); <-done; synctest.Wait() }()

		synctest.Wait()
		require.False(t, inf.HasSynced(), "must not sync while the initial list fails")
		require.Error(t, inf.HealthCheck(ctx))

		for range 5 {
			if inf.HasSynced() {
				break
			}
			time.Sleep(DefaultRelistRetryInterval)
			synctest.Wait()
		}
		require.True(t, inf.HasSynced())
		assert.Equal(t, []string{"a"}, watcher.addedNames())
	})
}

func TestPeriodicInformer_WatcherErrorsReachTheErrorHandler(t *testing.T) {
	var handled []error
	inf, err := NewPeriodicInformer(relistTestKind, staticLister(relistObj("a")), PeriodicInformerOptions{
		ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
	})
	require.NoError(t, err)
	require.NoError(t, inf.AddEventHandler(&recordingWatcher{addErr: errors.New("boom")}))

	require.NoError(t, inf.relist(context.Background()))

	require.Len(t, handled, 1)
	assert.EqualError(t, handled[0], "boom")
}

func TestPeriodicInformer_WaitForSyncHonorsContext(t *testing.T) {
	inf, err := NewPeriodicInformer(relistTestKind, staticLister(), PeriodicInformerOptions{})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, inf.WaitForSync(ctx), context.Canceled)
}
