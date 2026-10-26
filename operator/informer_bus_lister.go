package operator

import (
	"context"
	"errors"
	"fmt"

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
