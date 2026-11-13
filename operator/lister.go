package operator

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/grafana/grafana-app-sdk/resource"
)

// DefaultListerPageSize is the page size the default ResourceLister requests when none is configured.
const DefaultListerPageSize = 500

// ResourceLister reads every object of one kind, for an informer which has no watch to re-deliver from a list.
//
// listResourceVersion is the version the snapshot was read at, for an informer which arbitrates a list against
// changes that raced it. An empty or non-numeric version disables that arbitration.
//
// The single method is the seam for a server-specific projection: an implementation which can ask storage for less
// than whole objects returns what it has, without the SDK knowing that endpoint exists.
//
// Experimental: this API may change in a minor release.
type ResourceLister interface {
	List(ctx context.Context) (objs []resource.Object, listResourceVersion string, err error)
}

// ClientListerOptions configures the default ResourceLister.
//
// Experimental: this API may change in a minor release.
type ClientListerOptions struct {
	// ListWatchOptions are the namespace and filters to apply to the list request.
	ListWatchOptions ListWatchOptions
	// PageSize is the number of items to request per page. An empty value uses DefaultListerPageSize.
	PageSize int
}

// NewClientLister returns the default ResourceLister: a paginated list of full objects over a ListWatchClient.
//
// It follows continue tokens to the end of the set, because a truncated snapshot does not read as stale to the
// informer reading it — a diffing informer takes it as a mass deletion, and a re-delivering one silently skips
// what it did not read. An error on any page therefore fails the whole list rather than returning what was read so
// far; the informer retries on its next pass.
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
	return out, listRV, nil
}

// PageFunc reads one page of a list. The SDK supplies the limit and the continue token; the resource, the namespace
// and the selectors are the caller's, closed over.
//
// Experimental: this API may change in a minor release.
type PageFunc func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error)

// PagedListerOptions configures the paged ResourceLister.
//
// Experimental: this API may change in a minor release.
type PagedListerOptions struct {
	// PageSize is the number of items to request per page. An empty value uses DefaultListerPageSize.
	PageSize int
}

// NewPagedLister returns a ResourceLister over a paginated list function, for a client the SDK does not own: a
// client-go generated clientset, whose List is a PageFunc but which cannot satisfy ListWatchClient. Prefer
// NewClientLister where there is a resource.Client to list through.
//
// Each listed item is converted into a resource.Object of the kind, so the client's items have to be convertible —
// a typed clientset's are; a dynamic client's unstructured ones are not, and are better listed through the kind's
// own client.
//
// It follows continue tokens to the end of the set, for the reason NewClientLister does. An expired continue token
// is one such error, rather than a reason to fall back to a single unpaginated list, which the server would
// silently cap into exactly the truncated snapshot this avoids.
//
// Experimental: this API may change in a minor release.
func NewPagedLister(kind resource.Kind, page PageFunc, opts PagedListerOptions) (ResourceLister, error) {
	if page == nil {
		return nil, errors.New("page function cannot be nil")
	}
	if kind.Schema == nil {
		return nil, errors.New("kind cannot be empty")
	}
	if opts.PageSize <= 0 {
		opts.PageSize = DefaultListerPageSize
	}
	return &pagedLister{kind: kind, page: page, opts: opts}, nil
}

type pagedLister struct {
	kind resource.Kind
	page PageFunc
	opts PagedListerOptions
}

var _ ResourceLister = (*pagedLister)(nil)

func (l *pagedLister) List(ctx context.Context) ([]resource.Object, string, error) {
	var (
		out         []resource.Object
		listRV      string
		continueTok string
	)
	for {
		page, err := l.page(ctx, metav1.ListOptions{Limit: int64(l.opts.PageSize), Continue: continueTok})
		if err != nil {
			return nil, "", fmt.Errorf("failed to list %s: %w", l.kind.GroupVersionKind().String(), err)
		}
		accessor, err := meta.ListAccessor(page)
		if err != nil {
			return nil, "", fmt.Errorf("list response is not a list: %w", err)
		}
		// A paginated list pins the resource version across pages via the continue token, so the first page's
		// version dates the whole snapshot.
		if listRV == "" {
			listRV = accessor.GetResourceVersion()
		}
		items, err := meta.ExtractList(page)
		if err != nil {
			return nil, "", fmt.Errorf("could not read the list's items: %w", err)
		}
		for _, item := range items {
			obj, err := toResourceObject(item, l.kind)
			if err != nil {
				return nil, "", err
			}
			out = append(out, obj)
		}
		continueTok = accessor.GetContinue()
		if continueTok == "" {
			break
		}
	}
	return out, listRV, nil
}
