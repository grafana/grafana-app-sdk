package resourcebus

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var testGVR = schema.GroupVersionResource{Group: "example.grafana.app", Version: "v1", Resource: "widgets"}

const testNamespace = "default"

func TestNewSubscribeConfig(t *testing.T) {
	cfg := NewSubscribeConfig(WithGroup(Group{Name: "group", Delivery: GroupDeliveryRoundRobin}),
		WithOnReconnect(func() {}), nil)
	assert.Equal(t, Group{Name: "group", Delivery: GroupDeliveryRoundRobin}, cfg.Group)
	assert.NotNil(t, cfg.OnReconnect)
	assert.Equal(t, SubscribeConfig{}, NewSubscribeConfig())
}

func TestFake_DeliversToEverySubscription(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	var first, second []Event
	_, err := f.Subscribe(ctx, testGVR, testNamespace, func(_ context.Context, evt Event) {
		first = append(first, evt)
	}, WithGroup(Group{Name: "group"}))
	require.NoError(t, err)
	sub2, err := f.Subscribe(ctx, testGVR, testNamespace, func(_ context.Context, evt Event) {
		second = append(second, evt)
	})
	require.NoError(t, err)

	assert.True(t, f.Subscribed(testGVR, testNamespace))
	assert.Equal(t, Group{Name: "group"}, f.Group(testGVR, testNamespace))

	evt := Event{Verb: VerbAdded, Namespace: testNamespace, Name: "a", ResourceVersion: "1"}
	require.NoError(t, f.Publish(ctx, testGVR, testNamespace, evt))
	assert.Equal(t, []Event{evt}, first)
	assert.Equal(t, []Event{evt}, second)

	require.NoError(t, sub2.Close())
	require.NoError(t, f.Publish(ctx, testGVR, testNamespace, evt))
	assert.Len(t, first, 2, "the open subscription keeps receiving")
	assert.Len(t, second, 1, "the closed subscription does not")
}

func TestFake_PublishWithoutSubscription(t *testing.T) {
	err := NewFake().Publish(context.Background(), testGVR, testNamespace, Event{Verb: VerbAdded, Name: "a"})
	assert.ErrorIs(t, err, ErrNoSubscription)
}

func TestFake_FailSubscribes(t *testing.T) {
	f := NewFake()
	f.FailSubscribes(2)
	handler := func(context.Context, Event) {}

	_, err := f.Subscribe(context.Background(), testGVR, testNamespace, handler)
	assert.Error(t, err)
	_, err = f.Subscribe(context.Background(), testGVR, testNamespace, handler)
	assert.Error(t, err)
	_, err = f.Subscribe(context.Background(), testGVR, testNamespace, handler)
	assert.NoError(t, err, "only the first two attempts fail")
}

func TestFake_Reconnect(t *testing.T) {
	f := NewFake()
	var reconnects int
	_, err := f.Subscribe(context.Background(), testGVR, testNamespace, func(context.Context, Event) {},
		WithOnReconnect(func() { reconnects++ }))
	require.NoError(t, err)

	f.Reconnect(testGVR, testNamespace)
	f.Reconnect(testGVR, "other")
	assert.Equal(t, 1, reconnects, "only the subscribed namespace reconnects")
}

func TestFake_SubscribeRejectsNilHandler(t *testing.T) {
	_, err := NewFake().Subscribe(context.Background(), testGVR, testNamespace, nil)
	assert.Error(t, err)
}

// A group shares delivery: each event reaches one member, in turn, which is what lets a test exercise the events a
// replica never sees live and recovers from its next re-list.
func TestFake_GroupSharesDelivery(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	group := Group{Name: "replicas", Delivery: GroupDeliveryRoundRobin}

	var first, second, ungrouped int
	_, err := f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) { first++ }, WithGroup(group))
	require.NoError(t, err)
	_, err = f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) { second++ }, WithGroup(group))
	require.NoError(t, err)
	_, err = f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) { ungrouped++ })
	require.NoError(t, err)

	for range 4 {
		require.NoError(t, f.Publish(ctx, testGVR, testNamespace, Event{Verb: VerbAdded, Name: "a"}))
	}

	assert.Equal(t, 2, first, "each member takes its turn")
	assert.Equal(t, 2, second)
	assert.Equal(t, 4, ungrouped, "a subscription outside any group still receives every event")
}

// An empty namespace means every namespace, per Subscriber, so a namespaced publish has to reach it.
func TestFake_AllNamespaceSubscriptionReceivesNamespacedEvents(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	var all, scoped int
	_, err := f.Subscribe(ctx, testGVR, "", func(context.Context, Event) { all++ })
	require.NoError(t, err)
	_, err = f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) { scoped++ })
	require.NoError(t, err)

	assert.True(t, f.Subscribed(testGVR, testNamespace))
	assert.True(t, f.Subscribed(testGVR, "other"), "the all-namespace subscription covers it")

	require.NoError(t, f.Publish(ctx, testGVR, testNamespace, Event{Verb: VerbAdded, Name: "a"}))
	require.NoError(t, f.Publish(ctx, testGVR, "other", Event{Verb: VerbAdded, Name: "b"}))

	assert.Equal(t, 2, all, "both namespaces reach the all-namespace subscription")
	assert.Equal(t, 1, scoped, "only its own namespace reaches the scoped one")
}

// Each scope picks its own group member, so an all-namespace group and a namespaced one do not share a turn.
func TestFake_GroupTurnsArePerScope(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	group := Group{Name: "replicas"}

	var all, scoped int
	_, err := f.Subscribe(ctx, testGVR, "", func(context.Context, Event) { all++ }, WithGroup(group))
	require.NoError(t, err)
	_, err = f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) { scoped++ }, WithGroup(group))
	require.NoError(t, err)

	require.NoError(t, f.Publish(ctx, testGVR, testNamespace, Event{Verb: VerbAdded, Name: "a"}))

	assert.Equal(t, 1, all, "the sole member of the all-namespace group takes its turn")
	assert.Equal(t, 1, scoped, "as does the sole member of the namespaced group")
}

// Delivery stops when the subscribe context is done, as well as on Close.
func TestFake_CancelingTheContextStopsDelivery(t *testing.T) {
	f := NewFake()
	ctx, cancel := context.WithCancel(context.Background())

	var received int
	_, err := f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) { received++ })
	require.NoError(t, err)
	require.NoError(t, f.Publish(context.Background(), testGVR, testNamespace, Event{Verb: VerbAdded, Name: "a"}))
	require.Equal(t, 1, received)

	cancel()
	assert.Eventually(t, func() bool { return !f.Subscribed(testGVR, testNamespace) }, time.Second, time.Millisecond)

	assert.ErrorIs(t, f.Publish(context.Background(), testGVR, testNamespace, Event{Verb: VerbAdded, Name: "b"}),
		ErrNoSubscription)
	assert.Equal(t, 1, received, "nothing is delivered after cancellation")
}

// A context which is already done must never be registered, and synchronously: a Publish immediately after Subscribe
// returns has to find nothing, with no window in which a watcher goroutine has yet to run.
func TestFake_AlreadyCanceledContextIsNeverRegistered(t *testing.T) {
	f := NewFake()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var received int
	_, err := f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) { received++ })
	require.NoError(t, err)

	assert.False(t, f.Subscribed(testGVR, testNamespace), "not registered, with no goroutine to wait for")
	assert.ErrorIs(t, f.Publish(context.Background(), testGVR, testNamespace, Event{Verb: VerbAdded, Name: "a"}),
		ErrNoSubscription)
	assert.Zero(t, received)
}

// A canceled context must not take an event away from a live member of the same group.
func TestFake_AlreadyCanceledContextDoesNotTakeAGroupTurn(t *testing.T) {
	f := NewFake()
	group := Group{Name: "replicas"}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	var dead, live int
	_, err := f.Subscribe(canceled, testGVR, testNamespace, func(context.Context, Event) { dead++ }, WithGroup(group))
	require.NoError(t, err)
	_, err = f.Subscribe(context.Background(), testGVR, testNamespace,
		func(context.Context, Event) { live++ }, WithGroup(group))
	require.NoError(t, err)

	for range 4 {
		require.NoError(t, f.Publish(context.Background(), testGVR, testNamespace, Event{Verb: VerbAdded, Name: "a"}))
	}

	assert.Zero(t, dead)
	assert.Equal(t, 4, live, "the live member takes every turn, not every other one")
}

// Close releases the context watcher, so repeated subscribe/close cycles under one long-lived context retain
// nothing — the watcher used to wait on a cancellation that never came.
func TestFake_CloseReleasesTheContextWatcher(t *testing.T) {
	f := NewFake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := runtime.NumGoroutine()
	for range 200 {
		sub, err := f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) {})
		require.NoError(t, err)
		require.NoError(t, sub.Close())
	}

	assert.Eventually(t, func() bool { return runtime.NumGoroutine() <= before+2 }, time.Second, time.Millisecond,
		"closed subscriptions must not retain a watcher each")
	assert.False(t, f.Subscribed(testGVR, testNamespace))
}

// Closing twice, or closing after the context is canceled, must not panic or double-release.
func TestFake_CloseIsIdempotent(t *testing.T) {
	f := NewFake()
	ctx, cancel := context.WithCancel(context.Background())

	sub, err := f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) {})
	require.NoError(t, err)
	require.NoError(t, sub.Close())
	require.NoError(t, sub.Close())

	cancel()
	assert.False(t, f.Subscribed(testGVR, testNamespace))
}

// A handler which closes another subscription stops delivery to it, including within the same publish: the
// recipient list is resolved before the first handler runs, so each one is re-checked before it is invoked.
func TestFake_HandlerClosingAnotherSubscriptionStopsItsDelivery(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	var firstCalls, secondCalls int
	var second Subscription
	_, err := f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) {
		firstCalls++
		require.NoError(t, second.Close())
	})
	require.NoError(t, err)
	second, err = f.Subscribe(ctx, testGVR, testNamespace, func(context.Context, Event) { secondCalls++ })
	require.NoError(t, err)

	require.NoError(t, f.Publish(ctx, testGVR, testNamespace, Event{Verb: VerbAdded, Name: "a"}))

	assert.Equal(t, 1, firstCalls)
	assert.Zero(t, secondCalls, "closed before its turn in this very publish")

	require.NoError(t, f.Publish(ctx, testGVR, testNamespace, Event{Verb: VerbAdded, Name: "b"}))
	assert.Equal(t, 2, firstCalls)
	assert.Zero(t, secondCalls)
}
