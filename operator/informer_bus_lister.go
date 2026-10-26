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

// ResourceLister reads every object of one kind, for a BusInformer or PeriodicInformer to diff or re-deliver.
//
// listResourceVersion is the version the snapshot was read at, which the informer's snapshot uses to arbitrate the
// re-list against live events which raced it; an empty or non-numeric version disables that arbitration.
//
// The single method is the seam for a server-specific projection: an implementation which can ask storage for
// identities alone (namespace, name and resource version, which is all the diff keys off and all a reconcile needs
// to re-fetch the object) returns those instead of full objects, without the SDK knowing that endpoint exists.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type ResourceLister interface {
	List(ctx context.Context) (objs []resource.Object, listResourceVersion string, err error)
}

// ClientListerOptions configures the default ResourceLister.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
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
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
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
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
var ErrProjectionUnsupported = errors.New("list projection unsupported")

// ResourceKey is one object's identity from a list projection which carries no object bodies: all a snapshot diff
// keys off, and all a reconcile needs to read the object for itself.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type ResourceKey struct {
	Namespace       string
	Name            string
	ResourceVersion string
}

// ResourceKeyLister reads one kind's identities from a server which can serve them without object bodies. It returns
// the resource version the snapshot was read at and a sequence which yields an error if the stream fails part-way.
//
// An implementation must honor the namespace and filters its re-list was built for, and must report
// ErrProjectionUnsupported — at construction, or from the sequence — when it cannot: the identities it returns carry
// no labels and no fields, so nothing downstream can apply a filter it did not, and listing the whole set instead
// would hand the informer objects the app never asked to watch.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type ResourceKeyLister interface {
	ListKeys(ctx context.Context) (listResourceVersion string, keys iter.Seq2[ResourceKey, error])
}

// IdentityListerOptions configures the identity ResourceLister.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
type IdentityListerOptions struct {
	// Recorder observes the list requests this lister issues and the projection which served them.
	// Its zero value discards the observations.
	Recorder RelistRecorder
}

// NewIdentityLister returns a ResourceLister which builds identity objects from a kind's keys, for a server which
// can list identities without object bodies. Pair it with NewFallbackLister so that a server which cannot serve the
// projection re-lists full objects instead.
//
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
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

func (l *identityLister) List(ctx context.Context) ([]resource.Object, string, error) {
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
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
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
// Experimental: the resource change bus is a work in progress and may change or be removed without notice.
func NewIdentityObject(kind resource.Kind, namespace, name, resourceVersion string) resource.Object {
	obj := kind.ZeroValue()
	obj.SetGroupVersionKind(kind.GroupVersionKind())
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.SetResourceVersion(resourceVersion)
	return obj
}
