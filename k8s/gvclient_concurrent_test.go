package k8s

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/grafana/grafana-app-sdk/resource"
)

// testCodec is a simple codec for testing
type testCodec struct{}

func (testCodec) Read(r io.Reader, into resource.Object) error {
	// For testing, just return nil - we're testing the concurrency, not the decoding
	return nil
}

func (testCodec) Write(w io.Writer, obj resource.Object) error {
	return nil
}

// TestWatchResponse_ConcurrentDecoding tests that concurrent decoding works correctly
func TestWatchResponse_ConcurrentDecoding(t *testing.T) {
	// Create a mock watch interface
	mockWatch := &mockWatchInterface{
		resultChan: make(chan watch.Event, 100),
	}

	// Create example object
	exampleObj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}

	// Create WatchResponse with concurrent decoding
	wr := &WatchResponse{
		watch:          mockWatch,
		ch:             make(chan resource.WatchEvent, 100),
		stopCh:         make(chan struct{}),
		ex:             exampleObj,
		codec:          testCodec{},
		decoderWorkers: 4,
	}

	// Start the watch response
	go wr.start()

	// Send test events
	numEvents := 100
	for i := range numEvents {
		obj := &resource.UntypedObject{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "test/v1",
				Kind:       "TestObject",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      fmt.Sprintf("obj-%d", i),
			},
		}
		mockWatch.resultChan <- watch.Event{
			Type:   watch.Added,
			Object: obj,
		}
	}

	// Read events and collect them
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	receivedNames := make(map[string]bool)
	for range numEvents {
		select {
		case evt := <-wr.WatchEvents():
			receivedNames[evt.Object.GetName()] = true
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for events, received %d/%d", len(receivedNames), numEvents)
		}
	}

	// Verify we received all events
	assert.Equal(t, numEvents, len(receivedNames))
	for i := range numEvents {
		expectedName := fmt.Sprintf("obj-%d", i)
		assert.True(t, receivedNames[expectedName], "Missing event for %s", expectedName)
	}

	// Stop the watch
	wr.Stop()
}

// TestWatchResponse_ConcurrentDecoding_PerObjectOrdering tests that events for the same object maintain order
func TestWatchResponse_ConcurrentDecoding_PerObjectOrdering(t *testing.T) {
	mockWatch := &mockWatchInterface{
		resultChan: make(chan watch.Event, 100),
	}

	exampleObj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}

	wr := &WatchResponse{
		watch:          mockWatch,
		ch:             make(chan resource.WatchEvent, 100),
		stopCh:         make(chan struct{}),
		ex:             exampleObj,
		codec:          testCodec{},
		decoderWorkers: 4,
	}

	go wr.start()

	// Send multiple events for the same object
	objectName := "test-object"
	numEventsPerObject := 10

	for i := range numEventsPerObject {
		obj := &resource.UntypedObject{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "test/v1",
				Kind:       "TestObject",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       "default",
				Name:            objectName,
				ResourceVersion: fmt.Sprintf("%d", i),
			},
		}
		mockWatch.resultChan <- watch.Event{
			Type:   watch.Modified,
			Object: obj,
		}
	}

	// Read events and verify they're in order
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := range numEventsPerObject {
		select {
		case evt := <-wr.WatchEvents():
			expectedVersion := fmt.Sprintf("%d", i)
			assert.Equal(t, expectedVersion, evt.Object.GetResourceVersion())
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for event %d", i)
		}
	}

	wr.Stop()
}

// TestWatchResponse_SyncDecoding tests that synchronous decoding still works (backwards compatibility)
func TestWatchResponse_SyncDecoding(t *testing.T) {
	mockWatch := &mockWatchInterface{
		resultChan: make(chan watch.Event, 10),
	}

	exampleObj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}

	// Create WatchResponse WITHOUT concurrent decoding (decoderWorkers = 0)
	wr := &WatchResponse{
		watch:          mockWatch,
		ch:             make(chan resource.WatchEvent, 10),
		stopCh:         make(chan struct{}),
		ex:             exampleObj,
		codec:          testCodec{},
		decoderWorkers: 0, // Synchronous
	}

	go wr.start()

	// Send a test event
	obj := &resource.UntypedObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "test/v1",
			Kind:       "TestObject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-obj",
		},
	}
	mockWatch.resultChan <- watch.Event{
		Type:   watch.Added,
		Object: obj,
	}

	// Read and verify
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case evt := <-wr.WatchEvents():
		assert.Equal(t, "test-obj", evt.Object.GetName())
	case <-ctx.Done():
		t.Fatal("Timeout waiting for event")
	}

	wr.Stop()
}

// TestWatchResponse_ConcurrentDecoding_StressTest tests concurrent decoding under load
func TestWatchResponse_ConcurrentDecoding_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	mockWatch := &mockWatchInterface{
		resultChan: make(chan watch.Event, 1000),
	}

	exampleObj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}

	wr := &WatchResponse{
		watch:          mockWatch,
		ch:             make(chan resource.WatchEvent, 1000),
		stopCh:         make(chan struct{}),
		ex:             exampleObj,
		codec:          testCodec{},
		decoderWorkers: 8,
	}

	go wr.start()

	// Send many events
	numEvents := 1000
	var wg sync.WaitGroup
	wg.Add(1)

	// Producer goroutine
	go func() {
		defer wg.Done()
		for i := range numEvents {
			obj := &resource.UntypedObject{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "test/v1",
					Kind:       "TestObject",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      fmt.Sprintf("obj-%d", i),
				},
			}
			mockWatch.resultChan <- watch.Event{
				Type:   watch.Added,
				Object: obj,
			}
		}
	}()

	// Consumer goroutine - collect all events
	receivedNames := make(map[string]bool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for range numEvents {
		select {
		case evt := <-wr.WatchEvents():
			receivedNames[evt.Object.GetName()] = true
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for events, received %d/%d", len(receivedNames), numEvents)
		}
	}

	wg.Wait()

	// Verify all events received
	require.Equal(t, numEvents, len(receivedNames))
	for i := range numEvents {
		expectedName := fmt.Sprintf("obj-%d", i)
		assert.True(t, receivedNames[expectedName], "Missing event for %s", expectedName)
	}

	wr.Stop()
}

// mockWatchInterface is a mock implementation of watch.Interface for testing
type mockWatchInterface struct {
	resultChan chan watch.Event
	stopped    bool
	mu         sync.Mutex
}

func (m *mockWatchInterface) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		close(m.resultChan)
		m.stopped = true
	}
}

func (m *mockWatchInterface) ResultChan() <-chan watch.Event {
	return m.resultChan
}
