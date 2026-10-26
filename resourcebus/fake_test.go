package resourcebus

import (
	"context"
	"testing"

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
