package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

const (
	// DefaultRelistInterval is the re-list cadence a RelistInformer or PeriodicInformer uses when none is configured.
	DefaultRelistInterval = 5 * time.Minute
	// DefaultRelistJitterFactor spreads the re-list cadence by up to this fraction, so that informers which started
	// together do not re-list in lockstep and stampede the API server.
	DefaultRelistJitterFactor = 0.1
	// DefaultRelistRetryInterval is how often a RelistInformer retries a failing watch or initial list.
	DefaultRelistRetryInterval = 5 * time.Second
)

// Watch event types a ChangeSource delivers, as the kubernetes watch names them.
const (
	watchEventAdded    = "ADDED"
	watchEventModified = "MODIFIED"
	watchEventDeleted  = "DELETED"
	watchEventBookmark = "BOOKMARK"
	watchEventError    = "ERROR"
)

// relistPass distinguishes the initial list from a later one. They differ in one way: an add on a later pass is a
// change the live stream did not deliver here, so its resource version dates the recovery, while the initial list's
// objects may be arbitrarily old.
type relistPass int

const (
	initialRelist relistPass = iota
	periodicRelist
)

// ChangeSource is where a RelistInformer learns about changes between re-lists. It is resource.Client's Watch alone,
// so a resource.Client is a source as it stands — a kubernetes watch — and NewBusWatchClient serves the same Watch from
// a resource change bus; either can be passed where the other is.
//
// The informer always watches from "now" (an empty resource version) and treats the end of a watch — its channel
// closing, or an ERROR event — as a gap: it opens a new watch and re-lists to cover whatever was missed in between.
// A source therefore signals that it lost events by ending the watch.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type ChangeSource interface {
	Watch(ctx context.Context, namespace string, options resource.WatchOptions) (resource.WatchResponse, error)
}

var _ ChangeSource = resource.Client(nil)

// identityPayload is implemented by a ChangeSource or ResourceLister which delivers identity-only objects: the
// kind's zero value with namespace, name and resource version set, and nothing else.
type identityPayload interface {
	IdentityPayload() bool
}

func isIdentityPayload(v any) bool {
	identity, ok := v.(identityPayload)
	return ok && identity.IdentityPayload()
}

// RelistInformerOptions are the options for a RelistInformer.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type RelistInformerOptions struct {
	// ListWatchOptions are the namespace and filters the watch asks the ChangeSource for. They must match what the
	// ResourceLister lists.
	ListWatchOptions ListWatchOptions
	// ResyncInterval is how often the full set is re-listed. A non-positive value uses DefaultRelistInterval.
	ResyncInterval time.Duration
	// AllowDegradedStart lets Run fall back to re-list-only operation when the watch cannot be opened, instead of
	// holding the initial list (and so WaitForSync) until it succeeds. The watch keeps retrying in the background and
	// forces a re-list once it opens. Consumers whose progress must not stall with the change source opt in; the
	// default suits a consumer which prefers not to start against a snapshot with no live updates.
	AllowDegradedStart bool
	// RetryInterval is how often a failing watch or initial list is retried.
	// A non-positive value uses DefaultRelistRetryInterval.
	RetryInterval time.Duration
	// ErrorHandler is called for errors which do not stop the informer, such as a watcher returning an error or a
	// failed re-list.
	ErrorHandler func(context.Context, error)
	// Metrics are the shared delivery metrics. A nil value disables observation.
	Metrics *RelistInformerMetrics
}

// RelistInformer drives watchers from a periodic re-list, and optionally from a ChangeSource between re-lists. It
// keeps no per-object cache that a reader can serve from, only the snapshot the re-list diffs against.
//
// The periodic re-list is what makes the informer correct rather than merely live. A change the source never
// delivered — routed to another replica of a shared bus group, published while the watch was down, or never
// announced at all — is caught by the next re-list, which diffs the listed set against the previous snapshot and
// dispatches the difference: a key seen for the first time as an add, a key seen before as an update, and a key which
// has vanished as a delete carrying its last-known state.
//
// Without a ChangeSource the informer is re-list only, and reacts to changes on the re-list cadence.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type RelistInformer struct {
	kind    resource.Kind
	lister  ResourceLister
	source  ChangeSource
	opts    RelistInformerOptions
	metrics kindMetrics

	// liveIdentity is whether the source delivers identity-only objects, which must not overwrite a stored object.
	liveIdentity bool

	store *relistStore

	// gap signals the run loop to re-list after the watch is re-established, since changes made while it was down
	// were never delivered. Buffered depth 1 with a non-blocking send coalesces bursts into one re-list.
	gap chan struct{}

	mu       sync.Mutex
	handlers []ResourceWatcher
	synced   atomic.Bool
	syncedCh chan struct{}
	stopped  atomic.Bool
}

var _ Informer = (*RelistInformer)(nil)

// NewRelistInformer creates a RelistInformer for one kind. lister reads the full set for the initial list and each
// re-list. source, which may be nil, delivers changes between re-lists, and must cover the same namespace and filters
// the lister lists.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
func NewRelistInformer(kind resource.Kind, lister ResourceLister, source ChangeSource, opts RelistInformerOptions) (*RelistInformer, error) {
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
	return &RelistInformer{
		kind:         kind,
		lister:       lister,
		source:       source,
		opts:         opts,
		metrics:      kindMetrics{metrics: opts.Metrics, kind: kind.GroupVersionKind().String()},
		liveIdentity: isIdentityPayload(source),
		store:        newRelistStore(),
		gap:          make(chan struct{}, 1),
		syncedCh:     make(chan struct{}),
	}, nil
}

// logger returns the informer's logger, tagged with the component and kind.
func (r *RelistInformer) logger(ctx context.Context) logging.Logger {
	return logging.FromContext(ctx).With("component", "RelistInformer", "kind", r.kind.GroupVersionKind().String())
}

// IdentityPayload reports whether the informer may deliver identity-only objects, because its source or its lister
// declares that it produces them.
func (r *RelistInformer) IdentityPayload() bool {
	return r.liveIdentity || isIdentityPayload(r.lister)
}

// AddEventHandler adds a ResourceWatcher as an event handler. There is no cache to replay, so a watcher added after
// Run only sees events from the next live event or re-list; add every watcher before Run.
func (r *RelistInformer) AddEventHandler(handler ResourceWatcher) error {
	if handler == nil {
		return errors.New("handler cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers = append(r.handlers, handler)
	return nil
}

// WaitForSync waits for the initial list to complete, or for the context to be done.
func (r *RelistInformer) WaitForSync(ctx context.Context) error {
	select {
	case <-r.syncedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HasSynced reports whether the initial list has completed.
func (r *RelistInformer) HasSynced() bool { return r.synced.Load() }

// HealthCheck reports an error until the initial list has completed, and once the informer has stopped.
func (r *RelistInformer) HealthCheck(context.Context) error {
	if r.stopped.Load() {
		return errors.New("informer is stopped")
	}
	if !r.synced.Load() {
		return errors.New("informer has not synced")
	}
	return nil
}

// HealthCheckName returns the name of the informer's health check.
func (r *RelistInformer) HealthCheckName() string {
	return "relist-informer-" + r.kind.GroupVersionKind().String()
}

// Schema returns the resource.Schema the informer is set up for.
func (r *RelistInformer) Schema() resource.Schema { return r.kind.Schema }

// PrometheusCollectors implements metrics.Provider. The collectors are shared by every informer given the same
// RelistInformerMetrics, so registering an already-registered collector is expected and must be tolerated.
func (r *RelistInformer) PrometheusCollectors() []prometheus.Collector {
	return r.opts.Metrics.PrometheusCollectors()
}

// Run opens the watch, performs the initial list, then serves live events and a periodic re-list until ctx is done.
// It blocks.
//
// The watch is opened before the initial list, so that a change made while the list runs is delivered rather than
// dropped in a list-to-watch gap. If it cannot be opened the default is to keep retrying and not list: re-listing a
// kind we cannot watch would start the consumer against a snapshot with no live updates until the next re-list.
// RelistInformerOptions.AllowDegradedStart opts into exactly that instead.
func (r *RelistInformer) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	var live sync.WaitGroup
	// Cancel before waiting, so that the watch goroutine is told to stop on every return path.
	defer live.Wait()
	defer cancel()
	r.stopped.Store(false)
	defer r.stopped.Store(true)

	if r.source != nil {
		r.metrics.observeWatchClosed()
		defer r.metrics.observeWatchClosed()
		if !r.startWatch(ctx, &live) {
			return nil
		}
	}

	// Retry the initial list until it succeeds: WaitForSync releases the consumer, so syncing after a failed list
	// would start it against an empty snapshot, leaving every existing object unreconciled until the next re-list.
	for {
		if err := r.relist(ctx, initialRelist); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.opts.RetryInterval):
		}
	}
	r.synced.Store(true)
	close(r.syncedCh)

	// A fresh timer per pass, rather than a ticker, lets the jitter vary every time. Re-arming before the re-list
	// keeps the cadence start-to-start rather than re-list duration plus interval.
	resync := time.NewTimer(wait.Jitter(r.opts.ResyncInterval, DefaultRelistJitterFactor))
	defer resync.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-resync.C:
			resync.Reset(wait.Jitter(r.opts.ResyncInterval, DefaultRelistJitterFactor))
			_ = r.relist(ctx, periodicRelist)
		case <-r.gap:
			r.logger(ctx).Debug("watch re-established, re-listing")
			_ = r.relist(ctx, periodicRelist)
		}
	}
}

// startWatch opens the watch, retrying while it fails, and hands it to a goroutine which serves it. It reports false
// when ctx was done before the informer could start. When starting degraded, the goroutine does the retrying instead
// and startWatch returns at once.
func (r *RelistInformer) startWatch(ctx context.Context, live *sync.WaitGroup) bool {
	w, err := r.openWatch(ctx)
	if err != nil && r.opts.AllowDegradedStart {
		r.logger(ctx).Warn("failed to open watch, starting in re-list-only degraded mode", "error", err)
	}
	for err != nil && !r.opts.AllowDegradedStart {
		r.logger(ctx).Warn("failed to open watch, will retry", "error", err)
		select {
		case <-ctx.Done():
			return false
		case <-time.After(r.opts.RetryInterval):
		}
		w, err = r.openWatch(ctx)
	}
	live.Add(1)
	go func() {
		defer live.Done()
		r.serveWatch(ctx, w)
	}()
	return true
}

// openWatch opens one watch from "now".
func (r *RelistInformer) openWatch(ctx context.Context) (resource.WatchResponse, error) {
	w, err := r.source.Watch(ctx, r.opts.ListWatchOptions.Namespace, resource.WatchOptions{
		LabelFilters:   r.opts.ListWatchOptions.LabelFilters,
		FieldSelectors: r.opts.ListWatchOptions.FieldSelectors,
	})
	if err != nil {
		return nil, err
	}
	r.metrics.observeWatchOpen()
	return w, nil
}

// serveWatch delivers live events until ctx is done, re-opening the watch each time it ends. A nil w means the watch
// has not opened yet (a degraded start), so serving begins by opening one. Every re-open is a gap, so it signals a
// re-list.
func (r *RelistInformer) serveWatch(ctx context.Context, w resource.WatchResponse) {
	for {
		if w == nil {
			w = r.reopenWatch(ctx)
			if w == nil {
				return
			}
			r.signalGap()
		}
		r.consume(ctx, w)
		w.Stop()
		r.metrics.observeWatchClosed()
		if ctx.Err() != nil {
			return
		}
		w = nil
	}
}

// reopenWatch opens a watch, retrying until it succeeds or ctx is done, in which case it returns nil. It waits out the
// retry interval before each attempt, so that a source which ends every watch at once cannot spin the informer; the
// re-list the re-open triggers covers the wait.
func (r *RelistInformer) reopenWatch(ctx context.Context) resource.WatchResponse {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.opts.RetryInterval):
		}
		w, err := r.openWatch(ctx)
		if err == nil {
			r.metrics.observeRewatch()
			r.logger(ctx).Info("watch opened")
			return w
		}
		r.logger(ctx).Warn("failed to open watch, will retry", "error", err)
	}
}

// consume delivers a watch's events until its channel closes, it reports an error, or ctx is done.
func (r *RelistInformer) consume(ctx context.Context, w resource.WatchResponse) {
	events := w.WatchEvents()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				r.logger(ctx).Debug("watch closed")
				return
			}
			if evt.EventType == watchEventError {
				r.logger(ctx).Debug("watch reported an error, re-opening")
				return
			}
			r.onEvent(ctx, evt)
		}
	}
}

// signalGap nudges the run loop to re-list. The send is non-blocking and a pending signal coalesces further gaps
// into the next re-list.
func (r *RelistInformer) signalGap() {
	select {
	case r.gap <- struct{}{}:
	default:
	}
}

// onEvent handles one live event.
func (r *RelistInformer) onEvent(ctx context.Context, evt resource.WatchEvent) {
	obj := evt.Object
	if evt.EventType == watchEventBookmark {
		return
	}
	if obj == nil || obj.GetName() == "" {
		r.handleError(ctx, fmt.Errorf("dropping %s event with no object", evt.EventType))
		return
	}
	rv := parseResourceVersion(obj.GetResourceVersion())
	switch evt.EventType {
	case watchEventDeleted:
		// Dispatch the last-known object if the snapshot still holds one, since the event's may carry identity only.
		// Evicting it keeps the next re-list from diffing a delivered delete as a change the live stream missed.
		if stored, ok := r.store.Get(obj.GetNamespace(), obj.GetName()); ok {
			obj = stored
		}
		r.store.DeleteAt(evt.Object.GetNamespace(), evt.Object.GetName(), evt.Object.GetResourceVersion())
		r.metrics.observeLiveEvent(relistVerbDelete, rv)
		r.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Delete(ctx, obj) })
	case watchEventAdded, watchEventModified:
		// An identity object carries no deletion timestamp, so writing a modification through would overwrite a
		// stored object's real one and mislead the next re-list's diff. Writing an add through is still necessary:
		// without it the object is absent until the next re-list, which would then re-report a delivered add as a
		// change the live stream missed.
		if evt.EventType == watchEventAdded || !r.liveIdentity {
			r.store.Update(obj)
		}
		if evt.EventType == watchEventAdded {
			r.metrics.observeLiveEvent(relistVerbAdd, rv)
			r.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Add(ctx, obj) })
			return
		}
		r.metrics.observeLiveEvent(relistVerbUpdate, rv)
		r.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Update(ctx, obj, obj) })
	default:
		r.handleError(ctx, fmt.Errorf("dropping event with unknown type '%s' for %s/%s",
			evt.EventType, obj.GetNamespace(), obj.GetName()))
	}
}

// relist reads the full set and dispatches the diff against the previous snapshot. On the initial list the snapshot
// is empty, so every object is an add and there is nothing to delete.
func (r *RelistInformer) relist(ctx context.Context, pass relistPass) error {
	objs, listRV, err := r.lister.List(ctx)
	if err != nil {
		r.handleError(ctx, fmt.Errorf("re-list failed: %w", err))
		return err
	}

	added, updated, removed := r.store.Replace(objs, listRV)
	r.logger(ctx).Debug("re-listed", "initial", pass == initialRelist, "count", len(objs),
		"added", len(added), "updated", len(updated), "removed", len(removed))

	for _, obj := range added {
		rv := int64(0)
		if pass == periodicRelist {
			rv = parseResourceVersion(obj.GetResourceVersion())
		}
		r.metrics.observeRelistEvent(relistVerbAdd, rv)
		r.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Add(ctx, obj) })
	}
	for _, obj := range updated {
		r.metrics.observeRelistEvent(relistVerbUpdate, 0)
		r.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Update(ctx, obj, obj) })
	}
	for _, obj := range removed {
		// The last-known version predates the delete, so it cannot date it.
		r.metrics.observeRelistEvent(relistVerbDelete, 0)
		r.dispatch(ctx, func(ctx context.Context, w ResourceWatcher) error { return w.Delete(ctx, obj) })
	}
	return nil
}

func (r *RelistInformer) dispatch(ctx context.Context, fn func(context.Context, ResourceWatcher) error) {
	r.mu.Lock()
	handlers := r.handlers
	r.mu.Unlock()
	for _, handler := range handlers {
		if err := fn(ctx, handler); err != nil {
			r.handleError(ctx, err)
		}
	}
}

func (r *RelistInformer) handleError(ctx context.Context, err error) {
	if r.opts.ErrorHandler != nil {
		r.opts.ErrorHandler(ctx, err)
		return
	}
	logging.FromContext(ctx).Error(err.Error(), "component", "RelistInformer",
		"kind", r.kind.GroupVersionKind().String(), "error", err)
}
