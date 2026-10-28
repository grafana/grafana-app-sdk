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

// PeriodicInformerOptions are the options for a PeriodicInformer.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type PeriodicInformerOptions struct {
	// ResyncInterval is how often the full set is re-listed.
	// A non-positive value uses DefaultRelistInterval.
	ResyncInterval time.Duration
	// RetryInterval is how often the initial list is retried while it fails.
	// A non-positive value uses DefaultRelistRetryInterval.
	RetryInterval time.Duration
	// ErrorHandler is called for errors which do not stop the informer, such as a watcher returning an error.
	ErrorHandler func(context.Context, error)
}

// PeriodicInformer is a re-list-only informer: it has no watch and no subscription, and keeps no cache. On each
// pass it lists the kind and delivers every listed object to its watchers as an add.
//
// It suits idempotent, cadence-driven work which re-evaluates every object on each pass regardless of whether it
// changed — age-based cleanup, for instance — where a live delta stream buys nothing. For anything which should
// react to changes promptly, use a RelistInformer with a ChangeSource, or a watch-backed informer, instead.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
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

var _ Informer = (*PeriodicInformer)(nil)

// NewPeriodicInformer creates a PeriodicInformer for one kind, reading the set to deliver from lister.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
func NewPeriodicInformer(kind resource.Kind, lister ResourceLister, opts PeriodicInformerOptions) (*PeriodicInformer, error) {
	if kind.Schema == nil {
		return nil, errors.New("kind cannot be empty")
	}
	if lister == nil {
		return nil, errors.New("lister cannot be nil")
	}
	if opts.ResyncInterval <= 0 {
		opts.ResyncInterval = DefaultRelistInterval
	}
	if opts.RetryInterval <= 0 {
		opts.RetryInterval = DefaultRelistRetryInterval
	}
	return &PeriodicInformer{
		kind:     kind,
		lister:   lister,
		opts:     opts,
		syncedCh: make(chan struct{}),
	}, nil
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

// Run performs the initial list, then re-lists every interval until ctx is done. It blocks.
func (p *PeriodicInformer) Run(ctx context.Context) error {
	p.stopped.Store(false)
	defer p.stopped.Store(true)

	// Retry the initial pass so that a transient API error at startup does not defer the first one by a whole
	// interval.
	for {
		if err := p.relist(ctx); err == nil {
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

	timer := time.NewTimer(wait.Jitter(p.opts.ResyncInterval, DefaultRelistJitterFactor))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			timer.Reset(wait.Jitter(p.opts.ResyncInterval, DefaultRelistJitterFactor))
			_ = p.relist(ctx)
		}
	}
}

// relist lists the kind and delivers every object as an add. Re-delivering unchanged objects each pass is the point:
// it is what re-triggers cadence-driven work.
func (p *PeriodicInformer) relist(ctx context.Context) error {
	objs, _, err := p.lister.List(ctx)
	if err != nil {
		p.handleError(ctx, fmt.Errorf("re-list failed: %w", err))
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
