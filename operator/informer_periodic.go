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
	// DefaultPeriodicSweepInterval is the cadence a PeriodicInformer sweeps at when none is configured.
	DefaultPeriodicSweepInterval = 5 * time.Minute
	// DefaultPeriodicSweepJitterFactor spreads the cadence by up to this fraction, so that informers which started
	// together do not list together for the rest of the process's life.
	DefaultPeriodicSweepJitterFactor = 0.1
	// DefaultPeriodicRetryInterval is how often a PeriodicInformer retries a failing initial list.
	DefaultPeriodicRetryInterval = 5 * time.Second
)

// PeriodicInformerOptions are the options for a PeriodicInformer.
//
// Experimental: this API may change in a minor release.
type PeriodicInformerOptions struct {
	// SweepInterval is how often the full set is re-listed, which for this informer is how often its handlers run
	// at all. A kind states the cadence it needs in WatchOptions.Sweep.Every and simple.App passes it here, so two
	// kinds served by this mechanism can sweep at different rates. A non-positive value uses
	// DefaultPeriodicSweepInterval.
	SweepInterval time.Duration
	// RetryInterval is how often the initial list is retried while it fails.
	// A non-positive value uses DefaultPeriodicRetryInterval.
	RetryInterval time.Duration
	// ErrorHandler is called for errors which do not stop the informer, such as a watcher returning an error.
	ErrorHandler func(context.Context, error)
}

// PeriodicInformer is a re-list-only informer: it has no watch and no subscription, and keeps no cache. On each
// pass it lists the kind and delivers every listed object to its watchers as an add.
//
// It is a sweep and nothing else: its deliveries carry whole objects read from the API and reach every instance,
// but no individual change is ever announced. It serves kinds whose handlers re-evaluate every object on each pass
// regardless of whether it changed — age-based cleanup, for instance, where what makes an object actionable is the
// passage of time. A kind which also acts on changes as they happen needs a mechanism with a live channel too.
//
// Experimental: this API may change in a minor release.
type PeriodicInformer struct {
	kind   resource.Kind
	lister ResourceLister
	opts   PeriodicInformerOptions

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

// NewPeriodicInformer creates a PeriodicInformer for one kind, reading the set to deliver from lister.
//
// Experimental: this API may change in a minor release.
func NewPeriodicInformer(kind resource.Kind, lister ResourceLister, opts PeriodicInformerOptions) (*PeriodicInformer, error) {
	if kind.Schema == nil {
		return nil, errors.New("kind cannot be empty")
	}
	if lister == nil {
		return nil, errors.New("lister cannot be nil")
	}
	if opts.SweepInterval <= 0 {
		opts.SweepInterval = DefaultPeriodicSweepInterval
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

// WatchOptions reports what this informer gives a kind's handlers: a sweep carrying whole objects to every
// instance, and no live channel at all. The cadence and page size are not reported, since a mechanism takes those
// rather than offering them.
func (*PeriodicInformer) WatchOptions() WatchOptions {
	return WatchOptions{
		Sweep: &WatchSweepOptions{
			Detail:       WatchDetailFullObject,
			Distribution: WatchDistributionAll,
		},
	}
}

// AddEventHandler adds a ResourceWatcher as an event handler. Only Add is ever called: the informer keeps no
// snapshot to diff, so it cannot tell an update or a delete from a re-delivery. Add every watcher before Run.
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

// HealthCheck reports an error until the initial list has completed, and once the informer has stopped.
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

// Run performs the initial list, then sweeps every interval until ctx is done. It blocks.
func (p *PeriodicInformer) Run(ctx context.Context) error {
	p.stopped.Store(false)
	defer p.stopped.Store(true)

	// Retry the initial pass so that a transient API error at startup does not defer the first one by a whole
	// interval.
	for {
		if err := p.sweep(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(p.opts.RetryInterval):
		}
	}
	p.synced.Store(true)
	close(p.syncedCh)

	timer := time.NewTimer(wait.Jitter(p.opts.SweepInterval, DefaultPeriodicSweepJitterFactor))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			timer.Reset(wait.Jitter(p.opts.SweepInterval, DefaultPeriodicSweepJitterFactor))
			_ = p.sweep(ctx)
		}
	}
}

// sweep lists the kind and delivers every object as an add. Re-delivering unchanged objects each pass is the point:
// it is what re-triggers cadence-driven work.
func (p *PeriodicInformer) sweep(ctx context.Context) error {
	objs, _, err := p.lister.List(ctx)
	if err != nil {
		p.handleError(ctx, fmt.Errorf("sweep failed to list: %w", err))
		return err
	}

	p.mu.Lock()
	handlers := p.handlers
	p.mu.Unlock()
	for _, obj := range objs {
		for _, handler := range handlers {
			if err := handler.Add(ctx, obj); err != nil {
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
