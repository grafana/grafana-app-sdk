package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

const (
	busQueueSize     = 1000
	busRetryInterval = 5 * time.Second
	busReadyTimeout  = 30 * time.Second
)

// NewNotificationMechanism serves plain handlers that use bus events as wake-ups
// and can tolerate missed notifications. A nil subscriber disables the mechanism.
func NewNotificationMechanism(subscriber resourcebus.Subscriber, group resourcebus.Group) InformerMechanism {
	if subscriber == nil {
		return nil
	}
	return newBusMechanism(busWatchOptions(group), func(
		kind resource.Kind, _ resource.ClientGenerator, opts InformerOptions,
	) (Informer, error) {
		return newBusInformer(kind, subscriber, group, opts)
	})
}

type unfilteredBusMechanism struct{ InformerMechanism }

func newBusMechanism(provides WatchOptions, supplier InformerSupplier) InformerMechanism {
	return unfilteredBusMechanism{NewInformerMechanism(provides, supplier)}
}

func (unfilteredBusMechanism) supportsQuery(query ListWatchOptions) bool {
	return len(query.LabelFilters) == 0 && len(query.FieldSelectors) == 0
}

func busWatchOptions(group resourcebus.Group) WatchOptions {
	distribution := WatchDistributionAll
	if group.Name != "" {
		distribution = WatchDistributionShared
	}
	return WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity, Distribution: distribution}}
}

type busInformer struct {
	kind        resource.Kind
	subscriber  resourcebus.Subscriber
	group       resourcebus.Group
	namespace   string
	timeout     time.Duration
	reportError func(context.Context, error)
	events      chan resourcebus.Event

	mu       sync.Mutex
	started  bool
	handlers []ResourceWatcher
	synced   chan struct{}
	stopped  chan struct{}
	runErr   error
}

func newBusInformer(kind resource.Kind, subscriber resourcebus.Subscriber, group resourcebus.Group, opts InformerOptions) (*busInformer, error) {
	if subscriber == nil || kind.Schema == nil {
		return nil, errors.New("bus informer requires a kind and subscriber")
	}
	query := opts.ListWatchOptions
	if !(unfilteredBusMechanism{}).supportsQuery(query) {
		return nil, errors.New("bus informer does not support selectors; use the API watch")
	}
	if group.Name == "" && group.Delivery != resourcebus.GroupDeliveryDefault {
		return nil, errors.New("bus delivery policy requires a group name")
	}
	report := opts.ErrorHandler
	if report == nil {
		report = DefaultErrorHandler
	}
	return &busInformer{kind: kind, subscriber: subscriber, group: group, namespace: query.Namespace,
		timeout: opts.EventTimeout, reportError: report, events: make(chan resourcebus.Event, busQueueSize),
		synced: make(chan struct{}), stopped: make(chan struct{})}, nil
}

func (b *busInformer) WatchOptions() WatchOptions { return busWatchOptions(b.group) }

func (b *busInformer) AddEventHandler(handler ResourceWatcher) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started || handler == nil {
		return errors.New("register a non-nil bus informer handler before Run")
	}
	b.handlers = append(b.handlers, handler)
	return nil
}

func (b *busInformer) WaitForSync(ctx context.Context) error {
	select {
	case <-b.synced:
		return nil
	case <-b.stopped:
		select {
		case <-b.synced:
			return nil
		default:
		}
		if b.runErr != nil {
			return b.runErr
		}
		return errors.New("bus informer stopped before syncing")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *busInformer) Run(ctx context.Context) error {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return errors.New("bus informer can only run once")
	}
	b.started = true
	b.mu.Unlock()

	b.runErr = b.run(ctx)
	close(b.stopped)
	return b.runErr
}

func (b *busInformer) run(ctx context.Context) error {
	sub, err := b.subscribe(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer b.closeSubscription(ctx, sub)
	close(b.synced)

	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return nil
		case event := <-b.events:
			b.handleNotification(ctx, event)
		}
	}
	return nil
}

func (b *busInformer) subscribe(ctx context.Context) (resourcebus.Subscription, error) {
	for {
		sub, err := b.openSubscription(ctx)
		if err == nil || ctx.Err() != nil || subscriptionRejected(err) {
			return sub, err
		}
		b.reportError(ctx, err)
		timer := time.NewTimer(wait.Jitter(busRetryInterval, 0.1))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (b *busInformer) openSubscription(ctx context.Context) (resourcebus.Subscription, error) {
	sub, err := b.subscriber.Subscribe(ctx, b.kind.GroupVersionResource(), b.namespace,
		b.enqueue, resourcebus.WithGroup(b.group))
	if err != nil {
		return nil, fmt.Errorf("subscribe to %s: %w", b.kind.GroupVersionResource(), err)
	}
	if sub == nil {
		return nil, fmt.Errorf("%w: subscriber returned no subscription", resourcebus.ErrSubscriptionRejected)
	}
	readyCtx, cancel := context.WithTimeout(ctx, busReadyTimeout)
	defer cancel()
	if err := sub.WaitReady(readyCtx); err != nil {
		b.closeSubscription(ctx, sub)
		return nil, fmt.Errorf("wait for subscription readiness: %w", err)
	}
	return sub, nil
}

func (b *busInformer) closeSubscription(ctx context.Context, sub resourcebus.Subscription) {
	if err := sub.Close(); err != nil {
		b.reportError(ctx, fmt.Errorf("close bus subscription: %w", err))
	}
}

func subscriptionRejected(err error) bool {
	return errors.Is(err, resourcebus.ErrSubscriptionRejected) ||
		apierrors.IsUnauthorized(err) || apierrors.IsForbidden(err)
}

func (b *busInformer) enqueue(ctx context.Context, event resourcebus.Event) {
	if ctx.Err() != nil {
		return
	}
	select {
	case b.events <- event:
	default:
		// A notification-only consumer explicitly accepts missed events.
	}
}

func (b *busInformer) handleNotification(ctx context.Context, event resourcebus.Event) {
	if event.Name == "" || (b.namespace != "" && b.namespace != event.Namespace) {
		return
	}
	obj := b.identity(event)
	switch event.Verb {
	case resourcebus.VerbAdded, resourcebus.VerbModified:
		b.dispatch(ctx, obj, func(ctx context.Context, w ResourceWatcher, target resource.Object) error {
			return w.Add(ctx, target)
		})
	case resourcebus.VerbDeleted:
		b.dispatch(ctx, obj, func(ctx context.Context, w ResourceWatcher, target resource.Object) error {
			return w.Delete(ctx, target)
		})
	default:
		b.reportError(ctx, fmt.Errorf("unknown bus verb %q", event.Verb))
	}
}

func (b *busInformer) identity(event resourcebus.Event) resource.Object {
	obj := b.kind.ZeroValue()
	obj.SetGroupVersionKind(b.kind.GroupVersionKind())
	obj.SetNamespace(event.Namespace)
	obj.SetName(event.Name)
	obj.SetResourceVersion(event.ResourceVersion)
	return obj
}

func (b *busInformer) dispatch(ctx context.Context, obj resource.Object, fn func(context.Context, ResourceWatcher, resource.Object) error) {
	for _, handler := range b.handlers {
		if ctx.Err() != nil {
			return
		}
		target := obj
		if len(b.handlers) > 1 {
			target = obj.Copy()
		}
		err := withInformerEventTimeout(ctx, b.timeout, func(eventCtx context.Context) error {
			return fn(eventCtx, handler, target)
		})
		if err != nil {
			b.reportError(ctx, err)
		}
	}
}
