package operator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/metrics"
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
		assert.Equal(t, w, iw.handler)
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
		assert.Equal(t, w1, iw1.handler)
		err = c.AddWatcher(w2, k)
		assert.Nil(t, err)
		assert.Equal(t, 2, c.watchers.KeySize(k))
		iw1, _ = c.watchers.ItemAt(k, 0)
		iw2, _ := c.watchers.ItemAt(k, 1)
		assert.Equal(t, w1, iw1.handler)
		assert.Equal(t, w2, iw2.handler)
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
		assert.Equal(t, w1, iw1.handler)
		assert.Equal(t, w2, iw2.handler)
		assert.Equal(t, w4, iw3.handler)

		c.RemoveWatcher(w1, resourceKind)
		assert.Equal(t, 2, c.watchers.KeySize(resourceKind))
		iw1, _ = c.watchers.ItemAt(resourceKind, 0)
		iw2, _ = c.watchers.ItemAt(resourceKind, 1)
		assert.Equal(t, w2, iw1.handler)
		assert.Equal(t, w4, iw2.handler)

		c.RemoveWatcher(w4, resourceKind)
		assert.Equal(t, 1, c.watchers.KeySize(resourceKind))
		iw1, _ = c.watchers.ItemAt(resourceKind, 0)
		assert.Equal(t, w2, iw1.handler)
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
		assert.Equal(t, r, iw.handler)
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
		assert.Equal(t, r1, ir1.handler)
		err = c.AddReconciler(r2, k)
		assert.Nil(t, err)
		assert.Equal(t, 2, c.reconcilers.KeySize(k))
		ir1, _ = c.reconcilers.ItemAt(k, 0)
		ir2, _ := c.reconcilers.ItemAt(k, 1)
		assert.Equal(t, r1, ir1.handler)
		assert.Equal(t, r2, ir2.handler)
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
		assert.Equal(t, r1, ir1.handler)
		assert.Equal(t, r2, ir2.handler)
		assert.Equal(t, r4, ir3.handler)

		c.RemoveReconciler(r1, resourceKind)
		assert.Equal(t, 2, c.reconcilers.KeySize(resourceKind))
		ir1, _ = c.reconcilers.ItemAt(resourceKind, 0)
		ir2, _ = c.reconcilers.ItemAt(resourceKind, 1)
		assert.Equal(t, r2, ir1.handler)
		assert.Equal(t, r4, ir2.handler)

		c.RemoveReconciler(r4, resourceKind)
		assert.Equal(t, 1, c.reconcilers.KeySize(resourceKind))
		ir1, _ = c.reconcilers.ItemAt(resourceKind, 0)
		assert.Equal(t, r2, ir1.handler)
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
		assert.Equal(t, i, ii1.Informer)
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
		assert.Equal(t, i1, ii1.Informer)
		err = c.AddInformer(i2, k)
		assert.Nil(t, err)
		assert.Equal(t, 2, c.informers.KeySize(k))
		ii1, _ = c.informers.ItemAt(k, 0)
		ii2, _ := c.informers.ItemAt(k, 1)
		assert.Equal(t, i1, ii1.Informer)
		assert.Equal(t, i2, ii2.Informer)
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
		ctx := t.Context()
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

func TestInformerController_Run_HandleEvents(t *testing.T) {
	t.Run("reconciler update", func(t *testing.T) {
		kind := "foo"
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
		wg.Add(1)
		reconcileAction := ReconcileActionUnknown
		c.AddReconciler(&SimpleReconciler{
			ReconcileFunc: func(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
				reconcileAction = request.Action
				wg.Done()
				return ReconcileResult{}, nil
			},
		}, kind)
		c.AddInformer(inf, kind)

		// Run
		ctx, cancel := context.WithCancel(context.Background())
		go c.Run(ctx)
		oldObj := emptyObject.Copy()
		newObj := emptyObject.Copy()
		newObj.SetResourceVersion("321") // Different RV
		inf.FireUpdate(context.Background(), oldObj, newObj)
		wg.Wait()
		cancel()
		assert.Equal(t, ReconcileActionUpdated, reconcileAction)
	})
	t.Run("reconciler cache resync", func(t *testing.T) {
		kind := "foo"
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
		wg.Add(1)
		reconcileAction := ReconcileActionUnknown
		c.AddReconciler(&SimpleReconciler{
			ReconcileFunc: func(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
				reconcileAction = request.Action
				wg.Done()
				return ReconcileResult{}, nil
			},
		}, kind)
		c.AddInformer(inf, kind)

		// Run
		ctx, cancel := context.WithCancel(context.Background())
		go c.Run(ctx)
		oldObj := emptyObject.Copy()
		newObj := emptyObject.Copy()
		inf.FireUpdate(context.Background(), oldObj, newObj)
		wg.Wait()
		cancel()
		assert.Equal(t, ReconcileActionResynced, reconcileAction)
	})
}

func TestInformerController_Run_BackoffRetry(t *testing.T) {
	// The backoff retry test needs to take at least 16 seconds to run properly, so it's isolated to its own function
	// to avoid the often-used default of a 30-second-timeout on tests affecting other retry tests which take a few seconds each to run
	addError := errors.New("I AM ERROR")
	var addAttempt atomic.Int64
	updateError := errors.New("JE SUIS ERROR")
	var updateAttempt atomic.Int64
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
				attempt:     int(addAttempt.Load()),
				lastAttempt: lastAdd,
			}
			lastAdd = time.Now()
			addAttempt.Add(1)
			return addError
		},
		UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
			updateChan <- chanTuple{
				attempt:     int(updateAttempt.Load()),
				lastAttempt: lastUpdate,
			}
			lastUpdate = time.Now()
			updateAttempt.Add(1)
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
			assert.GreaterOrEqual(t, time.Since(tpl.lastAttempt), time.Duration(math.Pow(2, float64(tpl.attempt-1)))*baseInterval-jitter)
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
			assert.GreaterOrEqual(t, time.Since(tpl.lastAttempt), time.Duration(math.Pow(2, float64(tpl.attempt-1)))*baseInterval-jitter)
		}
	}()

	inf.FireAdd(context.Background(), emptyObject)
	inf.FireUpdate(context.Background(), nil, emptyObject)

	assert.True(t, waitOrTimeout(&addWG, time.Second*30), fmt.Sprintf("timed out waiting for adds to finish (saw %d adds)", addAttempt.Load()))
	assert.True(t, waitOrTimeout(&updateWG, time.Second*30), fmt.Sprintf("timed out waiting for updates to finish (saw %d updates)", updateAttempt.Load()))
	cancel()
	wg.Wait()
}

func TestInformerController_Run_WithRetries(t *testing.T) {
	// Because these tests take more time, isolate them to their own test function
	t.Run("linear retry, update call interrupts add retry", func(t *testing.T) {
		addError := errors.New("I AM ERROR")
		var addAttempt atomic.Int64
		var updateAttempt atomic.Int64
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
				addCh <- int(addAttempt.Load())
				addAttempt.Add(1)
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateAttempt.Add(1)
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
		assert.LessOrEqual(t, int64(3), addAttempt.Load())
		assert.GreaterOrEqual(t, int64(4), addAttempt.Load())
		assert.Equal(t, int64(1), updateAttempt.Load())
	})

	t.Run("linear retry, successful retry stops new retries", func(t *testing.T) {
		addError := errors.New("I AM ERROR")
		var addAttempt atomic.Int64
		updateError := errors.New("JE SUIS ERROR")
		var updateAttempt atomic.Int64
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
				addCh <- int(addAttempt.Load())
				if addAttempt.Add(1) >= 2 {
					return nil
				}
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateCh <- int(updateAttempt.Load())
				if updateAttempt.Add(1) >= 2 {
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
		assert.Equal(t, int64(2), addAttempt.Load())
		assert.Equal(t, int64(2), updateAttempt.Load())
	})

	t.Run("linear retry, retry halts when limit reached", func(t *testing.T) {
		addError := errors.New("I AM ERROR")
		var addAttempt atomic.Int64
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
				addCh <- int(addAttempt.Load())
				addAttempt.Add(1)
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
		assert.Equal(t, int64(4), addAttempt.Load())
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
		ctx := t.Context()
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
		ctx := t.Context()
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
		for range 2 {
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
	deletedAt := metav1.Now()
	live := &resource.TypedSpecObject[string]{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	terminating := live.Copy()
	terminating.SetDeletionTimestamp(&deletedAt)
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
		{
			// A deletion need not change the generation, so the deletion timestamp alone has to dequeue the retry.
			name:        "deletion, retry of the earlier state with the same generation, should dequeue",
			newAction:   ResourceActionUpdate,
			newObject:   terminating,
			retryAction: ResourceActionUpdate,
			retryObject: live,
			expected:    true,
		},
		{
			name:        "deletion, create retry of the earlier state, should dequeue",
			newAction:   ResourceActionUpdate,
			newObject:   terminating,
			retryAction: ResourceActionCreate,
			retryObject: live,
			expected:    true,
		},
		{
			// A failed cleanup's own retry holds the deleting object.
			name:        "deletion, retry of the deletion, shouldn't dequeue",
			newAction:   ResourceActionUpdate,
			newObject:   terminating,
			retryAction: ResourceActionUpdate,
			retryObject: terminating.Copy(),
			expected:    false,
		},
		{
			// Retries are keyed by namespace and name, so a delete retry belongs to an earlier object of that name.
			name:        "deletion, delete retry, shouldn't dequeue",
			newAction:   ResourceActionUpdate,
			newObject:   terminating,
			retryAction: ResourceActionDelete,
			retryObject: live,
			expected:    false,
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

// newTestControllerWithProcessor creates an InformerController using the new RetryProcessor path.
// WorkerPoolSize is set to 1 for deterministic ordering in tests.
func newTestControllerWithProcessor(retryPolicy RetryPolicy, checkInterval time.Duration) *InformerController {
	cfg := InformerControllerConfig{
		RetryPolicy: retryPolicy,
		RetryProcessorConfig: &RetryProcessorConfig{
			WorkerPoolSize: 1, // deterministic ordering for tests
			CheckInterval:  checkInterval,
		},
	}
	return NewInformerController(cfg)
}

func TestInformerController_Run_WithRetryProcessor(t *testing.T) {
	t.Run("watcher error, one retry via processor", func(t *testing.T) {
		kind := "foo"
		var addCalls atomic.Int64
		reconcileCalls := 0
		inf := &testInformer{}
		c := newTestControllerWithProcessor(func(err error, attempt int) (bool, time.Duration) {
			if attempt >= 1 {
				return false, 0
			}
			return true, time.Millisecond * 10
		}, time.Millisecond*50)
		wg := sync.WaitGroup{}
		wg.Add(2)
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				n := addCalls.Add(1)
				require.LessOrEqual(t, n, int64(2), "Add should only be retried once based on the RetryPolicy")
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

		ctx, cancel := context.WithCancel(context.Background())
		go c.Run(ctx)
		inf.FireAdd(context.Background(), emptyObject)
		assert.True(t, waitOrTimeout(&wg, 5*time.Second), "timed out waiting for watcher retries via processor")
		cancel()
		assert.Equal(t, int64(2), addCalls.Load())
		assert.Equal(t, 1, reconcileCalls)
	})

	t.Run("reconciler error, one retry via processor", func(t *testing.T) {
		kind := "foo"
		addCalls := 0
		var reconcileCalls atomic.Int64
		inf := &testInformer{}
		c := newTestControllerWithProcessor(func(err error, attempt int) (bool, time.Duration) {
			if attempt > 1 {
				return false, 0
			}
			return true, time.Millisecond * 10
		}, time.Millisecond*50)
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
				n := reconcileCalls.Add(1)
				_ = n
				wg.Done()
				return ReconcileResult{}, errors.New("I AM ERROR")
			},
		}, kind)
		c.AddInformer(inf, kind)

		ctx, cancel := context.WithCancel(context.Background())
		go c.Run(ctx)
		inf.FireAdd(context.Background(), emptyObject)
		assert.True(t, waitOrTimeout(&wg, 5*time.Second), "timed out waiting for reconciler retries via processor")
		cancel()
		assert.Equal(t, 1, addCalls)
		assert.Equal(t, int64(2), reconcileCalls.Load())
	})
}

func TestInformerController_Run_WithRetryProcessorDequeue(t *testing.T) {
	t.Run("update dequeues stale add retry", func(t *testing.T) {
		kind := "foo"
		addError := errors.New("I AM ERROR")
		var addCalls atomic.Int64
		var updateCalls atomic.Int64
		inf := &testInformer{}
		// Use a long retry delay so the retry is sitting in the queue when we fire update
		c := newTestControllerWithProcessor(func(err error, attempt int) (bool, time.Duration) {
			return true, time.Second * 10
		}, time.Millisecond*50)
		// No RetryDequeuePolicy means all retries are dequeued on new events
		c.RetryDequeuePolicy = nil
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addCalls.Add(1)
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateCalls.Add(1)
				return nil
			},
		}, kind)
		c.AddInformer(inf, kind)

		ctx := t.Context()
		go c.Run(ctx)

		// Fire an add that fails - this queues a retry
		inf.FireAdd(context.Background(), emptyObject)
		assert.Equal(t, int64(1), addCalls.Load())

		// Fire an update - this should dequeue the pending add retry
		inf.FireUpdate(context.Background(), nil, emptyObject)
		assert.Equal(t, int64(1), updateCalls.Load())

		// Wait a bit and verify no additional add retries fire
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, int64(1), addCalls.Load(), "add retry should have been dequeued by update")
	})

	t.Run("deletion dequeues a stale resync retry", func(t *testing.T) {
		// A failed resync queues a retry holding the object as it was. Once the object is being deleted and cleanup
		// has run, replaying that retry would call Sync again and could recreate what cleanup removed. Another owner's
		// finalizer keeps the object terminating, so no delete event comes to dequeue it, and the deletion update
		// doesn't change the generation.
		kind := "foo"
		inf := &testInformer{}
		// The retry delay is long so the retry never runs during the test, which checks the queue instead.
		c := newTestControllerWithProcessor(func(err error, attempt int) (bool, time.Duration) {
			return true, time.Second * 10
		}, time.Millisecond*50)
		c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy
		sch := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
		obj := sch.ZeroValue()
		obj.SetName("test")
		obj.SetGeneration(1)
		obj.SetResourceVersion("1")
		obj.SetFinalizers([]string{DefaultFinalizerSupplier(sch), "other.example/cleanup"})
		watcher, server := newTestWatcherWithServer(t, sch, obj)
		watcher.SyncFunc = func(context.Context, resource.Object) error {
			return errors.New("downstream unavailable")
		}
		var deleteCalls atomic.Int64
		watcher.DeleteFunc = func(context.Context, resource.Object) error {
			deleteCalls.Add(1)
			return nil
		}
		require.NoError(t, c.AddWatcher(watcher, kind))
		require.NoError(t, c.AddInformer(inf, kind))
		go c.Run(t.Context())

		// A resync delivers the object unchanged.
		inf.FireUpdate(t.Context(), obj, obj)
		require.Equal(t, 1, c.retryProcessor.Len(), "the failed resync should have queued a retry")

		deletion := server.markDeleting()
		inf.FireUpdate(t.Context(), deletion[0], deletion[1])
		assert.Equal(t, int64(1), deleteCalls.Load())
		assert.Equal(t, []string{"other.example/cleanup"}, server.state().GetFinalizers())
		assert.Zero(t, c.retryProcessor.Len(), "the deletion should have dequeued the stale resync retry")
	})
}

func waitForSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatal(failure)
	}
}

func TestInformerController_Run_OrdersDeletionAndResyncRetry(t *testing.T) {
	for _, test := range []struct {
		name          string
		concurrent    bool
		blockOtherKey bool
	}{
		{name: "ready retry behind a different key", concurrent: true, blockOtherKey: true},
		{name: "running retry for the deleting object", concurrent: true},
		{name: "default ticker checks the completed retry error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const kind = "foo"
				const otherFinalizer = "other.example/cleanup"
				retryPolicy := func(_ error, attempt int) (bool, time.Duration) {
					if attempt == 0 {
						return true, 0
					}
					return true, time.Hour
				}
				var c *InformerController
				if test.concurrent {
					c = newTestControllerWithProcessor(retryPolicy, time.Second)
				} else {
					c = NewInformerController(InformerControllerConfig{RetryPolicy: retryPolicy})
				}
				sch := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
				obj := sch.ZeroValue()
				obj.SetName("test")
				obj.SetGeneration(1)
				obj.SetResourceVersion("1")
				obj.SetFinalizers([]string{DefaultFinalizerSupplier(sch), otherFinalizer})
				watcher, server := newTestWatcherWithServer(t, sch, obj)

				started := make(chan struct{})
				unblock := make(chan struct{})
				release := sync.OnceFunc(func() { close(unblock) })
				defer release()
				var syncCalls atomic.Int64
				var externalState atomic.Bool
				syncErr := errors.New("resync failed")
				retryErr := errors.New("resync failed again")
				c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy
				var latestFailureSeen atomic.Bool
				if !test.concurrent {
					c.RetryDequeuePolicy = func(action ResourceAction, current resource.Object, retryAction ResourceAction, retryObj resource.Object, err error) bool {
						// A custom policy can distinguish the retry's failure from
						// the error that originally enqueued it.
						latestFailureSeen.Store(errors.Is(err, retryErr))
						return errors.Is(err, retryErr) && OpinionatedRetryDequeuePolicy(action, current, retryAction, retryObj, err)
					}
				}
				watcher.SyncFunc = func(context.Context, resource.Object) error {
					if syncCalls.Add(1) == 2 {
						if !test.blockOtherKey {
							close(started)
							<-unblock
						}
						externalState.Store(true)
						return retryErr
					}
					return syncErr
				}
				watcher.DeleteFunc = func(context.Context, resource.Object) error {
					externalState.Store(false)
					return nil
				}
				inf := &testInformer{}
				require.NoError(t, c.AddWatcher(watcher, kind))
				require.NoError(t, c.AddInformer(inf, kind))

				// Queue both due requests before the single worker starts. The
				// earlier request must run first and block the target's retry.
				if test.blockOtherKey {
					c.retryProcessor.Enqueue(RetryRequest{
						Key:        "other-key",
						RetryAfter: time.Now().Add(-time.Second),
						RetryFunc: func() (*time.Duration, error) {
							close(started)
							<-unblock
							return nil, nil
						},
					})
				}
				inf.FireUpdate(t.Context(), obj, obj)
				require.EqualValues(t, 1, syncCalls.Load())
				if test.blockOtherKey {
					require.Equal(t, 2, c.retryProcessor.Len())
				}

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				go c.Run(ctx)
				waitForSignal(t, started, "the blocking retry did not start")

				// The other finalizer keeps the resource terminating. A raw
				// delete cannot mask a failure to invalidate stale retries.
				externalState.Store(true)
				deletion := server.markDeleting()
				delivering := make(chan struct{})
				cleaned := make(chan struct{})
				go func() {
					close(delivering)
					inf.FireUpdate(ctx, deletion[0], deletion[1])
					close(cleaned)
				}()
				<-delivering
				synctest.Wait()
				var cleanupRan bool
				select {
				case <-cleaned:
					cleanupRan = true
				default:
				}
				assert.Equal(t, test.blockOtherKey, cleanupRan,
					"cleanup should wait only for a retry of the same object")
				release()
				<-cleaned
				synctest.Wait()

				assert.False(t, externalState.Load(), "stale resync recreated state after cleanup")
				assert.Equal(t, []string{otherFinalizer}, server.state().GetFinalizers())
				if test.concurrent {
					assert.Zero(t, c.retryProcessor.Len(), "stale resync should not be rescheduled")
				} else {
					assert.True(t, latestFailureSeen.Load(), "dequeue should see the rescheduled attempt")
					assert.Zero(t, c.toRetry.Size(), "stale resync should not be rescheduled")
				}
			})
		})
	}
}

func TestInformerController_Run_LegacyRetryHandoff(t *testing.T) {
	for _, test := range []struct {
		name       string
		policy     RetryDequeuePolicy
		hardDelete bool
		remove     bool
	}{
		{name: "hard delete discards the failed retry", hardDelete: true},
		{name: "ordinary watcher update waits for the retry", policy: OpinionatedRetryDequeuePolicy},
		{name: "informer removal cancels deferred reconcile", policy: OpinionatedRetryDequeuePolicy, remove: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := NewInformerController(InformerControllerConfig{
				RetryDequeuePolicy: test.policy,
				RetryPolicy:        func(error, int) (bool, time.Duration) { return true, 0 },
			})
			c.retryTickerInterval = time.Millisecond
			inf := &testInformer{}
			require.NoError(t, c.AddInformer(inf, "kind"))
			obj := &resource.TypedSpecObject[string]{}
			obj.SetName("item")
			obj.SetUID("original")
			obj.SetResourceVersion("1")

			started, unblock := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(unblock) })
			var adds atomic.Int64
			add := func(context.Context, resource.Object) error {
				switch adds.Add(1) {
				case 1:
					return errors.New("schedule retry")
				case 2:
					close(started)
					<-unblock
					if test.hardDelete {
						return errors.New("superseded by delete")
					}
				}
				return nil
			}
			deliveries := make(chan string, 3)
			observe := func(_ context.Context, target resource.Object) error {
				deliveries <- target.GetName()
				return nil
			}
			if test.remove {
				require.NoError(t, c.AddReconciler(&SimpleReconciler{ReconcileFunc: func(ctx context.Context, req ReconcileRequest) (ReconcileResult, error) {
					if req.Action == ReconcileActionCreated {
						return ReconcileResult{}, add(ctx, req.Object)
					}
					return ReconcileResult{}, observe(ctx, req.Object)
				}}, "kind"))
			} else {
				require.NoError(t, c.AddWatcher(&SimpleWatcher{AddFunc: add, DeleteFunc: observe, UpdateFunc: func(ctx context.Context, _, target resource.Object) error {
					return observe(ctx, target)
				}}, "kind"))
			}
			ctx, stop := context.WithCancel(t.Context())
			stopped := make(chan struct{})
			go func() { defer close(stopped); _ = c.Run(ctx) }()
			defer func() { stop(); release(); <-stopped }()
			source, _ := c.informers.ItemAt("kind", 0)
			require.Eventually(t, func() bool { return source.run.Load() != nil }, time.Second, time.Millisecond)
			handler := inf.handlers[0]
			require.NoError(t, handler.Add(ctx, obj))
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("Add retry did not start")
			}
			// An unrelated object can still reach the same registration.
			other := obj.Copy()
			other.SetName("other")
			other.SetUID("other")
			require.NoError(t, handler.Delete(ctx, other))
			require.Equal(t, "other", <-deliveries)

			next := obj.Copy()
			next.SetResourceVersion("2")
			returned := make(chan error, 1)
			go func() {
				returned <- withInformerEventTimeout(ctx, 10*time.Millisecond, func(event context.Context) error {
					if test.hardDelete {
						return handler.Delete(event, next)
					}
					return handler.Update(event, obj, next)
				})
			}()
			select {
			case err := <-returned:
				require.ErrorIs(t, err, context.DeadlineExceeded)
			case <-time.After(time.Second):
				t.Fatal("expired delivery is still blocked by the retry")
			}
			require.Empty(t, deliveries)
			c.deliveryMu.Lock()
			order := c.deliveries[deliveryKey{"kind", obj.GetStaticMetadata().Identifier()}]
			c.deliveryMu.Unlock()
			require.NotNil(t, order)
			if test.remove {
				c.RemoveInformer(inf, "kind")
				require.Eventually(t, func() bool { return source.run.Load() == nil }, time.Second, time.Millisecond)
			}
			release()
			select {
			case <-order.done:
			case <-time.After(time.Second):
				t.Fatal("deferred delivery did not finish")
			}
			// Settlement removes the key. A discarded failure cannot run again.
			require.Eventually(t, func() bool { return c.toRetry.Size() == 0 }, time.Second, time.Millisecond)
			require.EqualValues(t, 2, adds.Load())
			if test.remove {
				require.Empty(t, deliveries)
			} else {
				require.Len(t, deliveries, 1)
				require.Equal(t, "item", <-deliveries)
			}
		})
	}
}

func TestInformerController_RetryTicker_DequeueDuringBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const key = "watcher:item"
		oldErr, retryErr := errors.New("first failure"), errors.New("retry failure")
		c := NewInformerController(InformerControllerConfig{
			RetryPolicy: func(error, int) (bool, time.Duration) { return true, 0 },
			RetryDequeuePolicy: func(_ ResourceAction, obj resource.Object, action ResourceAction, _ resource.Object, err error) bool {
				// The waiting Create is already obsolete. Decide the running
				// Update's fate only when its latest error is available.
				return obj.GetResourceVersion() == "2" && (action == ResourceActionCreate || errors.Is(err, retryErr))
			},
		})
		obj := &resource.TypedSpecObject[string]{}
		obj.SetResourceVersion("1")
		entered, unblock := make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(unblock) })
		defer release()
		var calls []string
		enqueue := func(action ResourceAction, run func() (*time.Duration, error)) {
			c.enqueueRetry(RetryRequest{Key: key, Action: action, Object: obj, LastError: oldErr, RetryAfter: time.Now(), RetryFunc: run}, nil)
		}
		enqueue(ResourceActionUpdate, func() (*time.Duration, error) {
			calls = append(calls, "running")
			if len(calls) == 1 {
				close(entered)
				<-unblock
			}
			return nil, retryErr
		})
		enqueue(ResourceActionCreate, func() (*time.Duration, error) {
			calls = append(calls, "superseded")
			return nil, nil
		})
		ctx, stop := context.WithCancel(t.Context())
		defer stop()
		go c.retryTicker(ctx)
		<-entered
		event := obj.Copy()
		event.SetResourceVersion("2")
		done := c.dequeueIfRequired(key, event, ResourceActionUpdate)
		require.Len(t, done, 1)
		// Mutation after dequeue must not change the saved decision. Work
		// enqueued subsequently for the same key must survive that decision.
		event.SetResourceVersion("3")
		enqueue(ResourceActionCreate, func() (*time.Duration, error) {
			calls = append(calls, "new")
			return nil, nil
		})
		release()
		<-done[0]
		time.Sleep(c.retryTickerInterval)
		synctest.Wait()
		require.Equal(t, []string{"running", "new"}, calls)
		require.Zero(t, c.toRetry.Size())
	})
}

func TestInformerController_Run_AddRetryAndPendingUpdate(t *testing.T) {
	for _, test := range []struct {
		name             string
		retryFails       bool
		cancelSpecChange bool
		cancelNextUpdate bool
	}{
		{name: "completed retry suppresses pending update"},
		{name: "failed retry permits recovery", retryFails: true},
		{name: "live update follows deferred catch-up", cancelSpecChange: true},
		{name: "update expiring behind catch-up still runs in order", cancelSpecChange: true, cancelNextUpdate: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const kind = "foo"
				// Admit one retry of the initial Add. A canceled delivery has
				// never run and must survive even if the policy declines its error.
				initialErr := errors.New("initial add failed")
				catchupErr := errors.New("catch-up failed")
				c := newTestControllerWithProcessor(func(err error, attempt int) (bool, time.Duration) {
					if errors.Is(err, catchupErr) {
						return attempt == 0, time.Hour
					}
					return errors.Is(err, initialErr) && attempt == 0, 0
				}, time.Second)
				c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy
				schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
				obj := schema.ZeroValue()
				obj.SetName("test")
				obj.SetUID("original")
				obj.SetResourceVersion("1")
				obj.SetGeneration(1)
				require.NoError(t, obj.SetSpec("original"))
				watcher, server := newTestWatcherWithServer(t, schema, obj)
				started := make(chan struct{})
				unblock := make(chan struct{})
				release := sync.OnceFunc(func() { close(unblock) })
				defer release()
				updated := make(chan struct{}, 1)
				finishCatchup := make(chan struct{})
				releaseCatchup := sync.OnceFunc(func() { close(finishCatchup) })
				defer releaseCatchup()
				applied := make(chan int64, 2)
				var addCalls, updateCalls atomic.Int64
				watcher.AddFunc = func(context.Context, resource.Object) error {
					switch addCalls.Add(1) {
					case 1:
						return initialErr
					case 2:
						close(started)
						<-unblock
						if test.retryFails {
							return errors.New("retry failed")
						}
					}
					return nil
				}
				watcher.UpdateFunc = func(ctx context.Context, old, current resource.Object) error {
					assert.NoError(t, ctx.Err())
					if current.GetGeneration() == 2 {
						assert.Equal(t, "original", old.GetSpec())
						assert.Equal(t, "changed", current.GetSpec())
					} else {
						assert.Equal(t, "changed", old.GetSpec())
						assert.Equal(t, "latest", current.GetSpec())
					}
					assert.Equal(t, server.state().GetResourceVersion(), current.GetResourceVersion())
					updateCalls.Add(1)
					if current.GetGeneration() == 2 {
						updated <- struct{}{}
						<-finishCatchup
					}
					applied <- current.GetGeneration()
					if test.cancelNextUpdate && current.GetGeneration() == 2 {
						// Generation 3 was delivered before this retry was queued.
						// It must still supersede the failed catch-up.
						return catchupErr
					}
					return nil
				}
				inf := &testInformer{}
				require.NoError(t, c.AddWatcher(watcher, kind))
				require.NoError(t, c.AddInformer(inf, kind))
				runCtx, stop := context.WithCancel(t.Context())
				defer stop()
				go c.Run(runCtx)
				inf.FireAdd(runCtx, obj)
				waitForSignal(t, started, "Add retry did not start")
				require.EqualValues(t, 2, addCalls.Load())
				change := server.changesFrom(0)[0]
				if test.cancelSpecChange {
					change = server.change(func(obj resource.Object) {
						obj.SetGeneration(2)
						require.NoError(t, obj.SetSpec("changed"))
					})
				}
				eventCtx, cancel := context.WithCancel(runCtx)
				defer cancel()
				done := make(chan struct{})
				go func() { inf.FireUpdate(eventCtx, change[0], change[1]); close(done) }()
				synctest.Wait()
				require.EqualValues(t, 2, addCalls.Load(), "pending update overlapped the running retry")
				if test.cancelSpecChange {
					cancel()
					synctest.Wait()
					select {
					case <-done:
					default:
						t.Fatal("canceled delivery is still waiting")
					}
				}
				release()
				<-done
				if test.cancelSpecChange {
					waitForSignal(t, updated, "spec update did not reach the watcher")
					// The add has installed its finalizer. This notification takes
					// the normal Update path while the catch-up is still writing.
					next := server.change(func(obj resource.Object) {
						obj.SetGeneration(3)
						require.NoError(t, obj.SetSpec("latest"))
					})
					nextCtx, cancelNext := context.WithCancel(runCtx)
					defer cancelNext()
					result := make(chan error, 1)
					go func() { result <- inf.handlers[0].Update(nextCtx, next[0], next[1]) }()
					synctest.Wait()
					require.EqualValues(t, 1, updateCalls.Load(), "generation 3 overtook the catch-up")
					if test.cancelNextUpdate {
						cancelNext()
						require.ErrorIs(t, <-result, context.Canceled)
					}
					releaseCatchup()
					if !test.cancelNextUpdate {
						require.NoError(t, <-result)
					}
					synctest.Wait()
					require.Len(t, applied, 2)
					assert.EqualValues(t, 2, <-applied)
					assert.EqualValues(t, 3, <-applied)
					assert.Zero(t, c.retryProcessor.Len(), "superseded catch-up must not be retried")
				}
				synctest.Wait()
				if test.retryFails {
					assert.EqualValues(t, 3, addCalls.Load())
				} else {
					assert.EqualValues(t, 2, addCalls.Load())
				}
				assert.Equal(t, []string{watcher.finalizer}, server.state().GetFinalizers())
				if test.cancelSpecChange {
					assert.EqualValues(t, 2, updateCalls.Load())
				} else {
					assert.Zero(t, updateCalls.Load())
				}
			})
		})
	}
}

func TestInformerController_Update_FinalizerRemovalPreservesEventVersion(t *testing.T) {
	schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
	obj := schema.ZeroValue()
	obj.SetName("item")
	obj.SetUID("original")
	obj.SetResourceVersion("1")
	obj.SetGeneration(1)
	// The second finalizer keeps the resource alive for the watch notification
	// produced by removing our own finalizer.
	obj.SetFinalizers([]string{DefaultFinalizerSupplier(schema), "example/other"})
	watcher, server := newTestWatcherWithServer(t, schema, obj)
	c := NewInformerController(DefaultInformerControllerConfig())
	inf := &testInformer{}
	require.NoError(t, c.AddInformer(inf, "kind"))
	require.NoError(t, c.AddWatcher(watcher, "kind"))
	var actions []ReconcileAction
	require.NoError(t, c.AddReconciler(&SimpleReconciler{ReconcileFunc: func(_ context.Context, req ReconcileRequest) (ReconcileResult, error) {
		actions = append(actions, req.Action)
		return ReconcileResult{}, nil
	}}, "kind"))

	deletion := server.markDeleting()
	original := deletion[1].Copy()
	inf.FireUpdate(t.Context(), deletion[0], deletion[1])
	require.Equal(t, original, deletion[1])
	// A shared informer reuses the last event's new object as the next old one.
	inf.FireUpdate(t.Context(), deletion[1], server.state())
	require.Equal(t, []ReconcileAction{ReconcileActionUpdated, ReconcileActionUpdated}, actions)
}

func TestInformerController_Run_InterruptedUpdateBeforeDelete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestControllerWithProcessor(func(_ error, attempt int) (bool, time.Duration) { return attempt == 0, 0 }, time.Second)
		c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy
		schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
		obj := schema.ZeroValue()
		obj.SetName("item")
		obj.SetUID("original")
		obj.SetGeneration(1)
		obj.SetResourceVersion("1")
		obj.SetFinalizers([]string{DefaultFinalizerSupplier(schema)})
		watcher, _ := newTestWatcherWithServer(t, schema, obj)
		started, unblock := make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(unblock) })
		defer release()
		var syncCalls int
		watcher.SyncFunc = func(context.Context, resource.Object) error {
			syncCalls++
			if syncCalls == 1 {
				return errors.New("sync failed")
			}
			close(started)
			<-unblock
			return nil
		}
		var calls []string
		watcher.UpdateFunc = func(context.Context, resource.Object, resource.Object) error {
			calls = append(calls, "owner update")
			return nil
		}
		later := &SimpleWatcher{
			UpdateFunc: func(_ context.Context, _, obj resource.Object) error {
				if obj.GetGeneration() == 2 {
					calls = append(calls, "observer update")
				}
				return nil
			},
			DeleteFunc: func(context.Context, resource.Object) error {
				calls = append(calls, "observer delete")
				return nil
			},
		}
		// A handler that completed before the timeout must not be replayed.
		require.NoError(t, c.AddWatcher(&SimpleWatcher{UpdateFunc: func(_ context.Context, _, target resource.Object) error {
			if target.GetGeneration() == 2 {
				calls = append(calls, "first update")
			}
			return nil
		}}, "kind"))
		require.NoError(t, c.AddWatcher(watcher, "kind"))
		require.NoError(t, c.AddWatcher(later, "kind"))
		require.NoError(t, c.AddReconciler(&SimpleReconciler{ReconcileFunc: func(_ context.Context, req ReconcileRequest) (ReconcileResult, error) {
			if req.Action != ReconcileActionResynced {
				calls = append(calls, "reconcile "+string(ResourceActionFromReconcileAction(req.Action)))
			}
			return ReconcileResult{}, nil
		}}, "kind"))
		inf := &testInformer{}
		require.NoError(t, c.AddInformer(inf, "kind"))
		ctx, stop := context.WithCancel(t.Context())
		defer stop()
		go c.Run(ctx)
		synctest.Wait()
		handler := inf.handlers[0]
		require.NoError(t, handler.Update(ctx, obj, obj))
		<-started
		next := obj.Copy()
		next.SetGeneration(2)
		next.SetResourceVersion("2")
		require.NoError(t, next.SetSpec("changed"))
		// Both deliveries expire behind the retry. Neither notification is
		// replayed by the informer, including the final hard delete.
		require.ErrorIs(t, withInformerEventTimeout(ctx, time.Second, func(ctx context.Context) error {
			return handler.Update(ctx, obj, next)
		}), context.DeadlineExceeded)
		require.ErrorIs(t, withInformerEventTimeout(ctx, time.Second, func(ctx context.Context) error {
			return handler.Delete(ctx, next)
		}), context.DeadlineExceeded)
		require.Equal(t, []string{"first update"}, calls)
		release()
		synctest.Wait()
		require.Equal(t, []string{"first update", "owner update", "observer update", "reconcile UPDATE", "observer delete", "reconcile DELETE"}, calls)
	})
}

func TestInformerController_Run_TerminatingUpdateOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := NewInformerController(DefaultInformerControllerConfig())
		inf := &testInformer{}
		require.NoError(t, c.AddInformer(inf, "kind"))
		started, unblock := make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(unblock) })
		defer release()
		completed := make(chan string, 2)
		require.NoError(t, c.AddWatcher(&SimpleWatcher{UpdateFunc: func(_ context.Context, _, obj resource.Object) error {
			if obj.GetResourceVersion() == "2" {
				close(started)
				<-unblock
			}
			completed <- obj.GetResourceVersion()
			return nil
		}}, "kind"))
		ctx, stop := context.WithCancel(t.Context())
		defer stop()
		go c.Run(ctx)
		synctest.Wait()
		handler := inf.handlers[0]
		old := &resource.TypedSpecObject[string]{}
		old.SetName("item")
		old.SetUID("original")
		old.SetResourceVersion("1")
		old.SetFinalizers([]string{"example/first", "example/second"})
		obj := old.Copy()
		now := metav1.Now()
		obj.SetDeletionTimestamp(&now)
		obj.SetResourceVersion("2")
		expired, cancel := context.WithCancel(ctx)
		cancel()
		require.ErrorIs(t, handler.Update(expired, old, obj), context.Canceled)
		<-started

		// Another finalizer's removal can deliver a newer version while
		// the observer is still handling the first terminating snapshot.
		next := obj.Copy()
		next.SetResourceVersion("3")
		next.SetFinalizers([]string{"example/second"})
		delivered := make(chan error, 1)
		go func() { delivered <- handler.Update(ctx, obj, next) }()
		synctest.Wait()
		select {
		case version := <-completed:
			t.Fatalf("observer completed version %s while version 2 was blocked", version)
		default:
		}
		release()
		require.NoError(t, <-delivered)
		require.Equal(t, "2", <-completed)
		require.Equal(t, "3", <-completed)
	})
}

func TestInformerController_Run_GenerationZeroSyncRetry(t *testing.T) {
	for _, test := range []struct {
		name                           string
		startup, running, metadataOnly bool
	}{
		{name: "queued cache sync follows a spec change"},
		{name: "queued sync survives a metadata change", metadataOnly: true},
		{name: "running startup sync finishes before the update", startup: true, running: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := newTestControllerWithProcessor(func(_ error, attempt int) (bool, time.Duration) {
					return attempt == 0, time.Second
				}, time.Second)
				c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy
				// ConfigMap data lives outside spec and storage omits generation.
				// Neither GetSpec nor generation can tell these versions apart.
				type configMap = resource.WrappedObject[*corev1.ConfigMap]
				obj := resource.NewWrappedObject(&corev1.ConfigMap{Data: map[string]string{"value": "original"}})
				schema := resource.NewSimpleSchema("", "v1", obj, &resource.TypedList[*configMap]{}, resource.WithKind("ConfigMap"))
				value := func(obj resource.Object) string { return obj.(*configMap).Object.Data["value"] }
				obj.SetName("item")
				obj.SetUID("original")
				obj.SetResourceVersion("1")
				obj.SetFinalizers([]string{DefaultFinalizerSupplier(schema)})
				watcher, err := NewOpinionatedWatcher(schema, newFinalizerAPIServer(obj), OpinionatedWatcherConfig{})
				require.NoError(t, err)
				started, unblock := make(chan struct{}), make(chan struct{})
				release := sync.OnceFunc(func() { close(unblock) })
				defer release()
				// Record both callback kind and payload. A metadata-only change
				// still needs the failed Sync even if Update has already succeeded.
				var calls []string
				var mu sync.Mutex
				record := func(action string, obj resource.Object) {
					mu.Lock()
					defer mu.Unlock()
					calls = append(calls, fmt.Sprintf("%s %s %s", action, obj.GetResourceVersion(), value(obj)))
				}
				var syncCalls atomic.Int64
				watcher.SyncFunc = func(_ context.Context, target resource.Object) error {
					if syncCalls.Add(1) == 1 {
						return errors.New("sync failed")
					}
					if test.running {
						close(started)
						<-unblock
					}
					record("sync", target)
					return nil
				}
				watcher.UpdateFunc = func(_ context.Context, _, target resource.Object) error {
					record("update", target)
					return nil
				}
				inf := &testInformer{}
				require.NoError(t, c.AddInformer(inf, "kind"))
				require.NoError(t, c.AddWatcher(watcher, "kind"))
				ctx, stop := context.WithCancel(t.Context())
				defer stop()
				go c.Run(ctx)
				synctest.Wait()
				handler := inf.handlers[0]
				if test.startup {
					require.NoError(t, handler.Add(ctx, obj))
				} else {
					require.NoError(t, handler.Update(ctx, obj, obj))
				}
				if test.running {
					<-started
				}
				next := obj.Copy()
				next.SetResourceVersion("2")
				if test.metadataOnly {
					next.SetLabels(map[string]string{"updated": "true"})
				} else {
					next.(*configMap).Object.Data["value"] = "changed"
				}
				delivered := make(chan error, 1)
				go func() { delivered <- handler.Update(ctx, obj, next) }()
				synctest.Wait()
				if test.running {
					select {
					case <-delivered:
						t.Fatal("update overtook the running Sync")
					default:
					}
				}
				release()
				require.NoError(t, <-delivered)
				time.Sleep(time.Second)
				synctest.Wait()
				if test.running {
					require.Equal(t, []string{"sync 1 original", "update 2 changed"}, calls)
				} else {
					require.Equal(t, []string{"update 2 " + value(next), "sync 2 " + value(next)}, calls)
				}
			})
		})
	}
}

func TestInformerController_RemoveWatcher_CancelsRunningSyncRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runCtx, stop := context.WithCancel(t.Context())
		defer stop()
		c := newTestControllerWithProcessor(func(_ error, attempt int) (bool, time.Duration) {
			return attempt == 0, 0
		}, time.Second)
		schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
		watcher, err := NewOpinionatedWatcher(schema, &mockPatchClient{}, OpinionatedWatcherConfig{})
		require.NoError(t, err)
		obj := schema.ZeroValue()
		obj.SetName("item")
		obj.SetFinalizers([]string{watcher.finalizer})
		started, finished := make(chan struct{}), make(chan struct{})
		calls := 0
		watcher.SyncFunc = func(ctx context.Context, _ resource.Object) error {
			calls++
			if calls == 1 {
				return errors.New("sync failed")
			}
			close(started)
			<-ctx.Done()
			close(finished)
			return nil
		}
		inf := &testInformer{}
		require.NoError(t, c.AddInformer(inf, "kind"))
		require.NoError(t, c.AddWatcher(watcher, "kind"))
		go c.Run(runCtx)
		synctest.Wait()
		// This informer uses its run context for events. Only removing the
		// registration can interrupt the in-flight retry before shutdown.
		inf.FireAdd(runCtx, obj)
		<-started
		c.RemoveWatcher(watcher, "kind")
		synctest.Wait()
		select {
		case <-finished:
		default:
			t.Fatal("watcher removal left its retry running")
		}
		require.NoError(t, runCtx.Err())
	})
}

func TestInformerController_DeletionRetryWaitCancellation(t *testing.T) {
	for _, test := range []struct {
		name   string
		action ResourceAction
	}{
		{"startup without redelivery", ResourceActionCreate},
		{"update without redelivery", ResourceActionUpdate},
		{"redelivery", ResourceActionUpdate},
		{"repeated canceled deliveries", ResourceActionUpdate},
		{"same name recreated", ResourceActionUpdate},
		{"deferred cleanup retries", ResourceActionCreate},
		{"controller stops", ResourceActionUpdate},
		{"informer removed", ResourceActionUpdate},
		{"informer removed during cleanup", ResourceActionUpdate},
		{"watcher removed", ResourceActionUpdate},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const kind = "foo"
				const redeliveries = 10
				stoppedCleanup := errors.New("cleanup stopped")
				c := newTestControllerWithProcessor(func(err error, attempt int) (bool, time.Duration) {
					assert.NotErrorIs(t, err, stoppedCleanup, "work from the removed informer must not schedule a retry")
					return attempt == 0, 0
				}, time.Second)
				c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy

				schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
				obj := schema.ZeroValue()
				obj.SetName("test")
				obj.SetGeneration(1)
				obj.SetResourceVersion("1")
				obj.SetUID("original")
				const secondFinalizer = "example/watcher"
				const reconcilerFinalizer = "example/reconciler"
				obj.SetFinalizers([]string{DefaultFinalizerSupplier(schema), secondFinalizer, reconcilerFinalizer})
				watcher, server := newTestWatcherWithServer(t, schema, obj)

				started := make(chan struct{})
				unblock := make(chan struct{})
				release := sync.OnceFunc(func() { close(unblock) })
				defer release()
				var eventTimeout time.Duration
				if test.name == "deferred cleanup retries" {
					// A periodic LIST reports terminating objects as Adds. Its
					// deadline can expire before the running retry finishes.
					eventTimeout = time.Second
				}
				checkCleanupContext := func(ctx context.Context) {
					t.Helper()
					assert.NoError(t, ctx.Err())
					if eventTimeout > 0 {
						deadline, ok := ctx.Deadline()
						if assert.True(t, ok, "deferred cleanup and its retries need a fresh deadline") {
							assert.Equal(t, eventTimeout, time.Until(deadline))
						}
					}
				}
				var syncCalls atomic.Int64
				watcher.SyncFunc = func(context.Context, resource.Object) error {
					if syncCalls.Add(1) == 1 {
						return errors.New("resync failed")
					}
					close(started)
					<-unblock
					return nil
				}
				var deleteCalls atomic.Int64
				watcher.DeleteFunc = func(ctx context.Context, target resource.Object) error {
					checkCleanupContext(ctx)
					if test.name == "same name recreated" {
						assert.Equal(t, "replacement", string(target.GetUID()))
					}
					if deleteCalls.Add(1) == 1 && test.name == "deferred cleanup retries" {
						return errors.New("cleanup unavailable")
					}
					if test.name == "informer removed during cleanup" {
						<-ctx.Done()
						return stoppedCleanup
					}
					return nil
				}
				require.NoError(t, c.AddWatcher(watcher, kind))
				var laterWatcherCalls atomic.Int64
				require.NoError(t, c.AddWatcher(&SimpleWatcher{
					AddFunc: func(context.Context, resource.Object) error {
						laterWatcherCalls.Add(1)
						return nil
					},
					UpdateFunc: func(_ context.Context, _, target resource.Object) error {
						if target.GetDeletionTimestamp() != nil {
							laterWatcherCalls.Add(1)
						}
						return nil
					},
				}, kind))

				second, err := NewOpinionatedWatcher(schema, server, OpinionatedWatcherConfig{
					Finalizer: func(resource.Schema) string { return secondFinalizer },
				})
				require.NoError(t, err)
				var secondDeletes atomic.Int64
				second.DeleteFunc = func(ctx context.Context, _ resource.Object) error {
					checkCleanupContext(ctx)
					if test.name != "watcher removed" {
						assert.Positive(t, deleteCalls.Load(), "first watcher must dispatch before the second")
					}
					secondDeletes.Add(1)
					return nil
				}
				require.NoError(t, c.AddWatcher(second, kind))
				// The event can be canceled while the retry processor and
				// the older callback are still alive.
				runCtx, stop := context.WithCancel(t.Context())
				defer stop()
				reconciler, err := NewOpinionatedReconciler(server, reconcilerFinalizer)
				require.NoError(t, err)
				var reconcileDeletes atomic.Int64
				reconciler.Wrap(&SimpleReconciler{ReconcileFunc: func(ctx context.Context, req ReconcileRequest) (ReconcileResult, error) {
					if req.Action == ReconcileActionDeleted {
						checkCleanupContext(ctx)
						assert.EqualValues(t, 1, secondDeletes.Load(), "both watchers must dispatch before the reconciler")
						reconcileDeletes.Add(1)
					}
					return ReconcileResult{}, nil
				}})
				require.NoError(t, c.AddReconciler(reconciler, kind))
				inf := &testInformer{}
				require.NoError(t, c.AddInformer(inf, kind))
				go c.Run(runCtx)
				synctest.Wait()
				source, ok := c.informers.ItemAt(kind, 0)
				require.True(t, ok)
				run := source.run.Load()
				update := inf.handlers[0].Update
				add := inf.handlers[0].Add
				require.NoError(t, update(runCtx, obj, obj))

				waitForSignal(t, started, "the resync retry did not start")

				deletion := server.markDeleting()
				deliver := func(ctx context.Context) error {
					if test.action == ResourceActionCreate {
						return add(ctx, deletion[1])
					}
					return update(ctx, deletion[0], deletion[1])
				}
				eventCtx, cancel := context.WithCancel(runCtx)
				defer cancel()
				canceledDelivery := make(chan error, 1)
				go func() { canceledDelivery <- withInformerEventTimeout(eventCtx, eventTimeout, deliver) }()
				synctest.Wait()
				select {
				case err := <-canceledDelivery:
					t.Fatalf("deletion returned before cancellation or retry completion: %v", err)
				default:
				}
				wantErr := context.Canceled
				if eventTimeout > 0 {
					time.Sleep(eventTimeout)
					wantErr = context.DeadlineExceeded
				} else {
					cancel()
				}
				synctest.Wait()
				select {
				case err := <-canceledDelivery:
					require.ErrorIs(t, err, wantErr)
				default:
					t.Fatal("deletion still waiting after cancellation")
				}
				assert.Zero(t, deleteCalls.Load())
				assert.Zero(t, laterWatcherCalls.Load())
				require.Equal(t, obj.GetFinalizers(), server.state().GetFinalizers())

				// No new event is needed to finish the canceled delivery. A redelivery
				// can race that work, and removal must cancel just the affected owner.
				var redelivery chan error
				switch test.name {
				case "redelivery":
					redelivery = make(chan error, 1)
					go func() { redelivery <- deliver(runCtx) }()
				case "repeated canceled deliveries":
					// Startup Adds and live Updates retain the same cleanup while
					// the retry is blocked, even when every delivery expires.
					for range redeliveries {
						require.ErrorIs(t, add(eventCtx, deletion[1]), context.Canceled)
						require.ErrorIs(t, update(eventCtx, deletion[0], deletion[1]), context.Canceled)
					}
					synctest.Wait()
					require.Len(t, run.cleanups, 1)
					key := c.deliveries[deliveryKey{kind, obj.GetStaticMetadata().Identifier()}]
					pending := run.cleanups[key].pending
					pending = slices.DeleteFunc(slices.Clone(pending), func(d deferredDelivery) bool { return !d.cleanup })
					require.Len(t, pending, 3, "the two watchers and one reconciler coalesce finalizer cleanup")
					require.Len(t, pending[0].running, 1, "each notification saw the same blocked retry")
					require.Empty(t, pending[1].running)
					require.Empty(t, pending[2].running)
				case "same name recreated":
					// The informer missed the old object's removal. A startup Add
					// for its terminating replacement must retain separate work.
					server.change(func(target resource.Object) {
						target.SetUID("replacement")
						target.SetDeletionTimestamp(nil)
					})
					replacement := server.markDeleting()
					require.ErrorIs(t, add(eventCtx, replacement[1]), context.Canceled)
					synctest.Wait()
					require.Len(t, run.cleanups, 2)
				case "controller stops":
					stop()
				case "informer removed":
					c.RemoveInformer(inf, kind)
				case "watcher removed":
					c.RemoveWatcher(watcher, kind)
				}
				synctest.Wait()
				require.Zero(t, deleteCalls.Load())
				release()
				if test.name == "informer removed during cleanup" {
					synctest.Wait()
					require.EqualValues(t, 1, deleteCalls.Load(), "the callback must be in flight at removal")
					c.RemoveInformer(inf, kind)
				}
				if redelivery != nil {
					require.NoError(t, <-redelivery)
				}
				if test.name != "controller stops" && test.name != "informer removed" && test.name != "informer removed during cleanup" {
					remaining := 0
					if test.name == "watcher removed" {
						remaining = 1
					}
					// Concurrent owners can hit an RV conflict. Wait for its
					// backoff and the successful patch, beyond callback return.
					require.Eventually(t, func() bool { return len(server.state().GetFinalizers()) == remaining }, 10*time.Second, time.Millisecond)
				}
				synctest.Wait()
				assert.Empty(t, run.cleanups)
				if test.name == "repeated canceled deliveries" {
					assert.EqualValues(t, 1+2*redeliveries, laterWatcherCalls.Load(), "plain watchers receive each Add and Update")
				}
				switch test.name {
				case "controller stops", "informer removed", "informer removed during cleanup":
					if test.name != "informer removed during cleanup" {
						assert.Zero(t, deleteCalls.Load())
					}
					assert.Zero(t, secondDeletes.Load())
					assert.Zero(t, reconcileDeletes.Load())
					assert.Equal(t, obj.GetFinalizers(), server.state().GetFinalizers())
				case "watcher removed":
					assert.Zero(t, deleteCalls.Load())
					assert.EqualValues(t, 1, reconcileDeletes.Load())
					assert.Equal(t, []string{watcher.finalizer}, server.state().GetFinalizers())
				default:
					expectedCalls := 1
					if test.name == "deferred cleanup retries" {
						expectedCalls = 2
					}
					assert.EqualValues(t, expectedCalls, deleteCalls.Load())
					assert.EqualValues(t, 1, secondDeletes.Load())
					assert.EqualValues(t, 1, reconcileDeletes.Load())
					assert.Empty(t, server.state().GetFinalizers())
				}
			})
		})
	}
}

func TestInformerController_PrometheusCollectors(t *testing.T) {
	c := NewInformerController(InformerControllerConfig{
		MetricsConfig: metrics.DefaultConfig("test"),
	})
	collectors := c.PrometheusCollectors()
	assert.ElementsMatch(t, []prometheus.Collector{
		c.totalEvents,
		c.reconcileLatency,
		c.inflightEvents,
		c.inflightActions,
		c.reconcilerLatency,
		c.watcherLatency,
		c.reconcilerErrors,
		c.watcherErrors,
	}, collectors)
}

func TestInformerController_Metrics_ReconcilerErrors(t *testing.T) {
	kind := "foo"
	inf := &testInformer{}
	c := NewInformerController(InformerControllerConfig{
		MetricsConfig: metrics.DefaultConfig("test"),
		// No RetryPolicy so errors are dropped after first attempt — keeps the test simple.
		RetryPolicy: func(_ error, _ int) (bool, time.Duration) { return false, 0 },
	})
	c.AddReconciler(&SimpleReconciler{
		ReconcileFunc: func(_ context.Context, _ ReconcileRequest) (ReconcileResult, error) {
			return ReconcileResult{}, errors.New("reconcile error")
		},
	}, kind)
	c.AddInformer(inf, kind)

	ctx := t.Context()
	go c.Run(ctx)

	inf.FireAdd(context.Background(), emptyObject)

	assert.Equal(t, float64(1), counterValueFromVec(t, c.reconcilerErrors, string(ResourceActionCreate), ""))
}

func TestInformerController_Metrics_WatcherErrors(t *testing.T) {
	kind := "foo"
	inf := &testInformer{}
	c := NewInformerController(InformerControllerConfig{
		MetricsConfig: metrics.DefaultConfig("test"),
		RetryPolicy:   func(_ error, _ int) (bool, time.Duration) { return false, 0 },
	})
	c.AddWatcher(&SimpleWatcher{
		AddFunc: func(_ context.Context, _ resource.Object) error {
			return errors.New("watcher add error")
		},
		UpdateFunc: func(_ context.Context, _, _ resource.Object) error {
			return errors.New("watcher update error")
		},
		DeleteFunc: func(_ context.Context, _ resource.Object) error {
			return errors.New("watcher delete error")
		},
	}, kind)
	c.AddInformer(inf, kind)

	ctx := t.Context()
	go c.Run(ctx)

	inf.FireAdd(context.Background(), emptyObject)
	inf.FireUpdate(context.Background(), emptyObject, emptyObject)
	inf.FireDelete(context.Background(), emptyObject)

	assert.Equal(t, float64(1), counterValueFromVec(t, c.watcherErrors, string(ResourceActionCreate), ""))
	assert.Equal(t, float64(1), counterValueFromVec(t, c.watcherErrors, string(ResourceActionUpdate), ""))
	assert.Equal(t, float64(1), counterValueFromVec(t, c.watcherErrors, string(ResourceActionDelete), ""))
}

func counterValueFromVec(t *testing.T, cv *prometheus.CounterVec, labelValues ...string) float64 {
	t.Helper()
	m, err := cv.GetMetricWithLabelValues(labelValues...)
	require.NoError(t, err)
	var pb dto.Metric
	require.NoError(t, m.Write(&pb))
	return pb.GetCounter().GetValue()
}
