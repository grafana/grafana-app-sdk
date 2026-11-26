package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/grafana-app-sdk/resource"
)

// DefaultListerPageSize is the page size the default ResourceLister requests when none is configured.
const DefaultListerPageSize = 500

// ResourceLister reads every object of one kind. A successful result must be a complete snapshot.
//
// listResourceVersion is the resourceVersion of the snapshot. NewBusRecovery uses it to order the LIST against bus
// notifications. The periodic LIST does not use it.
//
// Experimental: this API may change in a minor release.
type ResourceLister interface {
	List(ctx context.Context) (objs []resource.Object, listResourceVersion string, err error)
}

// ClientListerOptions configures NewClientLister.
//
// Experimental: this API may change in a minor release.
type ClientListerOptions struct {
	// ListWatchOptions holds the namespace and the selectors for the LIST request.
	ListWatchOptions ListWatchOptions
	// PageSize is the number of objects per page. Zero uses DefaultListerPageSize.
	PageSize int64
}

// NewClientLister returns a ResourceLister that reads full objects with client. It reads every page. If any page
// fails, the whole List fails, because a partial snapshot would look like deleted objects.
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
	if clients == nil {
		return nil, errors.New("a client generator is required to list the kind")
	}
	client, err := clients.ClientFor(kind)
	if err != nil {
		return nil, err
	}
	return NewClientLister(client, kind, opts)
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
			Limit:          int(l.opts.PageSize),
			Continue:       continueTok,
		}, page)
		if err != nil {
			return nil, "", fmt.Errorf("failed to list %s: %w", l.kind.GroupVersionKind().String(), err)
		}
		// The continue token keeps one version across pages, so the first page's version is the snapshot's.
		if continueTok == "" {
			listRV = page.GetResourceVersion()
		}
		out = append(out, page.GetItems()...)
		continueTok = page.GetContinue()
		if continueTok == "" {
			break
		}
	}
	return out, listRV, nil
}
