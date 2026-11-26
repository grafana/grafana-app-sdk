package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/grafana/grafana-app-sdk/resource"
)

const (
	defaultPeriodicInterval      = 5 * time.Minute
	DefaultPeriodicRetryInterval = 5 * time.Second
	informerRetryJitter          = 0.1
)

// PeriodicListOptions sets defaults for scheduled LIST delivery. A kind's
// resync settings and the app's informer settings take precedence.
type PeriodicListOptions struct {
	ResyncInterval time.Duration
	PageSize       int64
	RetryInterval  time.Duration
}

// NewPeriodicList offers scheduled delivery of full objects from a paginated LIST.
func NewPeriodicList(defaults PeriodicListOptions) InformerMechanism {
	return NewInformerMechanism(periodicWatchOptions(WatchDetailFullObject), func(
		kind resource.Kind, clients resource.ClientGenerator, opts InformerOptions,
	) (Informer, error) {
		lister, err := clientListerFor(kind, clients, listOptionsFor(opts, defaults.PageSize))
		if err != nil {
			return nil, err
		}
		return NewPeriodicInformer(kind, lister, periodicOptions(opts, defaults))
	})
}

func periodicWatchOptions(detail WatchDetail) WatchOptions {
	return WatchOptions{Resync: &WatchResyncOptions{Detail: detail, Distribution: WatchDistributionAll}}
}

func periodicOptions(opts InformerOptions, defaults PeriodicListOptions) PeriodicInformerOptions {
	return PeriodicInformerOptions{ResyncInterval: resyncIntervalOr(opts, defaults.ResyncInterval), RetryInterval: defaults.RetryInterval,
		EventTimeout: opts.EventTimeout, ErrorHandler: opts.ErrorHandler}
}

func resyncIntervalOr(opts InformerOptions, fallback time.Duration) time.Duration {
	if resync := opts.ResyncOptions(); resync != nil && resync.Interval > 0 {
		return resync.Interval
	}
	return fallback
}

// PeriodicInformerOptions configures a scheduled LIST. EventTimeout bounds each
// handler's context. A handler must honor cancellation to stop early.
type PeriodicInformerOptions struct {
	ResyncInterval time.Duration
	RetryInterval  time.Duration
	EventTimeout   time.Duration
	ErrorHandler   func(context.Context, error)
}

// PeriodicInformer performs one complete LIST at a time and dispatches every
// object through Add. Objects removed between sweeps generate no delete event.
type PeriodicInformer struct {
	kind   resource.Kind
	lister ResourceLister
	opts   PeriodicInformerOptions
	detail WatchDetail

	mu       sync.Mutex
	handlers []ResourceWatcher
	started  bool
	synced   chan struct{}
	stopped  chan struct{}
}

// NewPeriodicInformer creates an informer whose lister returns full objects.
func NewPeriodicInformer(kind resource.Kind, lister ResourceLister, opts PeriodicInformerOptions) (*PeriodicInformer, error) {
	return newPeriodicInformer(kind, lister, opts, WatchDetailFullObject)
}

func newPeriodicInformer(kind resource.Kind, lister ResourceLister, opts PeriodicInformerOptions, detail WatchDetail) (*PeriodicInformer, error) {
	if kind.Schema == nil || lister == nil {
		return nil, errors.New("periodic informer requires a kind and lister")
	}
	if opts.ResyncInterval <= 0 {
		opts.ResyncInterval = defaultPeriodicInterval
	}
	if opts.RetryInterval <= 0 {
		opts.RetryInterval = DefaultPeriodicRetryInterval
	}
	if opts.ErrorHandler == nil {
		opts.ErrorHandler = DefaultErrorHandler
	}
	return &PeriodicInformer{kind: kind, lister: lister, opts: opts, detail: detail,
		synced: make(chan struct{}), stopped: make(chan struct{})}, nil
}

func (p *PeriodicInformer) WatchOptions() WatchOptions { return periodicWatchOptions(p.detail) }
func (p *PeriodicInformer) Schema() resource.Schema    { return p.kind.Schema }

func (p *PeriodicInformer) AddEventHandler(handler ResourceWatcher) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || handler == nil {
		return errors.New("register a non-nil periodic informer handler before Run")
	}
	p.handlers = append(p.handlers, handler)
	return nil
}

func (p *PeriodicInformer) WaitForSync(ctx context.Context) error {
	if p.HasSynced() {
		return nil
	}
	select {
	case <-p.synced:
		return nil
	case <-p.stopped:
		return errors.New("periodic informer stopped before syncing")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *PeriodicInformer) HasSynced() bool {
	select {
	case <-p.synced:
		return true
	default:
		return false
	}
}

func (p *PeriodicInformer) HealthCheckName() string {
	return "periodic-informer-" + p.kind.GroupVersionKind().String()
}

func (p *PeriodicInformer) HealthCheck(context.Context) error {
	select {
	case <-p.stopped:
		return errors.New("periodic informer has stopped")
	default:
		if !p.HasSynced() {
			return errors.New("periodic informer has not synced")
		}
		return nil
	}
}

func (p *PeriodicInformer) Run(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return errors.New("periodic informer can only run once")
	}
	p.started = true
	p.mu.Unlock()
	defer close(p.stopped)

	for ctx.Err() == nil {
		delay := p.opts.ResyncInterval
		if err := p.sweep(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			p.opts.ErrorHandler(ctx, err)
			delay = p.opts.RetryInterval
		} else if !p.HasSynced() {
			close(p.synced)
		}
		if !waitInformerInterval(ctx, delay) {
			break
		}
	}
	return nil
}

func (p *PeriodicInformer) sweep(ctx context.Context) error {
	objects, _, err := p.lister.List(ctx)
	if err != nil {
		return fmt.Errorf("periodic LIST: %w", err)
	}
	for _, obj := range objects {
		for _, handler := range p.handlers {
			if err := ctx.Err(); err != nil {
				return err
			}
			delivered := obj
			if len(p.handlers) > 1 {
				delivered = obj.Copy()
			}
			err := withInformerEventTimeout(ctx, p.opts.EventTimeout, handler, func(eventCtx context.Context) error {
				return handler.Add(eventCtx, delivered)
			})
			if err != nil {
				p.opts.ErrorHandler(ctx, err)
			}
		}
	}
	return ctx.Err()
}

func waitInformerInterval(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(wait.Jitter(delay, informerRetryJitter))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
