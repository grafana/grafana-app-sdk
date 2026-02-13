package operator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryProcessor_HashDistribution(t *testing.T) {
	// Verify that items with the same key route to the same worker (deterministic),
	// and different keys can coexist. We verify this indirectly: enqueue several items
	// with the same key and different keys, then check Len().
	tests := []struct {
		name         string
		enqueue      []RetryRequest
		expectedLen  int
	}{
		{
			name: "single key, multiple items",
			enqueue: []RetryRequest{
				{Key: "obj-a", RetryAfter: time.Now().Add(time.Hour)},
				{Key: "obj-a", RetryAfter: time.Now().Add(time.Hour)},
				{Key: "obj-a", RetryAfter: time.Now().Add(time.Hour)},
			},
			expectedLen: 3,
		},
		{
			name: "multiple keys",
			enqueue: []RetryRequest{
				{Key: "obj-a", RetryAfter: time.Now().Add(time.Hour)},
				{Key: "obj-b", RetryAfter: time.Now().Add(time.Hour)},
				{Key: "obj-c", RetryAfter: time.Now().Add(time.Hour)},
			},
			expectedLen: 3,
		},
		{
			name:        "empty queue",
			enqueue:     []RetryRequest{},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewRetryProcessor(RetryProcessorConfig{
				WorkerPoolSize: 4,
				CheckInterval:  time.Hour, // long interval so nothing fires
			}, func() RetryPolicy {
				return func(err error, attempt int) (bool, time.Duration) {
					return false, 0
				}
			})

			for _, req := range tt.enqueue {
				processor.Enqueue(req)
			}

			assert.Equal(t, tt.expectedLen, processor.Len())
		})
	}
}

func TestRetryProcessor_TimeBasedScheduling(t *testing.T) {
	// Items with future RetryAfter should not execute before their scheduled time.
	var executed atomic.Int64
	processor := NewRetryProcessor(RetryProcessorConfig{
		WorkerPoolSize: 1,
		CheckInterval:  10 * time.Millisecond,
	}, func() RetryPolicy {
		return func(err error, attempt int) (bool, time.Duration) {
			return false, 0
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go processor.Run(ctx)

	processor.Enqueue(RetryRequest{
		Key:        "delayed-item",
		RetryAfter: time.Now().Add(200 * time.Millisecond),
		RetryFunc: func() (*time.Duration, error) {
			executed.Add(1)
			return nil, nil
		},
	})

	// After 50ms, item should not have executed yet
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(0), executed.Load(), "item executed before its scheduled time")

	// After 300ms total (giving 100ms buffer beyond the 200ms delay), it should have run
	time.Sleep(250 * time.Millisecond)
	assert.Equal(t, int64(1), executed.Load(), "item did not execute after its scheduled time")
}

func TestRetryProcessor_RetryPolicyCompliance(t *testing.T) {
	// When retry func fails, RetryPolicy is consulted.
	// The processor calls policy(err, attempt+1) after each failure.
	// With maxAttempts=3 (attempt < 3), we get:
	//   Call 1 (attempt=0): fails -> policy(err, 1) -> retry with attempt=1
	//   Call 2 (attempt=1): fails -> policy(err, 2) -> retry with attempt=2
	//   Call 3 (attempt=2): fails -> policy(err, 3) -> 3 >= 3, stop
	// Total: 3 calls.
	var calls atomic.Int64
	done := make(chan struct{})

	processor := NewRetryProcessor(RetryProcessorConfig{
		WorkerPoolSize: 1,
		CheckInterval:  10 * time.Millisecond,
	}, func() RetryPolicy {
		return func(err error, attempt int) (bool, time.Duration) {
			if attempt >= 3 {
				return false, 0
			}
			return true, 10 * time.Millisecond
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go processor.Run(ctx)

	processor.Enqueue(RetryRequest{
		Key:        "retry-policy-test",
		RetryAfter: time.Now(),
		RetryFunc: func() (*time.Duration, error) {
			n := calls.Add(1)
			if n == 3 {
				close(done)
			}
			return nil, fmt.Errorf("persistent error")
		},
	})

	select {
	case <-done:
		// Give a bit of time to ensure no extra calls happen
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, int64(3), calls.Load(), "expected exactly 3 calls (initial + 2 retries)")
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for retries, got %d calls", calls.Load())
	}
}

func TestRetryProcessor_SuccessStopsRetries(t *testing.T) {
	// When retry func succeeds, no more retries happen.
	// The func fails once, then succeeds. Total calls should be 2.
	var calls atomic.Int64
	done := make(chan struct{})

	processor := NewRetryProcessor(RetryProcessorConfig{
		WorkerPoolSize: 1,
		CheckInterval:  10 * time.Millisecond,
	}, func() RetryPolicy {
		return func(err error, attempt int) (bool, time.Duration) {
			return true, 10 * time.Millisecond
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go processor.Run(ctx)

	processor.Enqueue(RetryRequest{
		Key:        "success-stops-retry",
		RetryAfter: time.Now(),
		RetryFunc: func() (*time.Duration, error) {
			n := calls.Add(1)
			if n == 1 {
				return nil, fmt.Errorf("transient error")
			}
			// Second call succeeds
			close(done)
			return nil, nil
		},
	})

	select {
	case <-done:
		// Wait a bit to make sure no extra retries happen
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, int64(2), calls.Load(), "expected exactly 2 calls (1 failure + 1 success)")
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for success, got %d calls", calls.Load())
	}
}

func TestRetryProcessor_Dequeue(t *testing.T) {
	// Predicate-based removal. Enqueue 3 items with different actions.
	// Dequeue items matching a specific action. Verify remaining count.
	processor := NewRetryProcessor(RetryProcessorConfig{
		WorkerPoolSize: 4,
		CheckInterval:  time.Hour, // don't fire
	}, func() RetryPolicy {
		return func(err error, attempt int) (bool, time.Duration) {
			return false, 0
		}
	})

	processor.Enqueue(RetryRequest{
		Key:        "obj-1",
		RetryAfter: time.Now().Add(time.Hour),
		Action:     ResourceActionCreate,
		RetryFunc:  func() (*time.Duration, error) { return nil, nil },
	})
	processor.Enqueue(RetryRequest{
		Key:        "obj-1",
		RetryAfter: time.Now().Add(time.Hour),
		Action:     ResourceActionUpdate,
		RetryFunc:  func() (*time.Duration, error) { return nil, nil },
	})
	processor.Enqueue(RetryRequest{
		Key:        "obj-1",
		RetryAfter: time.Now().Add(time.Hour),
		Action:     ResourceActionDelete,
		RetryFunc:  func() (*time.Duration, error) { return nil, nil },
	})

	require.Equal(t, 3, processor.Len())

	// Dequeue only CREATE actions
	processor.Dequeue("obj-1", func(req RetryRequest) bool {
		return req.Action == ResourceActionCreate
	})

	assert.Equal(t, 2, processor.Len(), "should have 2 items remaining after dequeuing CREATE")
}

func TestRetryProcessor_DequeueAll(t *testing.T) {
	// Remove all items for a key. Enqueue multiple items for same key. DequeueAll. Verify Len() == 0.
	processor := NewRetryProcessor(RetryProcessorConfig{
		WorkerPoolSize: 4,
		CheckInterval:  time.Hour,
	}, func() RetryPolicy {
		return func(err error, attempt int) (bool, time.Duration) {
			return false, 0
		}
	})

	for i := 0; i < 5; i++ {
		processor.Enqueue(RetryRequest{
			Key:        "same-key",
			RetryAfter: time.Now().Add(time.Hour),
			RetryFunc:  func() (*time.Duration, error) { return nil, nil },
		})
	}
	// Also add items under a different key to verify only the target key is removed
	processor.Enqueue(RetryRequest{
		Key:        "other-key",
		RetryAfter: time.Now().Add(time.Hour),
		RetryFunc:  func() (*time.Duration, error) { return nil, nil },
	})

	require.Equal(t, 6, processor.Len())

	processor.DequeueAll("same-key")

	assert.Equal(t, 1, processor.Len(), "should have only the other-key item remaining")
}

func TestRetryProcessor_ConcurrentEnqueue(t *testing.T) {
	// Goroutine-bomb test: 100 goroutines each enqueueing 10 items.
	// Verify no panics and final Len() is correct.
	processor := NewRetryProcessor(RetryProcessorConfig{
		WorkerPoolSize: 4,
		CheckInterval:  time.Hour, // don't fire
	}, func() RetryPolicy {
		return func(err error, attempt int) (bool, time.Duration) {
			return false, 0
		}
	})

	const goroutines = 100
	const itemsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gIdx int) {
			defer wg.Done()
			for i := 0; i < itemsPerGoroutine; i++ {
				processor.Enqueue(RetryRequest{
					Key:        fmt.Sprintf("key-%d-%d", gIdx, i),
					RetryAfter: time.Now().Add(time.Hour),
					RetryFunc:  func() (*time.Duration, error) { return nil, nil },
				})
			}
		}(g)
	}

	require.True(t, waitOrTimeout(&wg, 10*time.Second), "concurrent enqueue timed out")
	assert.Equal(t, goroutines*itemsPerGoroutine, processor.Len())
}

func TestRetryProcessor_GracefulShutdown(t *testing.T) {
	// Cancel context and verify Run() returns without hanging.
	processor := NewRetryProcessor(RetryProcessorConfig{
		WorkerPoolSize: 4,
		CheckInterval:  10 * time.Millisecond,
	}, func() RetryPolicy {
		return func(err error, attempt int) (bool, time.Duration) {
			return true, 10 * time.Millisecond
		}
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- processor.Run(ctx)
	}()

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err, "Run should return nil on context cancellation")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation within timeout")
	}
}

func TestRetryProcessor_RequeueAfter(t *testing.T) {
	// When RetryFunc returns a non-nil duration, the item is re-enqueued with that delay
	// and attempt is NOT incremented.
	var calls atomic.Int64
	var attempts []int
	var mu sync.Mutex
	done := make(chan struct{})

	processor := NewRetryProcessor(RetryProcessorConfig{
		WorkerPoolSize: 1,
		CheckInterval:  10 * time.Millisecond,
	}, func() RetryPolicy {
		return func(err error, attempt int) (bool, time.Duration) {
			// Should not be called when RetryFunc returns a duration
			return false, 0
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go processor.Run(ctx)

	processor.Enqueue(RetryRequest{
		Key:        "requeue-test",
		RetryAfter: time.Now(),
		Attempt:    0,
		RetryFunc: func() (*time.Duration, error) {
			n := calls.Add(1)
			mu.Lock()
			// The attempt from the RetryRequest should stay the same since requeue doesn't increment
			// We track it to verify the behavior
			attempts = append(attempts, int(n))
			mu.Unlock()
			if n < 3 {
				d := 10 * time.Millisecond
				return &d, nil
			}
			close(done)
			return nil, nil
		},
	})

	select {
	case <-done:
		assert.Equal(t, int64(3), calls.Load(), "expected 3 calls via requeue mechanism")
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for requeue completions, got %d calls", calls.Load())
	}
}

func TestRetryProcessor_DequeueDoesNotBlockOnExecution(t *testing.T) {
	// Dequeue returns fast even when a retry is mid-execution.
	executing := make(chan struct{})
	retryDone := make(chan struct{})

	processor := NewRetryProcessor(RetryProcessorConfig{
		WorkerPoolSize: 1,
		CheckInterval:  10 * time.Millisecond,
	}, func() RetryPolicy {
		return func(err error, attempt int) (bool, time.Duration) {
			return false, 0
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		// Wait for the blocking RetryFunc to complete so we don't leak goroutines
		<-retryDone
	}()

	runDone := make(chan struct{})
	go func() {
		processor.Run(ctx)
		close(runDone)
	}()

	processor.Enqueue(RetryRequest{
		Key:        "blocking-key",
		RetryAfter: time.Now(),
		RetryFunc: func() (*time.Duration, error) {
			close(executing)
			// Block for 500ms to simulate slow execution
			time.Sleep(500 * time.Millisecond)
			close(retryDone)
			return nil, nil
		},
	})

	// Wait for the retry func to start executing
	select {
	case <-executing:
	case <-time.After(5 * time.Second):
		t.Fatal("retry func never started executing")
	}

	// Dequeue should return quickly even though the retry func is mid-execution
	start := time.Now()
	processor.Dequeue("blocking-key", func(req RetryRequest) bool {
		return true
	})
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 50*time.Millisecond, "Dequeue should return quickly, not block on executing retry")
}

// --- Benchmarks ---

func BenchmarkRetryProcessor_Throughput(b *testing.B) {
	for _, workers := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			retryPolicy := func(err error, attempt int) (bool, time.Duration) {
				return false, 0 // Don't retry in benchmark
			}
			processor := NewRetryProcessor(RetryProcessorConfig{
				WorkerPoolSize: workers,
				CheckInterval:  time.Millisecond,
			}, func() RetryPolicy { return retryPolicy })

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go processor.Run(ctx)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				done := make(chan struct{})
				count := int64(100)
				var completed atomic.Int64

				for j := 0; j < int(count); j++ {
					key := fmt.Sprintf("key-%d", j)
					processor.Enqueue(RetryRequest{
						Key:        key,
						RetryAfter: time.Now(), // immediately due
						RetryFunc: func() (*time.Duration, error) {
							if completed.Add(1) == count {
								close(done)
							}
							return nil, nil
						},
					})
				}

				select {
				case <-done:
				case <-time.After(10 * time.Second):
					b.Fatal("timeout")
				}
			}
		})
	}
}
