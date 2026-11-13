package benchmark_test

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	clientfeatures "k8s.io/client-go/features"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
	"github.com/grafana/grafana-app-sdk/simple"
)

// runInformerWithValidation runs an informer and validates that all expected events are received.
// It sets up event handlers, runs the informer, waits for sync, validates event delivery,
// and handles shutdown with the specified timeout.
func runInformerWithValidation(b *testing.B, inf operator.Informer, expectedCount int) {
	// Track received events to verify event delivery
	var (
		receivedCount atomic.Int32
		doneCh        = make(chan struct{})
	)

	if err := inf.AddEventHandler(&operator.SimpleWatcher{
		AddFunc: func(_ context.Context, obj resource.Object) error {
			if int(receivedCount.Add(1)) == expectedCount {
				close(doneCh)
			}

			return nil
		},
	}); err != nil {
		b.Fatalf("failed to add event handler: %v", err)
	}

	// Create context with cancellation to be able to stop the informer
	ctx, cancel := context.WithCancel(b.Context())

	// Run informer in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- inf.Run(ctx)
	}()

	// Wait for the informer to sync fully.
	if err := inf.WaitForSync(ctx); err != nil {
		cancel()
		b.Fatalf("failed to wait for informer to sync: %v", err)
	}

	// Wait for all events to be processed
	select {
	case <-doneCh:
		// All events received
	case <-b.Context().Done():
		cancel()
		b.Fatalf("timeout waiting for events, got %d of %d", receivedCount.Load(), expectedCount)
	}

	// Verify all objects were received via event handler
	actualCount := int(receivedCount.Load())
	if actualCount != expectedCount {
		cancel()
		b.Fatalf("expected %d objects via event handler, got %d", expectedCount, actualCount)
	}

	// Stop informer
	cancel()

	// Wait for shutdown
	select {
	case <-errCh:
	// Clean shutdown
	case <-b.Context().Done():
		// Shutdown timeout
		b.Fatalf("timeout waiting for informer to stop")
	}
}

// BenchmarkDefaultInformerSupplier benchmarks the standard K8s informer supplier.
func BenchmarkDefaultInformerSupplier(b *testing.B) {
	if err := suppressLogger(); err != nil {
		b.Fatalf("failed to suppress logger: %v", err)
	}

	scenarios := []struct {
		name         string
		useWatchList bool
		objectCount  int
	}{
		{"10_objects", false, 10},
		{"100_objects", false, 100},
		{"1000_objects", false, 1000},
		{"10000_objects", false, 10000},
		{"10000_objects_watchlist", true, 10000},
	}

	for _, scenario := range scenarios {
		kind := benchmarkKind()
		objects := generateTestObjects(scenario.objectCount)

		b.ReportAllocs()

		b.Run(scenario.name, func(b *testing.B) {
			if scenario.useWatchList {
				if err := setClientFeature(clientfeatures.WatchListClient, true); err != nil {
					b.Fatalf("failed to set client feature: %v", err)
				}

				defer func() {
					if err := setClientFeature(clientfeatures.WatchListClient, false); err != nil {
						b.Fatalf("failed to unset client feature: %v", err)
					}
				}()
			}

			clientGen := &mockClientGeneratorWithK8sClient{
				kind:    kind,
				objects: objects,
			}

			opts := operator.InformerOptions{
				ListWatchOptions: operator.ListWatchOptions{},
			}

			for b.Loop() {
				// Create informer via DefaultInformerSupplier
				inf, err := simple.DefaultInformerSupplier(kind, clientGen, opts)
				if err != nil {
					b.Fatalf("failed to create informer: %v", err)
				}

				// Run informer and validate event delivery
				runInformerWithValidation(b, inf, scenario.objectCount)
			}
		})
	}
}

// BenchmarkOptimizedInformerSupplier benchmarks the custom cache informer supplier.
func BenchmarkOptimizedInformerSupplier(b *testing.B) {
	if err := suppressLogger(); err != nil {
		b.Fatalf("failed to suppress logger: %v", err)
	}

	scenarios := []struct {
		name              string
		objectCount       int
		useWatchList      bool
		watchListPageSize int64
	}{
		{"10_objects", 10, false, 0},
		{"100_objects", 100, false, 0},
		{"1000_objects", 1000, false, 0},
		{"10000_objects", 10000, false, 0},
		{"10000_objects_page", 10000, false, 1000},
		{"10000_objects_watchlist", 10000, true, 0},
	}

	for _, scenario := range scenarios {
		// Setup: Create mock client with objects (done once, outside timing)
		kind := benchmarkKind()
		objects := generateTestObjects(scenario.objectCount)

		b.ReportAllocs()

		b.Run(scenario.name, func(b *testing.B) {
			clientGen := &mockClientGeneratorWithK8sClient{
				kind:    kind,
				objects: objects,
			}

			// Configure options
			opts := operator.InformerOptions{
				ListWatchOptions:  operator.ListWatchOptions{},
				WatchListPageSize: scenario.watchListPageSize,
				UseWatchList:      scenario.useWatchList,
			}

			for b.Loop() {
				// Create informer via OptimizedInformerSupplier
				inf, err := simple.OptimizedInformerSupplier(kind, clientGen, opts)
				if err != nil {
					b.Fatalf("failed to create informer: %v", err)
				}

				// Run informer and validate event delivery
				runInformerWithValidation(b, inf, scenario.objectCount)
			}
		})
	}
}

// BenchmarkBusInformerSupplier benchmarks the bus-driven informer's initial list, which is what its snapshot costs:
// one entry per object, each holding the listed object.
func BenchmarkBusInformerSupplier(b *testing.B) {
	if err := suppressLogger(); err != nil {
		b.Fatalf("failed to suppress logger: %v", err)
	}

	scenarios := []struct {
		name        string
		objectCount int
		pageSize    int
	}{
		{"10_objects", 10, 0},
		{"100_objects", 100, 0},
		{"1000_objects", 1000, 0},
		{"10000_objects", 10000, 0},
		{"10000_objects_page_5000", 10000, 5000},
		{"50000_objects_page_5000", 50000, 5000},
	}

	for _, scenario := range scenarios {
		kind := benchmarkKind()
		objects := generateTestObjects(scenario.objectCount)

		b.ReportAllocs()

		b.Run(scenario.name, func(b *testing.B) {
			clientGen := &mockClientGeneratorWithK8sClient{
				kind:    kind,
				objects: objects,
			}
			supplier := simple.BusInformerSupplier(resourcebus.NewFake(), simple.BusInformerConfig{
				PageSize: scenario.pageSize,
			})

			for b.Loop() {
				inf, err := supplier(kind, clientGen, operator.InformerOptions{})
				if err != nil {
					b.Fatalf("failed to create informer: %v", err)
				}

				runInformerWithValidation(b, inf, scenario.objectCount)
			}
		})
	}
}

// BenchmarkBusInformerLiveEvents benchmarks the per-event cost of the identity payload, which allocates the kind's
// zero value per event and retains one per added object in the snapshot. This is what a metadata-only object type
// would reduce, so it is the measurement to compare such a type against.
func BenchmarkBusInformerLiveEvents(b *testing.B) {
	if err := suppressLogger(); err != nil {
		b.Fatalf("failed to suppress logger: %v", err)
	}

	for _, eventCount := range []int{10000, 50000} {
		kind := benchmarkKind()
		gvr := kind.GroupVersionResource()

		b.ReportAllocs()

		b.Run(fmt.Sprintf("%d_events", eventCount), func(b *testing.B) {
			for b.Loop() {
				sub := resourcebus.NewFake()
				inf, err := operator.NewBusInformer(kind, sub, emptyLister{}, operator.BusInformerOptions{
					Payload:        operator.PayloadIdentity,
					ResyncInterval: time.Hour,
					// The subscriber callback only enqueues, so the whole burst has to fit: a full queue drops
					// events and forces a re-list, and emptyLister has nothing to recover them from.
					EventQueueSize: eventCount,
				})
				if err != nil {
					b.Fatalf("failed to create informer: %v", err)
				}

				var received atomic.Int32
				done := make(chan struct{})
				if err := inf.AddEventHandler(&operator.SimpleWatcher{
					AddFunc: func(context.Context, resource.Object) error {
						if int(received.Add(1)) == eventCount {
							close(done)
						}
						return nil
					},
				}); err != nil {
					b.Fatalf("failed to add event handler: %v", err)
				}

				ctx, cancel := context.WithCancel(b.Context())
				runDone := make(chan struct{})
				go func() {
					defer close(runDone)
					_ = inf.Run(ctx)
				}()
				if err := inf.WaitForSync(ctx); err != nil {
					cancel()
					b.Fatalf("failed to wait for informer to sync: %v", err)
				}

				for i := range eventCount {
					if err := sub.Publish(ctx, gvr, "", resourcebus.Event{
						Verb:            resourcebus.VerbAdded,
						Name:            fmt.Sprintf("object-%d", i),
						ResourceVersion: strconv.Itoa(i + 1),
					}); err != nil {
						cancel()
						b.Fatalf("failed to publish: %v", err)
					}
				}

				// Publish returns once the event is queued, so wait for the handler to have seen all of them
				// before stopping: the measurement is of delivery through to the watcher, not of enqueueing.
				select {
				case <-done:
				case <-ctx.Done():
					cancel()
					b.Fatalf("timeout waiting for events, got %d of %d", received.Load(), eventCount)
				}

				cancel()
				<-runDone
			}
		})
	}
}

// emptyLister stands in for the periodic re-list, so that the live-event benchmark measures only delivery.
type emptyLister struct{}

func (emptyLister) List(context.Context) ([]resource.Object, string, error) { return nil, "", nil }
