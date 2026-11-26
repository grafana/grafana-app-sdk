package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/grafana-app-sdk/resource"
)

// DefaultListerPageSize is the page size the default ResourceLister requests when none is configured.
const DefaultListerPageSize = 500

// ResourceLister reads every object of one kind, for an informer with no watch. A successful result represents a complete snapshot.
//
// listResourceVersion dates the snapshot, for an informer which arbitrates a list against changes that raced it. Periodic delivery does not require the version.
//
// Experimental: this API may change in a minor release.
type ResourceLister interface {
	List(ctx context.Context) (objs []resource.Object, listResourceVersion string, err error)
}

// ClientListerOptions configures a ResourceLister.
//
// Experimental: this API may change in a minor release.
type ClientListerOptions struct {
	// ListWatchOptions are the namespace and filters to apply to the list request.
	ListWatchOptions ListWatchOptions
	// PageSize is the number of items per page. An empty value uses DefaultListerPageSize.
	PageSize int64
}

// NewClientLister returns a ResourceLister reading whole objects over a ListWatchClient.
//
// It pages to the end of the set and fails the whole list on any page's error: a truncated snapshot does not read as
// stale to the informer reading it, which takes the missing objects for deleted or skips them outright.
//
// Experimental: this API may change in a minor release.
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

func clientListerFor(kind resource.Kind, clients resource.ClientGenerator, opts ClientListerOptions) (ResourceLister, error) {
	client, err := clientForList(kind, clients)
	if err != nil {
		return nil, err
	}
	return NewClientLister(client, kind, opts)
}

func clientForList(kind resource.Kind, clients resource.ClientGenerator) (resource.Client, error) {
	if clients == nil {
		return nil, errors.New("a client generator is required to list the kind")
	}
	return clients.ClientFor(kind)
}

func listOptionsFor(opts InformerOptions, defaultPageSize int64) ClientListerOptions {
	pageSize := opts.WatchListPageSize
	if resync := opts.ResyncOptions(); resync != nil {
		pageSize = resync.ListPageSize
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	return ClientListerOptions{ListWatchOptions: opts.ListWatchOptions, PageSize: pageSize}
}

type clientLister struct {
	client ListWatchClient
	kind   resource.Kind
	opts   ClientListerOptions
}

var _ ResourceLister = (*clientLister)(nil)

func (l *clientLister) List(ctx context.Context) ([]resource.Object, string, error) {
	return collectListPages(ctx, func(ctx context.Context, token string) (listPage, error) {
		page := l.kind.ZeroListValue()
		err := l.client.ListInto(ctx, l.opts.ListWatchOptions.Namespace, resource.ListOptions{
			LabelFilters:   l.opts.ListWatchOptions.LabelFilters,
			FieldSelectors: l.opts.ListWatchOptions.FieldSelectors,
			Limit:          int(l.opts.PageSize),
			Continue:       token,
		}, page)
		if err != nil {
			return listPage{}, fmt.Errorf("failed to list %s: %w", l.kind.GroupVersionKind().String(), err)
		}
		return listPage{objects: page.GetItems(), version: page.GetResourceVersion(), next: page.GetContinue()}, nil
	})
}

type listPage struct {
	objects []resource.Object
	version string
	next    string
}

func collectListPages(ctx context.Context, read func(context.Context, string) (listPage, error)) ([]resource.Object, string, error) {
	var out []resource.Object
	var version, token string
	for {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		RecordInformerListPage(ctx)
		page, err := read(ctx, token)
		if err != nil {
			return nil, "", err
		}
		if token == "" {
			version = page.version
		}
		out = append(out, page.objects...)
		if page.next == "" {
			return out, version, nil
		}
		token = page.next
	}
}
