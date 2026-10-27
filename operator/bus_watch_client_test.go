package operator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// fakeGetter serves the hydrated payload's reads.
type fakeGetter struct {
	mu      sync.Mutex
	objects map[string]resource.Object
	err     error
	gets    int
}

func (g *fakeGetter) Get(_ context.Context, id resource.Identifier) (resource.Object, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gets++
	if g.err != nil {
		return nil, g.err
	}
	obj, ok := g.objects[relistStoreKey(id.Namespace, id.Name)]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: relistTestKind.Group(), Resource: relistTestKind.Plural()}, id.Name)
	}
	return obj, nil
}

func newBusSource(t *testing.T, sub resourcebus.Subscriber, opts BusWatchOptions, getter *fakeGetter) *BusWatchClient {
	t.Helper()
	if getter == nil {
		getter = &fakeGetter{}
	}
	client, err := NewBusWatchClient(getter, sub, relistTestKind, opts)
	require.NoError(t, err)
	return client
}

// watchBus opens a watch on the client for the test namespace and returns it with a stop func which must be deferred.
func watchBus(t *testing.T, client *BusWatchClient) (resource.WatchResponse, func()) {
	t.Helper()
	w, err := client.Watch(context.Background(), relistTestNamespace, resource.WatchOptions{})
	require.NoError(t, err)
	return w, func() {
		w.Stop()
		synctest.Wait()
	}
}

func TestNewBusWatchClient_Validation(t *testing.T) {
	sub := resourcebus.NewFake()
	getter := &fakeGetter{}

	_, err := NewBusWatchClient(getter, nil, relistTestKind, BusWatchOptions{})
	assert.Error(t, err, "a nil subscriber is rejected")

	_, err = NewBusWatchClient(getter, sub, resource.Kind{}, BusWatchOptions{})
	assert.Error(t, err, "an empty kind is rejected")

	_, err = NewBusWatchClient(getter, sub, relistTestKind, BusWatchOptions{Payload: "nonsense"})
	assert.Error(t, err, "an unknown payload is rejected")

	_, err = NewBusWatchClient(nil, sub, relistTestKind, BusWatchOptions{Payload: PayloadHydrated})
	assert.Error(t, err, "the hydrated payload requires a getter")

	identity, err := NewBusWatchClient(nil, sub, relistTestKind, BusWatchOptions{})
	require.NoError(t, err, "the identity payload reads nothing, so needs no getter")
	assert.True(t, identity.IdentityPayload(), "the identity payload is the default")

	hydrated, err := NewBusWatchClient(getter, sub, relistTestKind, BusWatchOptions{Payload: PayloadHydrated})
	require.NoError(t, err)
	assert.False(t, hydrated.IdentityPayload())
}

// The bus watch is interchangeable with a resource.Client as a ChangeSource, but is not a ListWatchClient: a
// reflector-based informer would re-list in a loop against a watch which cannot resume, so it must not accept one.
func TestBusWatchClient_IsAChangeSourceButNotAListWatchClient(t *testing.T) {
	var source any = newBusSource(t, resourcebus.NewFake(), BusWatchOptions{}, nil)

	_, isSource := source.(ChangeSource)
	assert.True(t, isSource)
	_, isListWatch := source.(ListWatchClient)
	assert.False(t, isListWatch)
}

// The bus cannot replay, so a watch which asks to resume or for initial events is refused rather than silently
// starting from now.
func TestBusWatchClient_RefusesWhatTheBusCannotServe(t *testing.T) {
	client := newBusSource(t, resourcebus.NewFake(), BusWatchOptions{}, nil)
	sendInitial := true

	_, err := client.Watch(context.Background(), "", resource.WatchOptions{ResourceVersion: "42"})
	assert.Error(t, err)

	_, err = client.Watch(context.Background(), "", resource.WatchOptions{SendInitialEvents: &sendInitial})
	assert.Error(t, err)
}

// The group reaches the bus whole, delivery included: a subscriber which cannot provide the requested sharing has
// to be able to see what was asked for in order to refuse it. With no group nothing is passed.
func TestBusWatchClient_SubscribesWithTheConfiguredGroup(t *testing.T) {
	for _, group := range []resourcebus.Group{
		{},
		{Name: "widgets-operator", Delivery: resourcebus.GroupDeliveryRoundRobin},
	} {
		synctest.Test(t, func(t *testing.T) {
			sub := resourcebus.NewFake()
			_, stop := watchBus(t, newBusSource(t, sub, BusWatchOptions{Group: group}, nil))
			defer stop()

			assert.Equal(t, group, sub.Group(relistTestGVR(), relistTestNamespace))
		})
	}
}

// Stopping the watch closes its subscription and its channel.
func TestBusWatchClient_StopClosesTheSubscription(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := resourcebus.NewFake()
		w, stop := watchBus(t, newBusSource(t, sub, BusWatchOptions{}, nil))
		require.True(t, sub.Subscribed(relistTestGVR(), relistTestNamespace))

		stop()

		assert.False(t, sub.Subscribed(relistTestGVR(), relistTestNamespace))
		_, open := <-w.WatchEvents()
		assert.False(t, open)
	})
}

// A reconnect, or an event which finds the buffer full, means events were lost: the watch ends so that its consumer
// recovers them.
func TestBusWatchClient_LostEventsEndTheWatch(t *testing.T) {
	t.Run("reconnect", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			sub := resourcebus.NewFake()
			w, stop := watchBus(t, newBusSource(t, sub, BusWatchOptions{}, nil))
			defer stop()

			sub.Reconnect(relistTestGVR(), relistTestNamespace)
			synctest.Wait()

			_, open := <-w.WatchEvents()
			assert.False(t, open)
			assert.False(t, sub.Subscribed(relistTestGVR(), relistTestNamespace))
		})
	})

	t.Run("full buffer", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			sub := resourcebus.NewFake()
			w, stop := watchBus(t, newBusSource(t, sub, BusWatchOptions{BufferSize: 1}, nil))
			defer stop()

			// Nothing reads the watch, so the first event is taken off the buffer and held for the consumer, the
			// second fills the buffer, and the third finds it full. The held event is dropped too.
			ctx := context.Background()
			for _, name := range []string{"a", "b", "c"} {
				require.NoError(t, sub.Publish(ctx, relistTestGVR(), relistTestNamespace, busEvent(resourcebus.VerbAdded, name, "1")))
				synctest.Wait()
			}

			var delivered []string
			for evt := range w.WatchEvents() {
				delivered = append(delivered, evt.Object.GetName())
			}
			assert.Empty(t, delivered, "the watch ends rather than delivering past the loss")
		})
	})
}

func TestBusWatchClient_ToWatchEvent(t *testing.T) {
	ctx := context.Background()
	hydratedObj := relistObj("x")
	hydratedObj.ResourceVersion = "200"
	hydratedObj.Spec = map[string]any{"field": "x"}

	t.Run("identity", func(t *testing.T) {
		client := newBusSource(t, resourcebus.NewFake(), BusWatchOptions{}, nil)
		for verb, eventType := range map[resourcebus.Verb]string{
			resourcebus.VerbAdded:    watchEventAdded,
			resourcebus.VerbModified: watchEventModified,
			resourcebus.VerbDeleted:  watchEventDeleted,
		} {
			evt, ok := client.toWatchEvent(ctx, busEvent(verb, "x", "200"))
			require.True(t, ok)
			assert.Equal(t, eventType, evt.EventType)
			assert.Equal(t, "x", evt.Object.GetName())
			assert.Equal(t, "200", evt.Object.GetResourceVersion())
			assert.Empty(t, evt.Object.GetSpec())
		}
	})

	t.Run("an unknown verb is dropped", func(t *testing.T) {
		var handled []error
		client := newBusSource(t, resourcebus.NewFake(), BusWatchOptions{
			ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
		}, nil)

		_, ok := client.toWatchEvent(ctx, busEvent("nonsense", "x", "1"))

		assert.False(t, ok)
		require.Len(t, handled, 1)
		assert.Contains(t, handled[0].Error(), "unknown verb")
	})

	t.Run("hydrated delivers the object read from the API", func(t *testing.T) {
		getter := &fakeGetter{objects: map[string]resource.Object{relistStoreKey(relistTestNamespace, "x"): hydratedObj}}
		client := newBusSource(t, resourcebus.NewFake(), BusWatchOptions{Payload: PayloadHydrated}, getter)

		evt, ok := client.toWatchEvent(ctx, busEvent(resourcebus.VerbModified, "x", "200"))

		require.True(t, ok)
		assert.Equal(t, watchEventModified, evt.EventType)
		assert.Equal(t, map[string]any{"field": "x"}, evt.Object.GetSpec())
	})

	t.Run("hydrated drops an event whose object cannot be read", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			getter *fakeGetter
			want   string
		}{
			{name: "read failure", getter: &fakeGetter{err: errors.New("api unavailable")}, want: "api unavailable"},
			{name: "deleted mid-flight", getter: &fakeGetter{}, want: "no longer exists"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				var handled []error
				client := newBusSource(t, resourcebus.NewFake(), BusWatchOptions{
					Payload:      PayloadHydrated,
					ErrorHandler: func(_ context.Context, err error) { handled = append(handled, err) },
				}, tt.getter)

				_, ok := client.toWatchEvent(ctx, busEvent(resourcebus.VerbAdded, "x", "200"))

				assert.False(t, ok)
				require.Len(t, handled, 1)
				assert.Contains(t, handled[0].Error(), tt.want)
			})
		}
	})

	t.Run("hydrated delivers a delete without a read", func(t *testing.T) {
		getter := &fakeGetter{}
		client := newBusSource(t, resourcebus.NewFake(), BusWatchOptions{Payload: PayloadHydrated}, getter)

		evt, ok := client.toWatchEvent(ctx, busEvent(resourcebus.VerbDeleted, "x", "200"))

		require.True(t, ok)
		assert.Equal(t, watchEventDeleted, evt.EventType)
		assert.Zero(t, getter.gets, "the object is already gone from storage, so there is nothing to read")
	})
}
