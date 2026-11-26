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

// NewBusNotifications offers live identities to handlers that can tolerate
// missed notifications. A nil subscriber disables this option in the selector.
func NewBusNotifications(subscriber resourcebus.Subscriber, group resourcebus.Group) InformerMechanism {
	if subscriber == nil {
		return nil
	}
	return newBusMechanism(busWatchOptions(group), func(
		kind resource.Kind, _ resource.ClientGenerator, opts InformerOptions,
	) (Informer, error) {
		return newBusInformer(kind, subscriber, busOptionsFor(group, opts))
	})
}

type unfilteredBusMechanism struct{ InformerMechanism }

func newBusMechanism(provides WatchOptions, supplier InformerSupplier) InformerMechanism {
	return unfilteredBusMechanism{NewInformerMechanism(provides, supplier)}
}

func (unfilteredBusMechanism) supportsQuery(query ListWatchOptions) bool {
	return len(query.LabelFilters) == 0 && len(query.FieldSelectors) == 0
}

func (m unfilteredBusMechanism) NewInformer(kind resource.Kind, clients resource.ClientGenerator, opts InformerOptions) (Informer, error) {
	if !m.supportsQuery(opts.ListWatchOptions) {
		return nil, errors.New("bus informer does not support selectors; use the API watch")
	}
	return m.InformerMechanism.NewInformer(kind, clients, opts)
}

func busWatchOptions(group resourcebus.Group) WatchOptions {
	distribution := WatchDistributionAll
	if group.Name != "" {
		distribution = WatchDistributionShared
	}
	return WatchOptions{Live: &WatchLiveOptions{Detail: WatchDetailIdentity, Distribution: distribution}}
}

// BusInformerOptions configures the subscription and its event handlers.
type BusInformerOptions struct {
	Namespace    string
	Group        resourcebus.Group
	EventTimeout time.Duration
	ErrorHandler func(context.Context, error)
}

func busOptionsFor(group resourcebus.Group, opts InformerOptions) BusInformerOptions {
	return BusInformerOptions{
		Namespace:    opts.ListWatchOptions.Namespace,
		Group:        group,
		EventTimeout: opts.EventTimeout,
		ErrorHandler: opts.ErrorHandler,
	}
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

func newBusInformer(kind resource.Kind, subscriber resourcebus.Subscriber, opts BusInformerOptions) (*busInformer, error) {
	if subscriber == nil || kind.Schema == nil {
		return nil, errors.New("bus informer requires a kind and subscriber")
	}
	if err := validateBusGroup(opts.Group); err != nil {
		return nil, err
	}
	report := opts.ErrorHandler
	if report == nil {
		report = DefaultErrorHandler
	}
	return &busInformer{kind: kind, subscriber: subscriber, group: opts.Group, namespace: opts.Namespace,
		timeout: opts.EventTimeout, reportError: report, events: make(chan resourcebus.Event, busQueueSize),
		synced: make(chan struct{}), stopped: make(chan struct{})}, nil
}

func validateBusGroup(group resourcebus.Group) error {
	if group.Name == "" && group.Delivery != resourcebus.GroupDeliveryDefault {
		return errors.New("bus delivery policy requires a group name")
	}
	if group.Delivery != resourcebus.GroupDeliveryDefault && group.Delivery != resourcebus.GroupDeliveryRoundRobin {
		return fmt.Errorf("%w: unsupported group delivery policy %q", resourcebus.ErrSubscriptionRejected, group.Delivery)
	}
	return nil
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

func (b *busInformer) HealthCheckName() string {
	return "bus-informer-" + b.kind.GroupVersionKind().String()
}

func (b *busInformer) HealthCheck(context.Context) error {
	select {
	case <-b.stopped:
		return errors.New("bus informer has stopped")
	default:
	}
	select {
	case <-b.synced:
		return nil
	default:
		return errors.New("bus subscription is not ready")
	}
}

func (b *busInformer) Run(ctx context.Context) error {
	return b.runOnce(ctx, b.run)
}

func (b *busInformer) runOnce(ctx context.Context, run func(context.Context) error) error {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return errors.New("bus informer can only run once")
	}
	b.started = true
	b.mu.Unlock()

	b.runErr = run(ctx)
	close(b.stopped)
	return b.runErr
}

func (b *busInformer) run(ctx context.Context) error {
	sub, err := b.subscribe(ctx, busSubscriptionHooks{})
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

type busSubscriptionHooks struct {
	onReconnect func()
	onOverflow  func()
}

func (b *busInformer) subscribe(ctx context.Context, hooks busSubscriptionHooks) (resourcebus.Subscription, error) {
	for {
		sub, err := b.openSubscription(ctx, hooks)
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

func (b *busInformer) openSubscription(ctx context.Context, hooks busSubscriptionHooks) (resourcebus.Subscription, error) {
	attemptCtx, cancelAttempt := context.WithCancel(ctx)
	opts := []resourcebus.SubscribeOption{resourcebus.WithGroup(b.group)}
	if hooks.onReconnect != nil {
		opts = append(opts, resourcebus.WithOnReconnect(func() {
			if attemptCtx.Err() == nil {
				hooks.onReconnect()
			}
		}))
	}
	sub, err := b.subscriber.Subscribe(attemptCtx, b.kind.GroupVersionResource(), b.namespace,
		func(eventCtx context.Context, event resourcebus.Event) {
			if attemptCtx.Err() == nil {
				b.enqueue(eventCtx, event, hooks.onOverflow)
			}
		}, opts...)
	if err != nil {
		cancelAttempt()
		if sub != nil {
			b.closeSubscription(ctx, sub)
		}
		return nil, fmt.Errorf("subscribe to %s: %w", b.kind.GroupVersionResource(), err)
	}
	if sub == nil {
		cancelAttempt()
		return nil, fmt.Errorf("%w: subscriber returned no subscription", resourcebus.ErrSubscriptionRejected)
	}
	active := busSubscription{Subscription: sub, cancel: cancelAttempt}
	readyCtx, cancelReady := context.WithTimeout(attemptCtx, busReadyTimeout)
	defer cancelReady()
	if err := sub.WaitReady(readyCtx); err != nil {
		b.closeSubscription(ctx, active)
		return nil, fmt.Errorf("wait for subscription readiness: %w", err)
	}
	return active, nil
}

type busSubscription struct {
	resourcebus.Subscription
	cancel context.CancelFunc
}

func (s busSubscription) Close() error {
	s.cancel()
	return s.Subscription.Close()
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

func (b *busInformer) enqueue(ctx context.Context, event resourcebus.Event, onOverflow func()) {
	if ctx.Err() != nil {
		return
	}
	select {
	case b.events <- event:
	default:
		if onOverflow != nil {
			onOverflow()
		}
	}
}

func (b *busInformer) handleNotification(ctx context.Context, event resourcebus.Event) {
	if !b.acceptEvent(event) {
		return
	}
	ctx = ContextWithAnnouncedVersion(ContextWithDelivery(ctx, DeliveryLive), event.ResourceVersion)
	obj := newIdentityObject(b.kind, event.Namespace, event.Name, event.ResourceVersion)
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

func (b *busInformer) acceptEvent(event resourcebus.Event) bool {
	return event.Name != "" && (b.namespace == "" || b.namespace == event.Namespace)
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
		err := withInformerEventTimeout(ctx, b.timeout, handler, func(eventCtx context.Context) error {
			return fn(eventCtx, handler, target)
		})
		if err != nil {
			b.reportError(ctx, err)
		}
	}
}
