package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// BusPayload selects which object a BusWatchClient delivers for an event.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusPayload string

const (
	// PayloadIdentity delivers the kind's zero value with namespace, name and resource version set. It suits a
	// reconciler which treats an event as a signal and reads the object itself, and costs no API request per event.
	// Nothing else on the object is populated, so a watcher which reads the spec, the deletion timestamp or the
	// finalizer list must not use it; see the opinionated-watcher constraint in the docs.
	PayloadIdentity BusPayload = "identity"
	// PayloadHydrated reads the object from the API before delivering it, so the informer receives a full object as
	// it would from a kubernetes watch, at the cost of one request per event.
	PayloadHydrated BusPayload = "hydrated"
)

// DefaultBusWatchBufferSize is how many events a BusWatchClient's watch holds for its consumer when none is
// configured.
const DefaultBusWatchBufferSize = 1024

// BusWatchOptions are the options for a BusWatchClient.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusWatchOptions struct {
	// Payload selects which object an event delivers. An empty value is PayloadIdentity.
	Payload BusPayload
	// Group joins this app's replicas into a group across which the bus shares delivery, so that each event reaches
	// one replica rather than all of them. A zero Group delivers every event to every replica. A group only suits a
	// consumer which re-lists to cover the events delivered elsewhere, such as a RelistInformer.
	Group resourcebus.Group
	// BufferSize is how many events a watch holds for its consumer. The bus must not be blocked, so an event which
	// arrives to a full buffer ends the watch instead, which a RelistInformer recovers from with a re-list.
	// A non-positive value uses DefaultBusWatchBufferSize.
	BufferSize int
	// ErrorHandler is called for events which are dropped, such as one with an unknown verb or one whose object could
	// not be read for PayloadHydrated.
	ErrorHandler func(context.Context, error)
}

// BusObjectGetter reads a single object, for PayloadHydrated. resource.Client satisfies it.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusObjectGetter interface {
	Get(ctx context.Context, identifier resource.Identifier) (resource.Object, error)
}

// BusWatchClient serves a resource.Client-shaped Watch from a resource change bus rather than the API server, so that
// it and a resource.Client are interchangeable as a RelistInformer's ChangeSource.
//
// Its watch has two properties a kubernetes watch does not:
//   - It starts from "now". The bus cannot replay, so a watch which asks to resume from a resource version or to
//     receive initial events is refused.
//   - It does not filter. A bus event carries identity only, so label and field selectors are left to the re-list.
//
// Both suit a RelistInformer, which watches from "now" and re-lists through its own lister, but would leave a
// reflector-based informer re-listing in a loop with no live events. So it implements Watch and nothing else of
// resource.Client on purpose: lacking ListInto it does not satisfy ListWatchClient, and handing it to a
// reflector-based informer is a compile error rather than a silent failure.
//
// A watch ends — its channel closes — when the bus reports a reconnect or the consumer falls a full buffer behind.
// Either way events were lost, and ending the watch is how the consumer is told to recover them.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type BusWatchClient struct {
	getter     BusObjectGetter
	kind       resource.Kind
	gvr        schema.GroupVersionResource
	subscriber resourcebus.Subscriber
	opts       BusWatchOptions
}

var _ ChangeSource = (*BusWatchClient)(nil)

// NewBusWatchClient returns a BusWatchClient whose watch is served by subscriber. getter serves the reads
// PayloadHydrated makes; it is required in that mode and ignored otherwise, and the kind's resource.Client satisfies
// it.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewBusWatchClient(getter BusObjectGetter, subscriber resourcebus.Subscriber, kind resource.Kind, opts BusWatchOptions) (*BusWatchClient, error) {
	if subscriber == nil {
		return nil, errors.New("subscriber cannot be nil")
	}
	if kind.Schema == nil {
		return nil, errors.New("kind cannot be empty")
	}
	if opts.Payload == "" {
		opts.Payload = PayloadIdentity
	}
	if opts.Payload != PayloadIdentity && opts.Payload != PayloadHydrated {
		return nil, fmt.Errorf("unknown payload '%s'", opts.Payload)
	}
	if opts.Payload == PayloadHydrated && getter == nil {
		return nil, errors.New("getter is required for the hydrated payload")
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = DefaultBusWatchBufferSize
	}
	return &BusWatchClient{
		getter:     getter,
		kind:       kind,
		gvr:        kind.GroupVersionResource(),
		subscriber: subscriber,
		opts:       opts,
	}, nil
}

// IdentityPayload reports whether the client's watch delivers identity-only objects.
func (c *BusWatchClient) IdentityPayload() bool { return c.opts.Payload == PayloadIdentity }

// Watch subscribes to the bus for the namespace (an empty namespace is every namespace) and delivers its events
// until the watch is stopped, ctx is done, or the watch ends because events were lost.
func (c *BusWatchClient) Watch(ctx context.Context, namespace string, options resource.WatchOptions) (resource.WatchResponse, error) {
	if options.ResourceVersion != "" && options.ResourceVersion != "0" {
		return nil, errors.New("a bus watch cannot resume from a resource version")
	}
	if options.SendInitialEvents != nil && *options.SendInitialEvents {
		return nil, errors.New("a bus watch cannot send initial events")
	}

	ctx, cancel := context.WithCancel(ctx)
	w := &busWatch{
		client: c,
		raw:    make(chan resourcebus.Event, c.opts.BufferSize),
		out:    make(chan resource.WatchEvent),
		lost:   make(chan struct{}),
		cancel: cancel,
	}
	subOpts := []resourcebus.SubscribeOption{resourcebus.WithOnReconnect(w.markLost)}
	if c.opts.Group.Name != "" {
		subOpts = append(subOpts, resourcebus.WithGroup(c.opts.Group))
	}
	sub, err := c.subscriber.Subscribe(ctx, c.gvr, namespace, w.enqueue, subOpts...)
	if err != nil {
		cancel()
		return nil, err
	}
	w.sub = sub
	go w.run(ctx)
	return w, nil
}

// busWatch is one subscription presented as a resource.WatchResponse. The bus handler only buffers, because it must
// not block; a goroutine drains the buffer, reads the object where the payload calls for it, and hands each event to
// the consumer.
type busWatch struct {
	client *BusWatchClient
	sub    resourcebus.Subscription
	raw    chan resourcebus.Event
	out    chan resource.WatchEvent
	cancel context.CancelFunc

	lostOnce sync.Once
	lost     chan struct{}
}

var _ resource.WatchResponse = (*busWatch)(nil)

func (w *busWatch) WatchEvents() <-chan resource.WatchEvent { return w.out }

// Stop ends the watch. The event channel closes once the delivery goroutine has exited.
func (w *busWatch) Stop() { w.cancel() }

// enqueue is the bus handler. It never blocks: an event which finds the buffer full is lost, and so ends the watch.
func (w *busWatch) enqueue(_ context.Context, evt resourcebus.Event) {
	select {
	case w.raw <- evt:
	default:
		w.markLost()
	}
}

// markLost ends the watch because events were not delivered. It is also the reconnect callback, so it must not
// block.
func (w *busWatch) markLost() {
	w.lostOnce.Do(func() { close(w.lost) })
}

func (w *busWatch) run(ctx context.Context) {
	defer close(w.out)
	defer func() {
		if err := w.sub.Close(); err != nil {
			logging.FromContext(ctx).Debug("failed to close subscription", "error", err)
		}
	}()
	defer w.cancel()
	// Once events are lost nothing further is delivered, even what is already buffered: the consumer recovers the
	// lot with a re-list, so delivering more would only delay it.
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.lost:
			w.logLost(ctx)
			return
		case evt := <-w.raw:
			out, ok := w.client.toWatchEvent(ctx, evt)
			if !ok {
				continue
			}
			select {
			case w.out <- out:
			case <-ctx.Done():
				return
			case <-w.lost:
				w.logLost(ctx)
				return
			}
		}
	}
}

func (w *busWatch) logLost(ctx context.Context) {
	logging.FromContext(ctx).Debug("bus watch lost events, ending it", "kind", w.client.kind.GroupVersionKind().String())
}

// toWatchEvent translates one bus event, reporting false for an event which is dropped. A hydrated event whose
// object cannot be read is dropped rather than delivered: a delete arrives as its own event, and anything else is
// recovered by the consumer's next re-list.
func (c *BusWatchClient) toWatchEvent(ctx context.Context, evt resourcebus.Event) (resource.WatchEvent, bool) {
	var eventType string
	switch evt.Verb {
	case resourcebus.VerbAdded:
		eventType = watchEventAdded
	case resourcebus.VerbModified:
		eventType = watchEventModified
	case resourcebus.VerbDeleted:
		// The object is already gone from storage, so there is nothing to read.
		return resource.WatchEvent{EventType: watchEventDeleted, Object: c.identityObject(evt)}, true
	default:
		c.handleError(ctx, fmt.Errorf("dropping event with unknown verb '%s' for %s/%s", evt.Verb, evt.Namespace, evt.Name))
		return resource.WatchEvent{}, false
	}
	if c.opts.Payload == PayloadIdentity {
		return resource.WatchEvent{EventType: eventType, Object: c.identityObject(evt)}, true
	}
	obj, err := c.getter.Get(ctx, resource.Identifier{Namespace: evt.Namespace, Name: evt.Name})
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = errors.New("object no longer exists")
		}
		c.handleError(ctx, fmt.Errorf("dropping event for %s/%s: %w", evt.Namespace, evt.Name, err))
		return resource.WatchEvent{}, false
	}
	return resource.WatchEvent{EventType: eventType, Object: obj}, true
}

// identityObject is the kind's zero value carrying the event's identity, which is all a consumer needs to read the
// object for itself.
func (c *BusWatchClient) identityObject(evt resourcebus.Event) resource.Object {
	return NewIdentityObject(c.kind, evt.Namespace, evt.Name, evt.ResourceVersion)
}

func (c *BusWatchClient) handleError(ctx context.Context, err error) {
	if c.opts.ErrorHandler != nil {
		c.opts.ErrorHandler(ctx, err)
		return
	}
	logging.FromContext(ctx).Error(err.Error(), "component", "BusWatchClient",
		"kind", c.kind.GroupVersionKind().String(), "error", err)
}
