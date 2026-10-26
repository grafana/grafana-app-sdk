package resourcebus

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ErrNoSubscription indicates that a Fake was asked to publish to a resource and namespace nothing is subscribed to.
var ErrNoSubscription = errors.New("no subscription")

// Fake is an in-memory Subscriber for tests: it records subscriptions and delivers published events to their handlers
// synchronously, on the calling goroutine, so a test needs no bus and no waiting.
//
// It honors Group: subscriptions sharing a name receive one event each in turn, rather than all of them, so a test
// can exercise what a replica misses live and recovers from the next re-list.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type Fake struct {
	mu    sync.Mutex
	subs  map[string][]*fakeSubscription
	failN int
	// next is the round-robin cursor per resource, namespace and group name, so grouped subscriptions take turns.
	next map[string]int
}

var _ Subscriber = (*Fake)(nil)

// NewFake returns a Fake with no subscriptions.
func NewFake() *Fake {
	return &Fake{subs: map[string][]*fakeSubscription{}, next: map[string]int{}}
}

// FailSubscribes makes the next n Subscribe calls fail, mimicking a bus which is not available yet.
func (f *Fake) FailSubscribes(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failN = n
}

// Subscribe implements Subscriber.
func (f *Fake) Subscribe(_ context.Context, gvr schema.GroupVersionResource, namespace string,
	handler func(context.Context, Event), opts ...SubscribeOption) (Subscription, error) {
	if handler == nil {
		return nil, errors.New("handler cannot be nil")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failN > 0 {
		f.failN--
		return nil, errors.New("bus unavailable")
	}
	key := fakeKey(gvr, namespace)
	sub := &fakeSubscription{fake: f, key: key, handler: handler, cfg: NewSubscribeConfig(opts...)}
	f.subs[key] = append(f.subs[key], sub)
	return sub, nil
}

// Subscribed reports whether anything is currently subscribed to the resource and namespace.
func (f *Fake) Subscribed(gvr schema.GroupVersionResource, namespace string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs[fakeKey(gvr, namespace)]) > 0
}

// Publish delivers evt to the handlers subscribed to the resource and namespace: every ungrouped subscription, and
// one member of each group, taken in turn. It returns ErrNoSubscription if there are none.
func (f *Fake) Publish(ctx context.Context, gvr schema.GroupVersionResource, namespace string, evt Event) error {
	key := fakeKey(gvr, namespace)
	subs := f.subscriptions(key)
	if len(subs) == 0 {
		return fmt.Errorf("%w for %s in namespace %q", ErrNoSubscription, gvr.String(), namespace)
	}
	for _, sub := range f.recipients(key, subs) {
		sub.handler(ctx, evt)
	}
	return nil
}

// Reconnect invokes the OnReconnect callback of every subscription to the resource and namespace, as a bus does when
// it re-establishes a subscription after a gap.
func (f *Fake) Reconnect(gvr schema.GroupVersionResource, namespace string) {
	for _, sub := range f.subscriptions(fakeKey(gvr, namespace)) {
		if sub.cfg.OnReconnect != nil {
			sub.cfg.OnReconnect()
		}
	}
}

// Group returns the group of the first subscription to the resource and namespace, or the zero Group if there is
// none.
func (f *Fake) Group(gvr schema.GroupVersionResource, namespace string) Group {
	subs := f.subscriptions(fakeKey(gvr, namespace))
	if len(subs) == 0 {
		return Group{}
	}
	return subs[0].cfg.Group
}

// recipients picks who receives one event: every ungrouped subscription, plus one member of each group, taken in
// turn. A group means delivery is shared, so handing the event to all of its members would be the thing a group
// exists to avoid.
func (f *Fake) recipients(key string, subs []*fakeSubscription) []*fakeSubscription {
	out := make([]*fakeSubscription, 0, len(subs))
	groups := map[string][]*fakeSubscription{}
	for _, sub := range subs {
		if sub.cfg.Group.Name == "" {
			out = append(out, sub)
			continue
		}
		groups[sub.cfg.Group.Name] = append(groups[sub.cfg.Group.Name], sub)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	for name, members := range groups {
		cursor := key + "/" + name
		out = append(out, members[f.next[cursor]%len(members)])
		f.next[cursor]++
	}
	return out
}

// subscriptions returns a copy of the subscription list, so that a handler which subscribes or closes during delivery
// does not mutate the slice being ranged over.
func (f *Fake) subscriptions(key string) []*fakeSubscription {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*fakeSubscription(nil), f.subs[key]...)
}

type fakeSubscription struct {
	fake    *Fake
	key     string
	handler func(context.Context, Event)
	cfg     SubscribeConfig
}

var _ Subscription = (*fakeSubscription)(nil)

func (*fakeSubscription) WaitReady(context.Context) error { return nil }

func (s *fakeSubscription) Close() error {
	s.fake.mu.Lock()
	defer s.fake.mu.Unlock()
	remaining := make([]*fakeSubscription, 0, len(s.fake.subs[s.key]))
	for _, other := range s.fake.subs[s.key] {
		if other != s {
			remaining = append(remaining, other)
		}
	}
	s.fake.subs[s.key] = remaining
	return nil
}

func fakeKey(gvr schema.GroupVersionResource, namespace string) string {
	return gvr.String() + "/" + namespace
}
