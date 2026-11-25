package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

const (
	// DefaultPeriodicResyncInterval is the cadence used when none is configured.
	DefaultPeriodicResyncInterval = 5 * time.Minute
	// DefaultPeriodicResyncJitterFactor spreads the cadence, so informers started together do not list together.
	DefaultPeriodicResyncJitterFactor = 0.1
	// DefaultPeriodicRetryInterval is how often a failed or deferred pass is retried.
	DefaultPeriodicRetryInterval = 5 * time.Second
)

// errPassDeferred is what resync returns for a pass it did not start, so Run retries it on the short interval.
var errPassDeferred = errors.New("previous pass has not drained")

// PeriodicInformerOptions are the options for a PeriodicInformer.
//
// Experimental: this API may change in a minor release.
type PeriodicInformerOptions struct {
	// ResyncInterval is how often the set is re-listed, which here is how often handlers run at all. A non-positive
	// value uses DefaultPeriodicResyncInterval.
	ResyncInterval time.Duration
	// RetryInterval is how often a failed or deferred pass is retried. A non-positive value uses
	// DefaultPeriodicRetryInterval.
	RetryInterval time.Duration
	// ErrorHandler is called for errors which do not stop the informer, such as a watcher returning one.
	ErrorHandler func(context.Context, error)
}

// PeriodicInformer is a re-list-only informer: no watch, no subscription, no cache. Each pass lists the kind and
// delivers every object as an add, so it serves handlers which re-evaluate everything on a cadence — age-based
// cleanup, say — and announces no individual change. A kind which acts on changes needs a live channel too.
//
// Each pass reads the API, unlike the watch's resync, which replays a cache.
//
// Built by NewPeriodicMechanism, a pass does not start while the per-key workers still hold deliveries from the
// last one: it is retried on RetryInterval until they drain, so handlers slower than the cadence run less often
// rather than accumulating a backlog. A PeriodicInformer built directly dispatches synchronously, so each pass
// already waits for the last.
//
// Re-delivering the whole kind on a cadence asks two things of the handlers it serves:
//
//   - They should let the next pass be the retry rather than returning ReconcileResult.RequeueAfter.
//     InformerController's OpinionatedRetryDequeuePolicy keeps a pending retry when a new add carries the same
//     generation, which every pass of this informer does, so each pass starts another chain beside the last — and a
//     retry held that way can still be holding the snapshot from before a deletion.
//   - An Opinionated variant finalizes only as fast as the cadence, and not at all for an object which stops
//     matching the lister's selectors while holding its finalizer. See docs/writing-a-reconciler.md.
//
// Experimental: this API may change in a minor release.
type PeriodicInformer struct {
	kind   resource.Kind
	lister ResourceLister
	opts   PeriodicInformerOptions
	// backlog reports the deliveries still queued from earlier passes, when handlers run asynchronously.
	backlog func() float64

	mu       sync.Mutex
	handlers []ResourceWatcher
	synced   atomic.Bool
	syncedCh chan struct{}
	stopped  atomic.Bool
}

var (
	_ Informer             = (*PeriodicInformer)(nil)
	_ WatchOptionsInformer = (*PeriodicInformer)(nil)
)

// NewPeriodicInformer creates a PeriodicInformer for one kind, reading the set to deliver from lister, which must
// return whole objects: PeriodicInformerWatchOptions is what the informer reports whatever it is given.
//
// Experimental: this API may change in a minor release.
func NewPeriodicInformer(kind resource.Kind, lister ResourceLister, opts PeriodicInformerOptions) (*PeriodicInformer, error) {
	if kind.Schema == nil {
		return nil, errors.New("kind cannot be empty")
	}
	if lister == nil {
		return nil, errors.New("lister cannot be nil")
	}
	if opts.ResyncInterval <= 0 {
		opts.ResyncInterval = DefaultPeriodicResyncInterval
	}
	if opts.RetryInterval <= 0 {
		opts.RetryInterval = DefaultPeriodicRetryInterval
	}
	return &PeriodicInformer{
		kind:     kind,
		lister:   lister,
		opts:     opts,
		syncedCh: make(chan struct{}),
	}, nil
}

// PeriodicInformerWatchOptions is what every PeriodicInformer provides: a resync carrying whole objects to every
// instance, and no live channel. The cadence and page size are omitted, being requests a mechanism takes rather
// than something it is compared on.
//
// It is a function so that a mechanism building these informers advertises the same thing they report, rather than
// restating it.
//
// Experimental: this API may change in a minor release.
func PeriodicInformerWatchOptions() WatchOptions {
	return WatchOptions{
		Resync: &WatchResyncOptions{
			Detail:       WatchDetailFullObject,
			Distribution: WatchDistributionAll,
		},
	}
}

// WatchOptions implements WatchOptionsInformer.
func (*PeriodicInformer) WatchOptions() WatchOptions { return PeriodicInformerWatchOptions() }

// AddEventHandler adds a ResourceWatcher. Only Add is ever called, since without a snapshot an update and a delete
// are indistinguishable from a re-delivery. Add every watcher before Run.
func (p *PeriodicInformer) AddEventHandler(handler ResourceWatcher) error {
	if handler == nil {
		return errors.New("handler cannot be nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers = append(p.handlers, handler)
	return nil
}

// WaitForSync waits for the initial list to complete, or for the context to be done.
func (p *PeriodicInformer) WaitForSync(ctx context.Context) error {
	select {
	case <-p.syncedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HasSynced reports whether the initial list has completed.
func (p *PeriodicInformer) HasSynced() bool { return p.synced.Load() }

// HealthCheck reports an error until the initial list completes, and once the informer has stopped.
func (p *PeriodicInformer) HealthCheck(context.Context) error {
	if p.stopped.Load() {
		return errors.New("informer is stopped")
	}
	if !p.synced.Load() {
		return errors.New("informer has not synced")
	}
	return nil
}

// HealthCheckName returns the name of the informer's health check.
func (p *PeriodicInformer) HealthCheckName() string {
	return "periodic-informer-" + p.kind.GroupVersionKind().String()
}

// Schema returns the resource.Schema the informer is set up for.
func (p *PeriodicInformer) Schema() resource.Schema { return p.kind.Schema }

// Run lists, then resyncs every interval until ctx is done. It blocks, and must be called once.
func (p *PeriodicInformer) Run(ctx context.Context) error {
	defer p.stopped.Store(true)

	// Retried so a transient error at startup does not defer the first pass by a whole interval.
	for {
		if err := p.resync(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(p.retryInterval()):
		}
	}
	p.synced.Store(true)
	close(p.syncedCh)

	timer := time.NewTimer(wait.Jitter(p.opts.ResyncInterval, DefaultPeriodicResyncJitterFactor))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			// A failed pass is retried on the short interval: waiting out the cadence would skip the work
			// entirely, and for an hourly kind a momentary API error would cost it an hour.
			next := wait.Jitter(p.opts.ResyncInterval, DefaultPeriodicResyncJitterFactor)
			if err := p.resync(ctx); err != nil {
				next = p.retryInterval()
			}
			timer.Reset(next)
		}
	}
}

// retryInterval jitters the retry delay, so replicas which started together do not go on hammering a degraded API
// in lockstep.
func (p *PeriodicInformer) retryInterval() time.Duration {
	return wait.Jitter(p.opts.RetryInterval, DefaultPeriodicResyncJitterFactor)
}

// resync delivers every listed object as an add. Re-delivering unchanged ones is the point: it re-triggers the work.
func (p *PeriodicInformer) resync(ctx context.Context) error {
	if p.backlog != nil {
		if pending := p.backlog(); pending > 0 {
			logging.FromContext(ctx).Warn("deferring resync until the previous pass drains", "component", "PeriodicInformer",
				"kind", p.kind.GroupVersionKind().String(), "pending", pending)
			return errPassDeferred
		}
	}
	objs, _, err := p.lister.List(ctx)
	if err != nil {
		p.handleError(ctx, fmt.Errorf("resync failed to list: %w", err))
		return err
	}

	p.mu.Lock()
	handlers := p.handlers
	p.mu.Unlock()
	for _, obj := range objs {
		// A whole kind is a long dispatch to hold shutdown up for, and every delivery past this point would carry
		// a canceled context to handlers which are stopping anyway.
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, handler := range handlers {
			delivered := obj
			if len(handlers) > 1 {
				// The opinionated handlers write a finalizer update back into the object they were given, so
				// handlers past the first must not be handed what an earlier one changed. Copied only when there is
				// more than one, since the usual case is a single handler and a whole kind per pass.
				delivered = obj.Copy()
			}
			if err := handler.Add(ctx, delivered); err != nil {
				p.handleError(ctx, err)
			}
		}
	}
	return nil
}

func (p *PeriodicInformer) handleError(ctx context.Context, err error) {
	if p.opts.ErrorHandler != nil {
		p.opts.ErrorHandler(ctx, err)
		return
	}
	logging.FromContext(ctx).Error(err.Error(), "component", "PeriodicInformer",
		"kind", p.kind.GroupVersionKind().String(), "error", err)
}

// PeriodicMechanismOptions are the fallbacks a PeriodicInformer mechanism builds with, for whatever a kind's
// WatchNeeds.Resync and the app-wide InformerOptions leave unset.
//
// Experimental: this API may change in a minor release.
type PeriodicMechanismOptions struct {
	// ResyncInterval is the cadence. A non-positive value uses DefaultPeriodicResyncInterval.
	ResyncInterval time.Duration
	// PageSize is the page size the lister requests. An empty value uses DefaultListerPageSize.
	PageSize int64
	// RetryInterval is how often a failed or deferred pass is retried. A non-positive value uses
	// DefaultPeriodicRetryInterval.
	RetryInterval time.Duration
}

// NewPeriodicMechanism returns the re-list, for NewSelectingInformerSupplier to give the kinds needing no live
// channel. It advertises what the informers it builds report, so listing it cannot claim the wrong thing, and each
// kind resyncs at the figures it asked for in WatchNeeds.Resync. Its NewInformer is an InformerSupplier, for an app
// whose every kind re-lists.
//
// InformerOptions.EventTimeout is not applied: handlers run with the informer's run context, as they do under the
// stock informers while those run.
//
// Experimental: this API may change in a minor release.
func NewPeriodicMechanism(opts PeriodicMechanismOptions) InformerMechanism {
	return NewInformerMechanism(PeriodicInformerWatchOptions(), periodicSupplier{opts: opts}.newInformer)
}

type periodicSupplier struct {
	opts PeriodicMechanismOptions
}

func (m periodicSupplier) newInformer(
	kind resource.Kind, clients resource.ClientGenerator, options InformerOptions,
) (Informer, error) {
	if clients == nil {
		return nil, errors.New("a client generator is required to list the kind")
	}
	resync := m.resolved(options)
	client, err := clients.ClientFor(kind)
	if err != nil {
		return nil, err
	}
	lister, err := NewClientLister(client, kind, ClientListerOptions{
		ListWatchOptions: options.ListWatchOptions,
		PageSize:         resync.ListPageSize,
	})
	if err != nil {
		return nil, err
	}

	inf, err := NewPeriodicInformer(kind, lister, PeriodicInformerOptions{
		ResyncInterval: resync.Interval,
		RetryInterval:  m.opts.RetryInterval,
		ErrorHandler:   options.ErrorHandler,
	})
	if err != nil {
		return nil, err
	}
	// Per-key workers, as under the other suppliers: a slow handler on the informer's loop would hold up the pass.
	// Their queue-depth metric is labeled by kind, which is empty unless something sets it.
	if options.ResourceKind == "" {
		options.ResourceKind = kind.GroupVersionKind().String()
	}
	concurrent, err := NewConcurrentInformerFromOptions(inf, options)
	if err != nil {
		return nil, err
	}
	inf.backlog = concurrent.QueueDepth
	// The worker wrapper is no WatchOptionsInformer, so what it wraps would go unread: carry the informer's own
	// answer out, rather than restating it here.
	return InformerWithWatchOptions(concurrent, inf.WatchOptions()), nil
}

// resolved is ResyncOptions with these options answering for whatever neither the kind nor the app set — including
// a resync asked for with no cadence, which resolves to zero.
func (m periodicSupplier) resolved(options InformerOptions) WatchResyncOptions {
	// Seeded from the app-wide page size, which ResyncOptions does not report when neither side asked for a resync.
	resync := WatchResyncOptions{ListPageSize: options.WatchListPageSize}
	if r := options.ResyncOptions(); r != nil {
		resync = *r
	}
	if resync.Interval <= 0 {
		resync.Interval = m.opts.ResyncInterval
	}
	if resync.ListPageSize <= 0 {
		resync.ListPageSize = m.opts.PageSize
	}
	return resync
}
