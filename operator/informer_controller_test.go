package operator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestInformerController_AddWatcher(t *testing.T) {
	t.Run("nil watcher", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		err := c.AddWatcher(nil, "")
		assert.Equal(t, errors.New("watcher cannot be nil"), err)
	})

	t.Run("empty resourceKind", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		err := c.AddWatcher(&SimpleWatcher{}, "")
		assert.Equal(t, errors.New("resourceKind cannot be empty"), err)
	})

	t.Run("first watcher", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		w := &SimpleWatcher{}
		k := "foo"
		err := c.AddWatcher(w, k)
		assert.Nil(t, err)
		assert.Equal(t, 1, c.watchers.KeySize(k))
		iw, _ := c.watchers.ItemAt(k, 0)
		assert.Equal(t, w, iw)
	})

	t.Run("existing watchers", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		w1 := &SimpleWatcher{}
		w2 := &SimpleWatcher{}
		k := "foo"
		err := c.AddWatcher(w1, k)
		assert.Nil(t, err)
		assert.Equal(t, 1, c.watchers.KeySize(k))
		iw1, _ := c.watchers.ItemAt(k, 0)
		assert.Equal(t, w1, iw1)
		err = c.AddWatcher(w2, k)
		assert.Nil(t, err)
		assert.Equal(t, 2, c.watchers.KeySize(k))
		iw1, _ = c.watchers.ItemAt(k, 0)
		iw2, _ := c.watchers.ItemAt(k, 1)
		assert.Equal(t, w1, iw1)
		assert.Equal(t, w2, iw2)
	})
}

func TestInformerController_RemoveWatcher(t *testing.T) {
	t.Run("nil watcher", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		// Ensure no panics
		c.RemoveWatcher(nil, "")
	})

	t.Run("empty resourceKind", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		// Ensure no panics
		c.RemoveWatcher(&SimpleWatcher{}, "")
	})

	t.Run("not in list", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		w1 := &SimpleWatcher{}
		w2 := &SimpleWatcher{}
		k := "foo"
		c.AddWatcher(w1, k)
		c.RemoveWatcher(w2, k)
		assert.Equal(t, 1, c.watchers.KeySize(k))
	})

	t.Run("only watcher in list", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		w := &SimpleWatcher{}
		k := "foo"
		c.AddWatcher(w, k)
		c.RemoveWatcher(w, k)
		assert.Equal(t, 0, c.watchers.KeySize(k))
	})

	t.Run("preserve order", func(t *testing.T) {
		w1 := &SimpleWatcher{}
		w2 := &SimpleWatcher{}
		w3 := &SimpleWatcher{}
		w4 := &SimpleWatcher{}
		resourceKind := "foo"
		c := NewInformerController(InformerControllerConfig{})
		c.AddWatcher(w1, resourceKind)
		c.AddWatcher(w2, resourceKind)
		c.AddWatcher(w3, resourceKind)
		c.AddWatcher(w4, resourceKind)

		// Do removes from the middle, beginning, and end of the list to make sure order is preserved
		c.RemoveWatcher(w3, resourceKind)
		assert.Equal(t, 3, c.watchers.KeySize(resourceKind))
		iw1, _ := c.watchers.ItemAt(resourceKind, 0)
		iw2, _ := c.watchers.ItemAt(resourceKind, 1)
		iw3, _ := c.watchers.ItemAt(resourceKind, 2)
		assert.Equal(t, w1, iw1)
		assert.Equal(t, w2, iw2)
		assert.Equal(t, w4, iw3)

		c.RemoveWatcher(w1, resourceKind)
		assert.Equal(t, 2, c.watchers.KeySize(resourceKind))
		iw1, _ = c.watchers.ItemAt(resourceKind, 0)
		iw2, _ = c.watchers.ItemAt(resourceKind, 1)
		assert.Equal(t, w2, iw1)
		assert.Equal(t, w4, iw2)

		c.RemoveWatcher(w4, resourceKind)
		assert.Equal(t, 1, c.watchers.KeySize(resourceKind))
		iw1, _ = c.watchers.ItemAt(resourceKind, 0)
		assert.Equal(t, w2, iw1)
	})
}

func TestInformerController_RemoveAllWatchersForResource(t *testing.T) {
	t.Run("empty key", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		// Ensure no panics
		c.RemoveAllWatchersForResource("")
	})

	t.Run("no watchers", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		// Ensure no panics
		c.RemoveAllWatchersForResource("foo")
	})

	t.Run("watchers", func(t *testing.T) {
		w1 := &SimpleWatcher{}
		w2 := &SimpleWatcher{}
		w3 := &SimpleWatcher{}
		k1 := "foo"
		k2 := "bar"
		c := NewInformerController(InformerControllerConfig{})
		c.AddWatcher(w1, k1)
		c.AddWatcher(w2, k1)
		c.AddWatcher(w3, k2)
		assert.Equal(t, 2, c.watchers.Size()) // 2 keys
		c.RemoveAllWatchersForResource(k1)
		assert.Equal(t, 1, c.watchers.Size()) // 1 key
		assert.Equal(t, 1, c.watchers.KeySize(k2))
	})
}

func TestInformerController_AddReconciler(t *testing.T) {
	t.Run("nil reconciler", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		err := c.AddReconciler(nil, "")
		assert.Equal(t, errors.New("reconciler cannot be nil"), err)
	})

	t.Run("empty resourceKind", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		err := c.AddReconciler(&SimpleReconciler{}, "")
		assert.Equal(t, errors.New("resourceKind cannot be empty"), err)
	})

	t.Run("first reconciler", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		r := &SimpleReconciler{}
		k := "foo"
		err := c.AddReconciler(r, k)
		assert.Nil(t, err)
		assert.Equal(t, 1, c.reconcilers.KeySize(k))
		iw, _ := c.reconcilers.ItemAt(k, 0)
		assert.Equal(t, r, iw)
	})

	t.Run("existing reconcilers", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		r1 := &SimpleReconciler{}
		r2 := &SimpleReconciler{}
		k := "foo"
		err := c.AddReconciler(r1, k)
		assert.Nil(t, err)
		assert.Equal(t, 1, c.reconcilers.KeySize(k))
		ir1, _ := c.reconcilers.ItemAt(k, 0)
		assert.Equal(t, r1, ir1)
		err = c.AddReconciler(r2, k)
		assert.Nil(t, err)
		assert.Equal(t, 2, c.reconcilers.KeySize(k))
		ir1, _ = c.reconcilers.ItemAt(k, 0)
		ir2, _ := c.reconcilers.ItemAt(k, 1)
		assert.Equal(t, r1, ir1)
		assert.Equal(t, r2, ir2)
	})
}

func TestInformerController_RemoveReconciler(t *testing.T) {
	t.Run("nil reconciler", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		// Ensure no panics
		c.RemoveReconciler(nil, "")
	})

	t.Run("empty resourceKind", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		// Ensure no panics
		c.RemoveReconciler(&SimpleReconciler{}, "")
	})

	t.Run("not in list", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		r1 := &SimpleReconciler{}
		r2 := &SimpleReconciler{}
		k := "foo"
		c.AddReconciler(r1, k)
		c.RemoveReconciler(r2, k)
		assert.Equal(t, 1, c.reconcilers.KeySize(k))
	})

	t.Run("only reconciler in list", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		r := &SimpleReconciler{}
		k := "foo"
		c.AddReconciler(r, k)
		c.RemoveReconciler(r, k)
		assert.Equal(t, 0, c.reconcilers.KeySize(k))
	})

	t.Run("preserve order", func(t *testing.T) {
		r1 := &SimpleReconciler{}
		r2 := &SimpleReconciler{}
		r3 := &SimpleReconciler{}
		r4 := &SimpleReconciler{}
		resourceKind := "foo"
		c := NewInformerController(InformerControllerConfig{})
		c.AddReconciler(r1, resourceKind)
		c.AddReconciler(r2, resourceKind)
		c.AddReconciler(r3, resourceKind)
		c.AddReconciler(r4, resourceKind)

		// Do removes from the middle, beginning, and end of the list to make sure order is preserved
		c.RemoveReconciler(r3, resourceKind)
		assert.Equal(t, 3, c.reconcilers.KeySize(resourceKind))
		ir1, _ := c.reconcilers.ItemAt(resourceKind, 0)
		ir2, _ := c.reconcilers.ItemAt(resourceKind, 1)
		ir3, _ := c.reconcilers.ItemAt(resourceKind, 2)
		assert.Equal(t, r1, ir1)
		assert.Equal(t, r2, ir2)
		assert.Equal(t, r4, ir3)

		c.RemoveReconciler(r1, resourceKind)
		assert.Equal(t, 2, c.reconcilers.KeySize(resourceKind))
		ir1, _ = c.reconcilers.ItemAt(resourceKind, 0)
		ir2, _ = c.reconcilers.ItemAt(resourceKind, 1)
		assert.Equal(t, r2, ir1)
		assert.Equal(t, r4, ir2)

		c.RemoveReconciler(r4, resourceKind)
		assert.Equal(t, 1, c.reconcilers.KeySize(resourceKind))
		ir1, _ = c.reconcilers.ItemAt(resourceKind, 0)
		assert.Equal(t, r2, ir1)
	})
}

func TestInformerController_RemoveAllReconcilersForResource(t *testing.T) {
	t.Run("empty key", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		// Ensure no panics
		c.RemoveAllReconcilersForResource("")
	})

	t.Run("no watchers", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		// Ensure no panics
		c.RemoveAllReconcilersForResource("foo")
	})

	t.Run("watchers", func(t *testing.T) {
		r1 := &SimpleReconciler{}
		r2 := &SimpleReconciler{}
		r3 := &SimpleReconciler{}
		k1 := "foo"
		k2 := "bar"
		c := NewInformerController(InformerControllerConfig{})
		c.AddReconciler(r1, k1)
		c.AddReconciler(r2, k1)
		c.AddReconciler(r3, k2)
		assert.Equal(t, 2, c.reconcilers.Size()) // 2 keys
		c.RemoveAllReconcilersForResource(k1)
		assert.Equal(t, 1, c.reconcilers.Size()) // 1 key
		assert.Equal(t, 1, c.reconcilers.KeySize(k2))
	})
}

func TestInformerController_AddInformer(t *testing.T) {
	t.Run("nil informer", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		err := c.AddInformer(nil, "")
		assert.Equal(t, errors.New("informer cannot be nil"), err)
	})

	t.Run("empty resourceKind", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		err := c.AddInformer(&mockInformer{}, "")
		assert.Equal(t, errors.New("resourceKind cannot be empty"), err)
	})

	t.Run("first informer", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		i := &mockInformer{}
		k := "foo"
		err := c.AddInformer(i, k)
		assert.Nil(t, err)
		assert.Equal(t, 1, c.informers.KeySize(k))
		ii1, _ := c.informers.ItemAt(k, 0)
		assert.Equal(t, i, ii1)
	})

	t.Run("existing informers", func(t *testing.T) {
		c := NewInformerController(InformerControllerConfig{})
		i1 := &mockInformer{}
		i2 := &mockInformer{}
		k := "foo"
		err := c.AddInformer(i1, k)
		assert.Nil(t, err)
		assert.Equal(t, 1, c.informers.KeySize(k))
		ii1, _ := c.informers.ItemAt(k, 0)
		assert.Equal(t, i1, ii1)
		err = c.AddInformer(i2, k)
		assert.Nil(t, err)
		assert.Equal(t, 2, c.informers.KeySize(k))
		ii1, _ = c.informers.ItemAt(k, 0)
		ii2, _ := c.informers.ItemAt(k, 1)
		assert.Equal(t, i1, ii1)
		assert.Equal(t, i2, ii2)
	})
}

func TestInformerController_Run(t *testing.T) {
	t.Run("normal operation", func(t *testing.T) {
		// Ensure that informer's Run() functions are called as part of the controller's Run()
		wg := sync.WaitGroup{}
		c := NewInformerController(InformerControllerConfig{})
		inf1 := &mockInformer{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				wg.Done()
				return nil
			},
		}
		c.AddInformer(inf1, "foo")
		inf2 := &mockInformer{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				wg.Done()
				return nil
			},
		}
		c.AddInformer(inf2, "bar")
		wg.Add(3)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		go func() {
			err := c.Run(ctx)
			assert.Nil(t, err)
			wg.Done()
		}()
		wg.Wait()
	})
}

func TestInformerController_Run_WithWatcherAndReconciler(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		// Ensure that events emitted from informers are propagated to watchers and reconcilers
		kind := "foo"
		addCalls := 0
		reconcileCalls := 0
		inf := &testInformer{}
		c := NewInformerController(InformerControllerConfig{})
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCalls++
				return nil
			},
		}, kind)
		c.AddReconciler(&SimpleReconciler{
			ReconcileFunc: func(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
				reconcileCalls++
				return ReconcileResult{}, nil
			},
		}, kind)
		c.AddInformer(inf, kind)

		// Run
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go c.Run(ctx)
		inf.FireAdd(context.Background(), emptyObject)
		assert.Equal(t, 1, addCalls)
		assert.Equal(t, 1, reconcileCalls)
	})

	t.Run("watcher error, one retry", func(t *testing.T) {
		// Ensure that errors processing events in the watcher are retried per the RetryPolicy
		kind := "foo"
		addCalls := 0
		reconcileCalls := 0
		inf := &testInformer{}
		c := NewInformerController(InformerControllerConfig{})
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			if attempt >= 1 {
				return false, 0
			}
			return true, time.Millisecond * 50
		}
		c.retryTickerInterval = time.Millisecond * 50
		wg := sync.WaitGroup{}
		wg.Add(2)
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCalls++
				require.Less(t, addCalls, 3, "Add should only be retried once based on the RetryPolicy")
				wg.Done()
				return errors.New("I AM ERROR")
			},
		}, kind)
		c.AddReconciler(&SimpleReconciler{
			ReconcileFunc: func(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
				reconcileCalls++
				return ReconcileResult{}, nil
			},
		}, kind)
		c.AddInformer(inf, kind)

		// Run
		ctx, cancel := context.WithCancel(context.Background())
		go c.Run(ctx)
		inf.FireAdd(context.Background(), emptyObject)
		wg.Wait()
		cancel()
		assert.Equal(t, 2, addCalls)
		assert.Equal(t, 1, reconcileCalls)
	})

	t.Run("no errors, reconciler retry", func(t *testing.T) {
		kind := "foo"
		addCalls := 0
		reconcileCalls := 0
		inf := &testInformer{}
		c := NewInformerController(InformerControllerConfig{})
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			if attempt >= 1 {
				return false, 0
			}
			return true, time.Millisecond * 50
		}
		c.retryTickerInterval = time.Millisecond * 50
		wg := sync.WaitGroup{}
		wg.Add(2)
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCalls++
				return nil
			},
		}, kind)
		c.AddReconciler(&SimpleReconciler{
			ReconcileFunc: func(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
				reconcileCalls++
				require.Less(t, reconcileCalls, 3, "Add should only be retried once based on the state check (improper state passed to Reconcile retry)")
				wg.Done()
				if len(request.State) > 0 {
					return ReconcileResult{}, nil
				}
				after := time.Millisecond * 100
				return ReconcileResult{
					State: map[string]any{
						"retry": true,
					},
					RequeueAfter: &after,
				}, nil
			},
		}, kind)
		c.AddInformer(inf, kind)

		// Run
		ctx, cancel := context.WithCancel(context.Background())
		go c.Run(ctx)
		inf.FireAdd(context.Background(), emptyObject)
		wg.Wait()
		cancel()
		assert.Equal(t, 1, addCalls)
		assert.Equal(t, 2, reconcileCalls)
	})

	t.Run("watcher error, reconciler retry", func(t *testing.T) {
		kind := "foo"
		addCalls := 0
		reconcileCalls := 0
		inf := &testInformer{}
		c := NewInformerController(InformerControllerConfig{})
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			if attempt >= 1 {
				return false, 0
			}
			return true, time.Millisecond * 50
		}
		c.retryTickerInterval = time.Millisecond * 50
		wg := sync.WaitGroup{}
		wg.Add(4)
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCalls++
				require.Less(t, addCalls, 3, "Add should only be retried once based on the RetryPolicy")
				wg.Done()
				return errors.New("I AM ERROR")
			},
		}, kind)
		c.AddReconciler(&SimpleReconciler{
			ReconcileFunc: func(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
				reconcileCalls++
				require.Less(t, reconcileCalls, 3, "Add should only be retried once based on the state check (improper state passed to Reconcile retry)")
				wg.Done()
				if len(request.State) > 0 {
					return ReconcileResult{}, nil
				}
				after := time.Millisecond * 100
				return ReconcileResult{
					State: map[string]any{
						"retry": true,
					},
					RequeueAfter: &after,
				}, nil
			},
		}, kind)
		c.AddInformer(inf, kind)

		// Run
		ctx, cancel := context.WithCancel(context.Background())
		go c.Run(ctx)
		inf.FireAdd(context.Background(), emptyObject)
		wg.Wait()
		cancel()
		assert.Equal(t, 2, addCalls)
		assert.Equal(t, 2, reconcileCalls)
	})

	t.Run("reconciler error, one retry", func(t *testing.T) {
		kind := "foo"
		addCalls := 0
		reconcileCalls := 0
		inf := &testInformer{}
		c := NewInformerController(InformerControllerConfig{})
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			if attempt > 1 {
				return false, 0
			}
			return true, time.Millisecond * 50
		}
		c.retryTickerInterval = time.Millisecond * 50
		wg := sync.WaitGroup{}
		wg.Add(2)
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCalls++
				return nil
			},
		}, kind)
		c.AddReconciler(&SimpleReconciler{
			ReconcileFunc: func(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
				reconcileCalls++
				wg.Done()
				return ReconcileResult{}, errors.New("I AM ERROR")
			},
		}, kind)
		c.AddInformer(inf, kind)

		// Run
		ctx, cancel := context.WithCancel(context.Background())
		go c.Run(ctx)
		inf.FireAdd(context.Background(), emptyObject)
		wg.Wait()
		cancel()
		assert.Equal(t, 1, addCalls)
		assert.Equal(t, 2, reconcileCalls)
	})

	t.Run("reconciler and watcher errors, one retry", func(t *testing.T) {
		kind := "foo"
		addCalls := 0
		reconcileCalls := 0
		inf := &testInformer{}
		c := NewInformerController(InformerControllerConfig{})
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			if attempt > 1 {
				return false, 0
			}
			return true, time.Millisecond * 50
		}
		c.retryTickerInterval = time.Millisecond * 50
		wg := sync.WaitGroup{}
		wg.Add(4)
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCalls++
				if addCalls >= 3 {
					t.Fatal("Add should only be retried once based on the RetryPolicy")
				}
				wg.Done()
				return errors.New("I AM ERROR")
			},
		}, kind)
		c.AddReconciler(&SimpleReconciler{
			ReconcileFunc: func(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
				reconcileCalls++
				if reconcileCalls >= 3 {
					t.Fatal("Reconcile should only be retried once based on the RetryPolicy")
				}
				wg.Done()
				return ReconcileResult{}, errors.New("ICH BIN ERROR")
			},
		}, kind)
		c.AddInformer(inf, kind)

		// Run
		ctx, cancel := context.WithCancel(context.Background())
		go c.Run(ctx)
		inf.FireAdd(context.Background(), emptyObject)
		wg.Wait()
		cancel()
		assert.Equal(t, 2, addCalls)
		assert.Equal(t, 2, reconcileCalls)
	})
}

func TestInformerController_Run_BackoffRetry(t *testing.T) {
	// The backoff retry test needs to take at least 16 seconds to run properly, so it's isolated to its own function
	// to avoid the often-used default of a 30-second-timeout on tests affecting other retry tests which take a few seconds each to run
	addError := errors.New("I AM ERROR")
	addAttempt := 0
	updateError := errors.New("JE SUIS ERROR")
	updateAttempt := 0
	baseInterval := time.Second
	jitter := time.Millisecond * 50
	wg := sync.WaitGroup{}
	inf := &testInformer{
		handlers: make([]ResourceWatcher, 0),
		onStop: func() {
			wg.Done()
		},
	}
	c := NewInformerController(InformerControllerConfig{})
	// One-second multiplier on exponential backoff.
	// Backoff will be 1s, 2s, 4s, 8s, 16s
	c.RetryPolicy = ExponentialBackoffRetryPolicy(baseInterval, 5)
	// Ensure the default dequeue policy doesn't interfere with our retries
	c.RetryDequeuePolicy = func(_ ResourceAction, _ resource.Object, _ ResourceAction, _ resource.Object, _ error) bool {
		return false
	}
	c.AddInformer(inf, "foo")
	type chanTuple struct {
		attempt     int
		lastAttempt time.Time
	}
	addChan := make(chan chanTuple, 6)
	defer close(addChan)
	updateChan := make(chan chanTuple, 6)
	defer close(updateChan)
	lastAdd := time.Now()
	lastUpdate := time.Now()
	c.AddWatcher(&SimpleWatcher{
		AddFunc: func(ctx context.Context, object resource.Object) error {
			addChan <- chanTuple{
				attempt:     addAttempt,
				lastAttempt: lastAdd,
			}
			lastAdd = time.Now()
			addAttempt++
			return addError
		},
		UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
			updateChan <- chanTuple{
				attempt:     updateAttempt,
				lastAttempt: lastUpdate,
			}
			lastUpdate = time.Now()
			updateAttempt++
			return updateError
		},
	}, "foo")
	wg.Add(2)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := c.Run(ctx)
		assert.Nil(t, err)
		wg.Done()
	}()

	addWG := sync.WaitGroup{}
	addWG.Add(5)
	go func() {
		for tpl := range addChan {
			if tpl.attempt < 5 {
				addWG.Done()
			}
			if tpl.attempt == 0 {
				continue
			}
			// Check that the time since the last call is not less than (retry attempt)^2 seconds. Retry attempt is attempt-1 (as we're tracking attempts including the first one in this test)
			assert.GreaterOrEqual(t, time.Now().Sub(tpl.lastAttempt), time.Duration(math.Pow(2, float64(tpl.attempt-1)))*baseInterval-jitter)
		}
	}()
	updateWG := sync.WaitGroup{}
	updateWG.Add(5)
	go func() {
		for tpl := range updateChan {
			if tpl.attempt < 5 {
				updateWG.Done()
			}
			if tpl.attempt == 0 {
				continue
			}
			// Check that the time since the last call is not less than (retry attempt)^2 seconds. Retry attempt is attempt-1 (as we're tracking attempts including the first one in this test)
			assert.GreaterOrEqual(t, time.Now().Sub(tpl.lastAttempt), time.Duration(math.Pow(2, float64(tpl.attempt-1)))*baseInterval-jitter)
		}
	}()

	inf.FireAdd(context.Background(), emptyObject)
	inf.FireUpdate(context.Background(), nil, emptyObject)

	assert.True(t, waitOrTimeout(&addWG, time.Second*30), fmt.Sprintf("timed out waiting for adds to finish (saw %d adds)", addAttempt))
	assert.True(t, waitOrTimeout(&updateWG, time.Second*30), fmt.Sprintf("timed out waiting for updates to finish (saw %d updates)", updateAttempt))
	cancel()
	wg.Wait()
}

func TestInformerController_Run_WithRetries(t *testing.T) {
	// Because these tests take more time, isolate them to their own test function
	t.Run("linear retry, update call interrupts add retry", func(t *testing.T) {
		addError := errors.New("I AM ERROR")
		addAttempt := 0
		updateAttempt := 0
		wg := sync.WaitGroup{}
		inf := &testInformer{
			handlers: make([]ResourceWatcher, 0),
			onStop: func() {
				wg.Done()
			},
		}
		c := NewInformerController(InformerControllerConfig{})
		// Make the retry ticker interval a half-second so we can run this test faster
		c.retryTickerInterval = time.Millisecond * 500
		// 500-ms linear retry policy
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			return true, time.Millisecond * 500
		}
		c.AddInformer(inf, "foo")
		addCh := make(chan int, 5)
		defer close(addCh)
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCh <- addAttempt
				addAttempt++
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateAttempt++
				return nil
			},
		}, "foo")
		wg.Add(2)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			err := c.Run(ctx)
			assert.Nil(t, err)
			wg.Done()
		}()
		inf.FireAdd(context.Background(), emptyObject)
		// Wait for three add attempts (initial and two retries), then fire an update
		for i := range 3 {
			t.Log("waiting for add ", i)
			<-addCh
		}
		// Fire an update, which should halt the add retries
		inf.FireUpdate(context.Background(), nil, emptyObject)
		// We _may_ get one more add retry, if it was already happening when the update was processed. But we shouldn't get any more than that
		// Wait a second before stopping in case another retry _does_ come through
		time.Sleep(time.Second * 1)
		cancel()
		wg.Wait()
		// We should have four total attempts, though we may be off-by-one because timing is hard,
		// so we actually check that 3 <= attempts <= 5
		assert.LessOrEqual(t, 3, addAttempt)
		assert.GreaterOrEqual(t, 4, addAttempt)
		assert.Equal(t, 1, updateAttempt)
	})

	t.Run("linear retry, successful retry stops new retries", func(t *testing.T) {
		addError := errors.New("I AM ERROR")
		addAttempt := 0
		updateError := errors.New("JE SUIS ERROR")
		updateAttempt := 0
		wg := sync.WaitGroup{}
		inf := &testInformer{
			handlers: make([]ResourceWatcher, 0),
			onStop: func() {
				wg.Done()
			},
		}
		c := NewInformerController(InformerControllerConfig{})
		// Make the retry ticker interval a 50 ms so we can run this test faster
		c.retryTickerInterval = time.Millisecond * 50
		// 500-ms linear retry policy
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			return true, time.Millisecond * 50
		}
		c.AddInformer(inf, "foo")
		addCh := make(chan int, 3)
		defer close(addCh)
		updateCh := make(chan int, 3)
		defer close(updateCh)
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCh <- addAttempt
				addAttempt++
				if addAttempt >= 2 {
					return nil
				}
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateCh <- updateAttempt
				updateAttempt++
				if updateAttempt >= 2 {
					return nil
				}
				return updateError
			},
		}, "foo")
		wg.Add(2)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			err := c.Run(ctx)
			assert.Nil(t, err)
			wg.Done()
		}()
		inf.FireAdd(context.Background(), emptyObject)
		// Wait for two adds
		<-addCh
		<-addCh
		// Wait for half a second, this should be enough time for many retries if the halt doesn't work
		time.Sleep(time.Millisecond * 500)
		inf.FireUpdate(context.Background(), nil, emptyObject)
		// Wait for two updates
		<-updateCh
		<-updateCh
		// Wait for half a second, this should be enough time for many retries if the halt doesn't work
		cancel()
		wg.Wait()
		// We should have only two attempts for each
		assert.Equal(t, 2, addAttempt)
		assert.Equal(t, 2, updateAttempt)
	})

	t.Run("linear retry, retry halts when limit reached", func(t *testing.T) {
		addError := errors.New("I AM ERROR")
		addAttempt := 0
		wg := sync.WaitGroup{}
		inf := &testInformer{
			handlers: make([]ResourceWatcher, 0),
			onStop: func() {
				wg.Done()
			},
		}
		c := NewInformerController(InformerControllerConfig{})
		// Make the retry ticker interval a 50 ms so we can run this test faster
		c.retryTickerInterval = time.Millisecond * 50
		// 500-ms linear retry policy
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			return attempt < 3, time.Millisecond * 50
		}
		c.AddInformer(inf, "foo")
		addCh := make(chan int, 3)
		defer close(addCh)
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCh <- addAttempt
				addAttempt++
				return addError
			},
		}, "foo")
		wg.Add(2)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			err := c.Run(ctx)
			assert.Nil(t, err)
			wg.Done()
		}()
		inf.FireAdd(context.Background(), emptyObject)
		// We should see three attempts, then be done
		for range 3 {
			<-addCh
		}
		// Wait for half a second, this should be enough time for many retries if the halt doesn't work
		time.Sleep(time.Millisecond * 500)
		cancel()
		wg.Wait()
		// We should have four total attempts, 1 initial and 3 retries
		assert.Equal(t, 4, addAttempt)
	})
}

func TestInformerController_Run_WithRetriesAndDequeuePolicy(t *testing.T) {
	t.Run("linear retry, don't dequeue", func(t *testing.T) {
		addError := errors.New("I AM ERROR")
		inf := &testInformer{}
		c := NewInformerController(InformerControllerConfig{})
		retryQuery := make(chan error, 1)
		retryResponse := make(chan bool, 10) // Larger buffer to avoid deadlocks
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			if attempt == 0 {
				return true, time.Second
			}
			// Send a signal that the RetryPolicy was queried
			retryQuery <- err
			// Wait until signaled to return a retry time.
			// This should block a pending retry until told to go again
			ret := <-retryResponse
			return ret, time.Second
		}
		c.retryTickerInterval = time.Millisecond * 50
		c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy
		c.AddInformer(inf, "foo")
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				return nil
			},
		}, "foo")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			err := c.Run(ctx)
			assert.Nil(t, err)
		}()

		// Ok, here's how the test goes. It's a bit complicated to avoid having to do timing things,
		// which can cause intermittent failures based on resourcing.
		// Fire off an add. This will fail in the watcher and ask the RetryPolicy if it should be retried.
		// The RetryPolicy always says yes without waiting for the first ask (0 attempts),
		// So this call will not block waiting for a response (testInformer.FireX calls block until the handlers are finished)
		inf.FireAdd(context.Background(), emptyObject)

		// Now that the retry is queued, we can fire off an update. This SHOULD NOT dequeue the pending add
		inf.FireUpdate(context.Background(), nil, emptyObject)

		// Now we wait for the RetryPolicy to be queried again, OR for a timeout, which indicates a failure
		timeout := time.NewTimer(2 * time.Second)
		t.Cleanup(func() { timeout.Stop() })
		select {
		case <-retryQuery:
			// The retry wasn't dequeued, we can tell it to stop retrying now and finish the test
			retryResponse <- true
		case <-timeout.C:
			assert.Fail(t, "Add Event retry appears to have been dequeued")
		}
	})

	t.Run("linear retry, don't dequeue, overlapping retries", func(t *testing.T) {
		addError := errors.New("I AM ERROR")
		updateError := errors.New("JE SUIS ERROR")
		inf := &testInformer{}
		c := NewInformerController(InformerControllerConfig{})
		retryQuery := make(chan error, 1)
		retryResponse := make(chan bool, 10) // Larger buffer to avoid deadlocks
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			if attempt == 0 {
				return true, time.Second
			}
			// Send a signal that the RetryPolicy was queried
			retryQuery <- err
			// Wait until signaled to return a retry time.
			// This should block a pending retry until told to go again
			ret := <-retryResponse
			return ret, time.Second
		}
		c.retryTickerInterval = time.Millisecond * 50
		c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy
		c.AddInformer(inf, "foo")
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				return updateError
			},
		}, "foo")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			err := c.Run(ctx)
			assert.Nil(t, err)
		}()

		// Ok, here's how the test goes. It's a bit complicated to avoid having to do timing things,
		// which can cause intermittent failures based on resourcing.
		// Fire off an add. This will fail in the watcher and ask the RetryPolicy if it should be retried.
		// The RetryPolicy always says yes without waiting for the first ask (0 attempts),
		// So this call will not block waiting for a response (testInformer.FireX calls block until the handlers are finished)
		inf.FireAdd(context.Background(), emptyObject)

		// Now that the retry is queued, we can fire off an update. This SHOULD NOT dequeue the pending add.
		// The update call should ALSO fail, which will query the retry policy, which will, on attempt 0, tell it to retry after 1 second
		// without notifying the channel
		inf.FireUpdate(context.Background(), nil, emptyObject)

		// Now we wait for the RetryPolicy to be queried again, OR for a timeout, which indicates a failure
		timeout := time.NewTimer(2 * time.Second)
		t.Cleanup(func() { timeout.Stop() })
		addRetries := 0
		updateRetries := 0
		for i := 0; i < 2; i++ {
			select {
			case err := <-retryQuery:
				// Check what request this is a retry for by examining the error
				if err == addError {
					addRetries++
				} else {
					updateRetries++
				}
				// The retry wasn't dequeued, we can tell it to stop retrying now and either wait for the other
				// request's retry, or finish the test
				retryResponse <- true
			case <-timeout.C:
				assert.Fail(t, "Add Event retry appears to have been dequeued")
			}
		}
		assert.Equal(t, 1, addRetries)
		assert.Equal(t, 1, updateRetries)
	})
}

func TestOpinionatedRetryDequeuePolicy(t *testing.T) {
	tests := []struct {
		name        string
		newAction   ResourceAction
		newObject   resource.Object
		retryAction ResourceAction
		retryObject resource.Object
		retryError  error
		expected    bool
	}{
		{
			name:        "subsequent delete, should delete",
			newAction:   ResourceActionDelete,
			newObject:   nil,
			retryAction: ResourceActionUpdate,
			retryObject: nil,
			retryError:  nil,
			expected:    true,
		},
		{
			name:        "different actions, shouldn't dequeue",
			newAction:   ResourceActionUpdate,
			newObject:   nil,
			retryAction: ResourceActionCreate,
			retryObject: nil,
			retryError:  nil,
			expected:    false,
		},
		{
			name:      "same generations, shouldn't dequeue",
			newAction: ResourceActionUpdate,
			newObject: &resource.TypedSpecObject[string]{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			retryAction: ResourceActionCreate,
			retryObject: &resource.TypedSpecObject[string]{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			retryError: nil,
			expected:   false,
		},
		{
			name:      "different generations, should dequeue",
			newAction: ResourceActionUpdate,
			newObject: &resource.TypedSpecObject[string]{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
			},
			retryAction: ResourceActionCreate,
			retryObject: &resource.TypedSpecObject[string]{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
			},
			retryError: nil,
			expected:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := OpinionatedRetryDequeuePolicy(test.newAction, test.newObject, test.retryAction, test.retryObject, test.retryError)
			assert.Equal(t, test.expected, res)
		})
	}
}

type mockInformer struct {
	AddEventHandlerFunc func(handler ResourceWatcher)
	RunFunc             func(ctx context.Context) error
}

func (i *mockInformer) AddEventHandler(handler ResourceWatcher) error {
	if i.AddEventHandlerFunc != nil {
		i.AddEventHandlerFunc(handler)
	}
	return nil
}
func (i *mockInformer) Run(ctx context.Context) error {
	if i.RunFunc != nil {
		return i.RunFunc(ctx)
	}
	return nil
}

func (i *mockInformer) WaitForSync(ctx context.Context) error {
	return nil
}

type testInformer struct {
	handlers []ResourceWatcher
	onStop   func()
}

func (ti *testInformer) AddEventHandler(handler ResourceWatcher) error {
	ti.handlers = append(ti.handlers, handler)
	return nil
}

func (ti *testInformer) Run(ctx context.Context) error {
	<-ctx.Done()
	if ti.onStop != nil {
		ti.onStop()
	}
	return nil
}

func (ti *testInformer) FireAdd(ctx context.Context, object resource.Object) {
	for _, w := range ti.handlers {
		w.Add(ctx, object)
	}
}

func (ti *testInformer) FireUpdate(ctx context.Context, oldObj, newObj resource.Object) {
	for _, w := range ti.handlers {
		w.Update(ctx, oldObj, newObj)
	}
}

func (ti *testInformer) FireDelete(ctx context.Context, object resource.Object) {
	for _, w := range ti.handlers {
		w.Delete(ctx, object)
	}
}

func (ti *testInformer) WaitForSync(ctx context.Context) error {
	return nil
}

var emptyObject = &resource.TypedSpecObject[string]{}
