package operator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
)

func TestInformerController_AddWatcher(t *testing.T) {
	t.Run("nil watcher", func(t *testing.T) {
		c := NewInformerController()
		err := c.AddWatcher(nil, "")
		assert.Equal(t, errors.New("watcher cannot be nil"), err)
	})

	t.Run("empty resourceKind", func(t *testing.T) {
		c := NewInformerController()
		err := c.AddWatcher(&SimpleWatcher{}, "")
		assert.Equal(t, errors.New("resourceKind cannot be empty"), err)
	})

	t.Run("first watcher", func(t *testing.T) {
		c := NewInformerController()
		w := &SimpleWatcher{}
		k := "foo"
		err := c.AddWatcher(w, k)
		assert.Nil(t, err)
		assert.Equal(t, 1, c.watchers.KeySize(k))
		iw, _ := c.watchers.ItemAt(k, 0)
		assert.Equal(t, w, iw)
	})

	t.Run("existing watchers", func(t *testing.T) {
		c := NewInformerController()
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
		c := NewInformerController()
		// Ensure no panics
		c.RemoveWatcher(nil, "")
	})

	t.Run("empty resourceKind", func(t *testing.T) {
		c := NewInformerController()
		// Ensure no panics
		c.RemoveWatcher(&SimpleWatcher{}, "")
	})

	t.Run("not in list", func(t *testing.T) {
		c := NewInformerController()
		w1 := &SimpleWatcher{}
		w2 := &SimpleWatcher{}
		k := "foo"
		c.AddWatcher(w1, k)
		c.RemoveWatcher(w2, k)
		assert.Equal(t, 1, c.watchers.KeySize(k))
	})

	t.Run("only watcher in list", func(t *testing.T) {
		c := NewInformerController()
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
		c := NewInformerController()
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
		c := NewInformerController()
		// Ensure no panics
		c.RemoveAllWatchersForResource("")
	})

	t.Run("no watchers", func(t *testing.T) {
		c := NewInformerController()
		// Ensure no panics
		c.RemoveAllWatchersForResource("foo")
	})

	t.Run("watchers", func(t *testing.T) {
		w1 := &SimpleWatcher{}
		w2 := &SimpleWatcher{}
		w3 := &SimpleWatcher{}
		k1 := "foo"
		k2 := "bar"
		c := NewInformerController()
		c.AddWatcher(w1, k1)
		c.AddWatcher(w2, k1)
		c.AddWatcher(w3, k2)
		assert.Equal(t, 2, c.watchers.Size()) // 2 keys
		c.RemoveAllWatchersForResource(k1)
		assert.Equal(t, 1, c.watchers.Size()) // 1 key
		assert.Equal(t, 1, c.watchers.KeySize(k2))
	})
}

func TestInformerController_AddInformer(t *testing.T) {
	t.Run("nil informer", func(t *testing.T) {
		c := NewInformerController()
		err := c.AddInformer(nil, "")
		assert.Equal(t, errors.New("informer cannot be nil"), err)
	})

	t.Run("empty resourceKind", func(t *testing.T) {
		c := NewInformerController()
		err := c.AddInformer(&mockInformer{}, "")
		assert.Equal(t, errors.New("resourceKind cannot be empty"), err)
	})

	t.Run("first informer", func(t *testing.T) {
		c := NewInformerController()
		i := &mockInformer{}
		k := "foo"
		err := c.AddInformer(i, k)
		assert.Nil(t, err)
		assert.Equal(t, 1, c.informers.KeySize(k))
		ii1, _ := c.informers.ItemAt(k, 0)
		assert.Equal(t, i, ii1)
	})

	t.Run("existing informers", func(t *testing.T) {
		c := NewInformerController()
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
		wg := sync.WaitGroup{}
		c := NewInformerController()
		inf1 := &mockInformer{
			RunFunc: func(stopCh <-chan struct{}) error {
				<-stopCh
				wg.Done()
				return nil
			},
		}
		c.AddInformer(inf1, "foo")
		inf2 := &mockInformer{
			RunFunc: func(stopCh <-chan struct{}) error {
				<-stopCh
				wg.Done()
				return nil
			},
		}
		c.AddInformer(inf2, "bar")
		wg.Add(3)

		stopCh := make(chan struct{})
		go func() {
			err := c.Run(stopCh)
			assert.Nil(t, err)
			wg.Done()
		}()
		go func() {
			time.Sleep(time.Second * 3)
			close(stopCh)
		}()
		wg.Wait()
	})

	t.Run("normal operation", func(t *testing.T) {
		wg := sync.WaitGroup{}
		c := NewInformerController()
		inf1 := &mockInformer{
			RunFunc: func(stopCh <-chan struct{}) error {
				<-stopCh
				wg.Done()
				return nil
			},
		}
		c.AddInformer(inf1, "foo")
		inf2 := &mockInformer{
			RunFunc: func(stopCh <-chan struct{}) error {
				<-stopCh
				wg.Done()
				return nil
			},
		}
		c.AddInformer(inf2, "bar")
		wg.Add(3)

		stopCh := make(chan struct{})
		go func() {
			err := c.Run(stopCh)
			assert.Nil(t, err)
			wg.Done()
		}()
		go func() {
			time.Sleep(time.Second * 3)
			close(stopCh)
		}()
		wg.Wait()
	})
}

func TestInformerController_Run_BackoffRetry(t *testing.T) {
	// The backoff retry test needs to take twenty seconds to run properly, so it's isolated to its own function
	// to avoid the often-used default of a 30-second-timeout on tests affecting other retry tests which take a few seconds each to run
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
	c := NewInformerController()
	// One-second multiplier on exponential backoff.
	// Backoff will be 1s, 2s, 4s, 8s, 16s
	c.RetryPolicy = ExponentialBackoffRetryPolicy(time.Second, 5)
	c.AddInformer(inf, "foo")
	c.AddWatcher(&SimpleWatcher{
		AddFunc: func(ctx context.Context, object resource.Object) error {
			addAttempt++
			return addError
		},
		UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
			updateAttempt++
			return updateError
		},
	}, "foo")
	wg.Add(2)

	stopCh := make(chan struct{})
	go func() {
		err := c.Run(stopCh)
		assert.Nil(t, err)
		wg.Done()
	}()
	inf.FireAdd(context.Background(), nil)
	// 3 retries takes 7 seconds, 4 takes 15. Use 10 for some leeway
	time.Sleep(time.Second * 10)
	// Fire an update, which should halt the add retries
	inf.FireUpdate(context.Background(), nil, nil)
	go func() {
		// 3 retries takes 7 seconds, 4 takes 15. Use 10 for some leeway
		time.Sleep(time.Second * 10)
		close(stopCh)
	}()
	wg.Wait()
	// We should have four total attempts for each call, initial + three retries
	assert.Equal(t, 4, addAttempt)
	assert.Equal(t, 4, updateAttempt)
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
		c := NewInformerController()
		// Make the retry ticker interval a half-second so we can run this test faster
		c.retryTickerInterval = time.Millisecond * 500
		// 500-ms linear retry policy
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			return true, time.Millisecond * 500
		}
		c.AddInformer(inf, "foo")
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addAttempt++
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateAttempt++
				return nil
			},
		}, "foo")
		wg.Add(2)

		stopCh := make(chan struct{})
		go func() {
			err := c.Run(stopCh)
			assert.Nil(t, err)
			wg.Done()
		}()
		inf.FireAdd(context.Background(), nil)
		// Wait for what should be four retries
		time.Sleep(time.Millisecond * 2300)
		// Fire an update, which should halt the add retries
		inf.FireUpdate(context.Background(), nil, nil)
		go func() {
			// 3 retries takes 7 seconds, 4 takes 15. Use 10 for some leeway
			time.Sleep(time.Second)
			close(stopCh)
		}()
		wg.Wait()
		// We should have four total attempts, though we may be off-by-one because timing is hard,
		// so we actually check that 3 <= attempts <= 5
		assert.LessOrEqual(t, 3, addAttempt)
		assert.GreaterOrEqual(t, 5, addAttempt)
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
		c := NewInformerController()
		// Make the retry ticker interval a 50 ms so we can run this test faster
		c.retryTickerInterval = time.Millisecond * 50
		// 500-ms linear retry policy
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			return true, time.Millisecond * 50
		}
		c.AddInformer(inf, "foo")
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addAttempt++
				if addAttempt >= 2 {
					return nil
				}
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateAttempt++
				if updateAttempt >= 2 {
					return nil
				}
				return updateError
			},
		}, "foo")
		wg.Add(2)

		stopCh := make(chan struct{})
		go func() {
			err := c.Run(stopCh)
			assert.Nil(t, err)
			wg.Done()
		}()
		inf.FireAdd(context.Background(), nil)
		// Wait for half a second, this should be enough time for many retries if the halt doesn't work
		time.Sleep(time.Millisecond * 500)
		inf.FireUpdate(context.Background(), nil, nil)
		go func() {
			// Wait for half a second, this should be enough time for many retries if the halt doesn't work
			time.Sleep(time.Millisecond * 500)
			close(stopCh)
		}()
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
		c := NewInformerController()
		// Make the retry ticker interval a 50 ms so we can run this test faster
		c.retryTickerInterval = time.Millisecond * 50
		// 500-ms linear retry policy
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			return attempt < 3, time.Millisecond * 50
		}
		c.AddInformer(inf, "foo")
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addAttempt++
				if addAttempt >= 2 {
					return nil
				}
				return addError
			},
		}, "foo")
		wg.Add(2)

		stopCh := make(chan struct{})
		go func() {
			err := c.Run(stopCh)
			assert.Nil(t, err)
			wg.Done()
		}()
		inf.FireAdd(context.Background(), nil)
		go func() {
			// Wait for half a second, this should be enough time for many retries if the halt doesn't work
			time.Sleep(time.Millisecond * 500)
			close(stopCh)
		}()
		wg.Wait()
		// We should have only two attempts for each
		assert.Equal(t, 2, addAttempt)
	})
}

func TestInformerController_Run_WithRetriesAndDequeuePolicy(t *testing.T) {
	// Because these tests take more time, isolate them to their own test function
	t.Run("linear retry, don't dequeue", func(t *testing.T) {
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
		c := NewInformerController()
		// Make the retry ticker interval a half-second so we can run this test faster
		c.retryTickerInterval = time.Millisecond * 500
		// 500-ms linear retry policy
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			return true, time.Millisecond * 500
		}
		c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy
		c.AddInformer(inf, "foo")
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addAttempt++
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateAttempt++
				return nil
			},
		}, "foo")
		wg.Add(2)

		stopCh := make(chan struct{})
		go func() {
			err := c.Run(stopCh)
			assert.Nil(t, err)
			wg.Done()
		}()
		inf.FireAdd(context.Background(), nil)
		// Wait for what should be four retries
		time.Sleep(time.Millisecond * 2300)
		// Fire an update, which in this case should not interrupt the add retries
		inf.FireUpdate(context.Background(), nil, nil)
		go func() {
			time.Sleep(time.Second)
			close(stopCh)
		}()
		wg.Wait()
		// We should have six total attempts, though we may be off-by-one because timing is hard,
		// so we actually check that 5 <= attempts <= 7
		assert.LessOrEqual(t, 5, addAttempt)
		assert.GreaterOrEqual(t, 7, addAttempt)
		assert.Equal(t, 1, updateAttempt)
	})
	// Because these tests take more time, isolate them to their own test function
	t.Run("linear retry, don't dequeue, overlapping retries", func(t *testing.T) {
		addError := errors.New("I AM ERROR")
		updateError := errors.New("JE SUIS ERROR")
		addAttempt := 0
		updateAttempt := 0
		wg := sync.WaitGroup{}
		inf := &testInformer{
			handlers: make([]ResourceWatcher, 0),
			onStop: func() {
				wg.Done()
			},
		}
		c := NewInformerController()
		// Make the retry ticker interval a half-second so we can run this test faster
		c.retryTickerInterval = time.Millisecond * 500
		// 500-ms linear retry policy
		c.RetryPolicy = func(err error, attempt int) (bool, time.Duration) {
			if attempt >= 6 {
				return false, 0
			}
			return true, time.Millisecond * 450
		}
		c.RetryDequeuePolicy = OpinionatedRetryDequeuePolicy
		c.AddInformer(inf, "foo")
		c.AddWatcher(&SimpleWatcher{
			AddFunc: func(ctx context.Context, object resource.Object) error {
				addAttempt++
				return addError
			},
			UpdateFunc: func(ctx context.Context, object resource.Object, object2 resource.Object) error {
				updateAttempt++
				return updateError
			},
		}, "foo")
		wg.Add(2)

		stopCh := make(chan struct{})
		go func() {
			err := c.Run(stopCh)
			assert.Nil(t, err)
			wg.Done()
		}()
		inf.FireAdd(context.Background(), nil)
		// Wait for what should be four retries
		time.Sleep(time.Millisecond * 2300)
		// Fire an update, which in this case should not interrupt the add retries
		inf.FireUpdate(context.Background(), nil, nil)
		go func() {
			time.Sleep(time.Millisecond * 4000)
			close(stopCh)
		}()
		wg.Wait()
		// We should have seven total attempts (six retries + initial)
		assert.Equal(t, 7, addAttempt)
		assert.Equal(t, 7, updateAttempt)
	})
}

type mockInformer struct {
	AddEventHandlerFunc func(handler ResourceWatcher)
	RunFunc             func(stopCh <-chan struct{}) error
}

func (i *mockInformer) AddEventHandler(handler ResourceWatcher) error {
	if i.AddEventHandlerFunc != nil {
		i.AddEventHandlerFunc(handler)
	}
	return nil
}
func (i *mockInformer) Run(stopCh <-chan struct{}) error {
	if i.RunFunc != nil {
		return i.RunFunc(stopCh)
	}
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

func (ti *testInformer) Run(stopCh <-chan struct{}) error {
	<-stopCh
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
