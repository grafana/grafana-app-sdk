package operator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// recordingHandler captures what a client-go handler is told, as a controller written against one would read it.
type recordingHandler struct {
	mu      sync.Mutex
	adds    []string
	initial []bool
	updates []string
	deletes []any
}

var _ cache.ResourceEventHandler = (*recordingHandler)(nil)

func (h *recordingHandler) OnAdd(obj any, isInInitialList bool) { //nolint:revive // client-go's signature.
	h.mu.Lock()
	defer h.mu.Unlock()
	h.adds = append(h.adds, obj.(resource.Object).GetName())
	h.initial = append(h.initial, isInInitialList)
}

func (h *recordingHandler) OnUpdate(_, newObj any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.updates = append(h.updates, newObj.(resource.Object).GetName())
}

func (h *recordingHandler) OnDelete(obj any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deletes = append(h.deletes, obj)
}

func (h *recordingHandler) snapshot() ([]string, []bool, []string, []any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.adds...), append([]bool(nil), h.initial...),
		append([]string(nil), h.updates...), append([]any(nil), h.deletes...)
}

func TestDeltaSource_AddEventHandlerRejectsNil(t *testing.T) {
	_, err := NewDeltaSource(newBusInformer(t, resourcebus.NewFake(), staticLister(), nil, BusInformerOptions{})).
		AddEventHandler(nil)
	assert.Error(t, err)
}

// What a controller written against client-go reads: the initial list flagged as such, a live add not, and a
// re-list-detected delete as the tombstone which says the object carried with it may be stale.
func TestDeltaSource_MapsDeliveriesToClientGoSemantics(t *testing.T) {
	var calls int
	lister := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		calls++
		if calls == 1 {
			return []resource.Object{busObjRV("a", 10)}, "100", nil
		}
		return nil, "200", nil
	})
	inf := newBusInformer(t, resourcebus.NewFake(), lister, nil, BusInformerOptions{})
	source := NewDeltaSource(inf)
	handler := &recordingHandler{}
	reg, err := source.AddEventHandler(handler)
	require.NoError(t, err)

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	adds, initial, _, _ := handler.snapshot()
	assert.Equal(t, []string{"a"}, adds)
	assert.Equal(t, []bool{true}, initial, "the informer has not reported synced yet")
	assert.False(t, reg.HasSynced())

	source.markSynced()
	assert.True(t, reg.HasSynced())
	select {
	case <-reg.HasSyncedChecker().Done():
	default:
		t.Fatal("the sync checker never completed")
	}

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbAdded, "b", "101"))
	_, initial, _, _ = handler.snapshot()
	assert.Equal(t, []bool{true, false}, initial, "a live add is not an initial-list delivery")

	inf.handleEvent(context.Background(), busEvent(resourcebus.VerbDeleted, "b", "102"))
	_, _, _, deletes := handler.snapshot()
	require.Len(t, deletes, 1)
	assert.IsType(t, &resource.UntypedObject{}, deletes[0], "a live delete carries the object itself")

	// The periodic re-list drops "a", which the informer inferred rather than was told.
	require.NoError(t, inf.relist(context.Background(), periodicRelist))
	_, _, _, deletes = handler.snapshot()
	require.Len(t, deletes, 2)
	tombstone, ok := deletes[1].(cache.DeletedFinalStateUnknown)
	require.True(t, ok, "a re-list-detected delete is a tombstone")
	assert.Equal(t, busTestNamespace+"/a", tombstone.Key)
	assert.Equal(t, "a", tombstone.Obj.(resource.Object).GetName())
}

// A re-list update is an update either way: client-go calls OnUpdate on a re-list too, even when nothing changed.
func TestDeltaSource_RelistUpdateIsAnUpdate(t *testing.T) {
	lister := staticLister(busObjRV("a", 10))
	inf := newBusInformer(t, resourcebus.NewFake(), lister, nil, BusInformerOptions{})
	source := NewDeltaSource(inf)
	handler := &recordingHandler{}
	_, err := source.AddEventHandler(handler)
	require.NoError(t, err)

	require.NoError(t, inf.relist(context.Background(), initialRelist))
	require.NoError(t, inf.relist(context.Background(), periodicRelist))

	adds, _, updates, _ := handler.snapshot()
	assert.Equal(t, []string{"a"}, adds)
	assert.Equal(t, []string{"a"}, updates)
}

// The registration's sync state is the informer's, so cache.WaitForCacheSync over it returns once the initial list
// is in.
func TestDeltaSource_RunSyncsTheRegistration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inf := newBusInformer(t, resourcebus.NewFake(), staticLister(busObjRV("a", 10)), nil, BusInformerOptions{})
		source := NewDeltaSource(inf)
		reg, err := source.AddEventHandler(&recordingHandler{})
		require.NoError(t, err)

		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			source.Run(stop)
		}()
		synctest.Wait()

		assert.True(t, cache.WaitForCacheSync(stop, reg.HasSynced))

		close(stop)
		<-done
	})
}

// Run has nowhere to return an error, so a failure which stops the informer must not be swallowed silently —
// RunWithContext is the way to get it.
func TestDeltaSource_RunWithContextReturnsTheError(t *testing.T) {
	wanted := errors.New("I AM ERROR")
	source := NewDeltaSource(stoppingInformer{err: wanted})

	err := source.RunWithContext(context.Background())

	assert.ErrorIs(t, err, wanted)
	assert.False(t, source.HasSynced())
}

func TestDeltaSource_RunStopsWithTheChannel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := NewDeltaSource(blockingInformer{})
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			source.Run(stop)
		}()
		synctest.Wait()

		close(stop)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Run did not return when the stop channel closed")
		}
	})
}

type stoppingInformer struct{ err error }

func (i stoppingInformer) Run(context.Context) error                   { return i.err }
func (stoppingInformer) WaitForSync(ctx context.Context) error         { <-ctx.Done(); return ctx.Err() }
func (stoppingInformer) AddEventHandler(handler ResourceWatcher) error { _ = handler; return nil }

type blockingInformer struct{}

func (blockingInformer) Run(ctx context.Context) error                 { <-ctx.Done(); return ctx.Err() }
func (blockingInformer) WaitForSync(ctx context.Context) error         { <-ctx.Done(); return ctx.Err() }
func (blockingInformer) AddEventHandler(handler ResourceWatcher) error { _ = handler; return nil }
