package operator

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

// DefaultListerPageSize is the page size the default ResourceLister requests when none is configured.
const DefaultListerPageSize = 500

// ResourceLister reads every object of one kind, for a RelistInformer or PeriodicInformer to diff or re-deliver.
//
// listResourceVersion is the version the snapshot was read at, which the informer's snapshot uses to arbitrate the
// re-list against live events which raced it; an empty or non-numeric version disables that arbitration.
//
// The single method is the seam for a server-specific projection: an implementation which can ask storage for
// identities alone (namespace, name and resource version, which is all the diff keys off and all a reconcile needs
// to re-fetch the object) returns those instead of full objects, without the SDK knowing that endpoint exists. Such a
// lister declares it with an IdentityPayload() bool method returning true, as NewIdentityLister's does, so that the
// informer can report that its deliveries may carry identity only.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type ResourceLister interface {
	List(ctx context.Context) (objs []resource.Object, listResourceVersion string, err error)
}

// ClientListerOptions configures the default ResourceLister.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type ClientListerOptions struct {
	// ListWatchOptions are the namespace and filters to apply to the list request.
	ListWatchOptions ListWatchOptions
	// PageSize is the number of items to request per page. An empty value uses DefaultListerPageSize.
	PageSize int
	// Recorder observes the list requests this lister issues and the projection which served them.
	// Its zero value discards the observations.
	Recorder RelistRecorder
}

// NewClientLister returns the default ResourceLister: a paginated list of full objects over a resource.Client.
//
// It follows continue tokens to the end of the set, because a truncated snapshot does not read as stale — the
// informer diffs it as a mass deletion. An error on any page therefore fails the whole re-list rather than returning
// what was read so far; the informer retries on its next tick.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
func NewClientLister(client ListWatchClient, kind resource.Kind, opts ClientListerOptions) (ResourceLister, error) {
	if client == nil {
		return nil, errors.New("client cannot be nil")
	}
	if kind.Schema == nil {
		return nil, errors.New("kind cannot be empty")
	}
	if opts.PageSize <= 0 {
		opts.PageSize = DefaultListerPageSize
	}
	return &clientLister{client: client, kind: kind, opts: opts}, nil
}

type clientLister struct {
	client ListWatchClient
	kind   resource.Kind
	opts   ClientListerOptions
}

var _ ResourceLister = (*clientLister)(nil)

func (l *clientLister) List(ctx context.Context) ([]resource.Object, string, error) {
	var (
		out         []resource.Object
		listRV      string
		continueTok string
	)
	for {
		page := l.kind.ZeroListValue()
		err := l.client.ListInto(ctx, l.opts.ListWatchOptions.Namespace, resource.ListOptions{
			LabelFilters:   l.opts.ListWatchOptions.LabelFilters,
			FieldSelectors: l.opts.ListWatchOptions.FieldSelectors,
			Limit:          l.opts.PageSize,
			Continue:       continueTok,
		}, page)
		l.opts.Recorder.Request()
		if err != nil {
			return nil, "", fmt.Errorf("failed to list %s: %w", l.kind.GroupVersionKind().String(), err)
		}
		// A paginated list pins the resource version across pages via the continue token, so the first page's
		// version dates the whole snapshot.
		if listRV == "" {
			listRV = page.GetResourceVersion()
		}
		out = append(out, page.GetItems()...)
		continueTok = page.GetContinue()
		if continueTok == "" {
			break
		}
	}
	l.opts.Recorder.Projection(ProjectionObjects)
	return out, listRV, nil
}

// ErrProjectionUnsupported is returned by a ResourceLister whose projection is not available for a re-list: the
// server does not serve it, or it cannot honor the scope and filters the re-list asks for. It is what tells a
// fallback lister to re-list another way rather than fail the re-list, so an adopter whose server may or may not
// serve a projection wraps the two and lets the fallback decide per call.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
var ErrProjectionUnsupported = errors.New("list projection unsupported")

// ResourceKey is one object's identity from a list projection which carries no object bodies: all a snapshot diff
// keys off, and all a reconcile needs to read the object for itself.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type ResourceKey struct {
	Namespace       string
	Name            string
	ResourceVersion string
}

// ResourceKeyLister reads one kind's identities from a server which can serve them without object bodies. It
// returns the resource version the snapshot was read at and a sequence which yields an error if the stream fails
// part-way; an implementation which cannot serve the projection returns ErrProjectionUnsupported from the sequence.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type ResourceKeyLister interface {
	ListKeys(ctx context.Context) (listResourceVersion string, keys iter.Seq2[ResourceKey, error])
}

// IdentityListerOptions configures the identity ResourceLister.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
type IdentityListerOptions struct {
	// ListWatchOptions are the namespace and filters the re-list asks for.
	ListWatchOptions ListWatchOptions
	// HonorsListWatchOptions declares that the key lister applies ListWatchOptions server-side. An identity carries
	// no labels and no fields, so a lister which does not cannot be filtered at all: leaving this false makes a
	// scoped or filtered re-list report ErrProjectionUnsupported instead of silently listing the whole set.
	HonorsListWatchOptions bool
	// Recorder observes the list requests this lister issues and the projection which served them.
	// Its zero value discards the observations.
	Recorder RelistRecorder
}

// NewIdentityLister returns a ResourceLister which builds identity objects from a kind's keys, for a server which
// can list identities without object bodies. Pair it with NewFallbackLister so that a server which cannot serve the
// projection re-lists full objects instead.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
func NewIdentityLister(kind resource.Kind, keys ResourceKeyLister, opts IdentityListerOptions) (ResourceLister, error) {
	if keys == nil {
		return nil, errors.New("key lister cannot be nil")
	}
	if kind.Schema == nil {
		return nil, errors.New("kind cannot be empty")
	}
	return &identityLister{kind: kind, keys: keys, opts: opts}, nil
}

type identityLister struct {
	kind resource.Kind
	keys ResourceKeyLister
	opts IdentityListerOptions
}

var _ ResourceLister = (*identityLister)(nil)

// IdentityPayload reports that the lister delivers identity-only objects, so that an informer re-listing through it
// is refused an opinionated reconciler or watcher just as an identity-payload source would be.
func (*identityLister) IdentityPayload() bool { return true }

func (l *identityLister) List(ctx context.Context) ([]resource.Object, string, error) {
	if !l.opts.HonorsListWatchOptions && l.opts.ListWatchOptions.isFiltered() {
		return nil, "", fmt.Errorf("%w: the key lister cannot apply a namespace or filters", ErrProjectionUnsupported)
	}

	l.opts.Recorder.Request()
	listRV, keys := l.keys.ListKeys(ctx)
	var out []resource.Object
	for key, err := range keys {
		// The snapshot is diffed whole, so a stream which fails part-way fails the re-list: a shorter set does not
		// read as stale, it diffs as a mass deletion.
		if err != nil {
			return nil, "", err
		}
		out = append(out, NewIdentityObject(l.kind, key.Namespace, key.Name, key.ResourceVersion))
	}
	l.opts.Recorder.Projection(ProjectionIdentities)
	return out, listRV, nil
}

// NewFallbackLister returns a ResourceLister which lists through primary, and through fallback for a re-list whose
// projection primary reports as unsupported. Any other error fails the re-list, so a transient outage is retried on
// the next tick rather than quietly served by the more expensive path.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
func NewFallbackLister(primary, fallback ResourceLister) (ResourceLister, error) {
	if primary == nil || fallback == nil {
		return nil, errors.New("both listers are required")
	}
	return &fallbackLister{primary: primary, fallback: fallback}, nil
}

type fallbackLister struct {
	primary  ResourceLister
	fallback ResourceLister
}

var _ ResourceLister = (*fallbackLister)(nil)

// IdentityPayload reports whether either lister may deliver identity-only objects: which one serves a re-list is
// decided per call, so the declaration has to cover both.
func (l *fallbackLister) IdentityPayload() bool {
	return isIdentityPayload(l.primary) || isIdentityPayload(l.fallback)
}

func (l *fallbackLister) List(ctx context.Context) ([]resource.Object, string, error) {
	objs, listRV, err := l.primary.List(ctx)
	if err == nil {
		return objs, listRV, nil
	}
	if !errors.Is(err, ErrProjectionUnsupported) {
		return nil, "", err
	}
	logging.FromContext(ctx).Warn("list projection unsupported, re-listing the fallback way", "error", err)
	return l.fallback.List(ctx)
}

// NewIdentityObject returns the kind's zero value carrying one object's identity and nothing else. It is what an
// identity payload delivers and what an identity projection snapshots, so a lister built against a server-specific
// projection produces the same shape the informer does.
//
// Experimental: the relist informer is a work in progress and may change or be removed without notice.
func NewIdentityObject(kind resource.Kind, namespace, name, resourceVersion string) resource.Object {
	obj := kind.ZeroValue()
	obj.SetGroupVersionKind(kind.GroupVersionKind())
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.SetResourceVersion(resourceVersion)
	return obj
}

// isFiltered reports whether the re-list asks for anything narrower than every object of the kind.
func (o ListWatchOptions) isFiltered() bool {
	return o.Namespace != "" || len(o.LabelFilters) > 0 || len(o.FieldSelectors) > 0
}
