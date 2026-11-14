package operator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestNewClientLister_Validation(t *testing.T) {
	_, err := NewClientLister(nil, listerTestKind, ClientListerOptions{})
	assert.Error(t, err, "a nil client is rejected")

	_, err = NewClientLister(mockListWatchClient{}, resource.Kind{}, ClientListerOptions{})
	assert.Error(t, err, "an empty kind is rejected")
}

func TestClientLister_List(t *testing.T) {
	t.Run("follows continue tokens to the end of the set", func(t *testing.T) {
		var requested []resource.ListOptions
		client := mockListWatchClient{
			ListIntoFunc: func(_ context.Context, _ string, options resource.ListOptions, into resource.ListObject) error {
				requested = append(requested, options)
				page := into.(*resource.UntypedList)
				switch options.Continue {
				case "":
					page.ResourceVersion = "150"
					page.Continue = "page-2"
					page.Items = []resource.Object{listerObj("a")}
				case "page-2":
					// Later pages repeat the pinned version; only the first page's is used.
					page.ResourceVersion = "150"
					page.Items = []resource.Object{listerObj("b")}
				default:
					return fmt.Errorf("unexpected continue token %q", options.Continue)
				}
				return nil
			},
		}
		lister, err := NewClientLister(client, listerTestKind, ClientListerOptions{
			ListWatchOptions: ListWatchOptions{Namespace: listerTestNamespace, LabelFilters: []string{"app=test"}},
			PageSize:         1,
		})
		require.NoError(t, err)

		objs, listRV, err := lister.List(context.Background())
		require.NoError(t, err)

		assert.Equal(t, []string{"a", "b"}, listerNames(objs))
		assert.Equal(t, "150", listRV, "the snapshot is dated by the first page")
		require.Len(t, requested, 2)
		assert.Equal(t, 1, requested[0].Limit)
		assert.Equal(t, []string{"app=test"}, requested[0].LabelFilters)
		assert.Equal(t, "page-2", requested[1].Continue)
	})

	t.Run("an error on a later page fails the whole list", func(t *testing.T) {
		client := mockListWatchClient{
			ListIntoFunc: func(_ context.Context, _ string, options resource.ListOptions, into resource.ListObject) error {
				if options.Continue != "" {
					// What an expired continue token looks like: a partial snapshot must never be returned, since
					// the informer reading it cannot tell one from a set which really did shrink.
					return errors.New("the continue parameter is too old")
				}
				page := into.(*resource.UntypedList)
				page.Continue = "page-2"
				page.Items = []resource.Object{listerObj("a")}
				return nil
			},
		}
		lister, err := NewClientLister(client, listerTestKind, ClientListerOptions{})
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.Error(t, err)
		assert.Nil(t, objs, "a partial snapshot is never returned")
	})

	t.Run("defaults the page size", func(t *testing.T) {
		var limit int
		client := mockListWatchClient{
			ListIntoFunc: func(_ context.Context, _ string, options resource.ListOptions, _ resource.ListObject) error {
				limit = options.Limit
				return nil
			},
		}
		lister, err := NewClientLister(client, listerTestKind, ClientListerOptions{})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, DefaultListerPageSize, limit)
	})
}

// listerPage builds one page of a typed list, as a generated clientset's List returns.
func listerPage(listRV, continueTok string, objs ...resource.Object) *resource.UntypedList {
	return &resource.UntypedList{
		ListMeta: metav1.ListMeta{ResourceVersion: listRV, Continue: continueTok},
		Items:    objs,
	}
}

func TestNewPagedLister_Validation(t *testing.T) {
	page := func(context.Context, metav1.ListOptions) (runtime.Object, error) { return nil, nil }

	_, err := NewPagedLister(listerTestKind, nil, PagedListerOptions{})
	assert.Error(t, err, "a nil page function is rejected")

	_, err = NewPagedLister(resource.Kind{}, page, PagedListerOptions{})
	assert.Error(t, err, "an empty kind is rejected")
}

func TestPagedLister_List(t *testing.T) {
	t.Run("follows continue tokens to the end of the set", func(t *testing.T) {
		var requested []metav1.ListOptions
		lister, err := NewPagedLister(listerTestKind, func(_ context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			requested = append(requested, opts)
			switch opts.Continue {
			case "":
				return listerPage("150", "page-2", listerObj("a")), nil
			case "page-2":
				// Later pages repeat the pinned version; only the first page's is used.
				return listerPage("150", "", listerObj("b")), nil
			default:
				return nil, fmt.Errorf("unexpected continue token %q", opts.Continue)
			}
		}, PagedListerOptions{PageSize: 1})
		require.NoError(t, err)

		objs, listRV, err := lister.List(context.Background())
		require.NoError(t, err)

		assert.Equal(t, []string{"a", "b"}, listerNames(objs))
		assert.Equal(t, "150", listRV, "the snapshot is dated by the first page")
		require.Len(t, requested, 2)
		assert.Equal(t, int64(1), requested[0].Limit)
		assert.Empty(t, requested[0].Continue)
		assert.Equal(t, "page-2", requested[1].Continue)
	})

	t.Run("an error on a later page fails the whole list", func(t *testing.T) {
		lister, err := NewPagedLister(listerTestKind, func(_ context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			if opts.Continue != "" {
				// What an expired continue token looks like: a partial snapshot must never be returned, since the
				// informer reading it cannot tell one from a set which really did shrink.
				return nil, errors.New("the continue parameter is too old")
			}
			return listerPage("", "page-2", listerObj("a")), nil
		}, PagedListerOptions{})
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.Error(t, err)
		assert.Nil(t, objs, "a partial snapshot is never returned")
	})

	t.Run("a response which is not a list fails the list", func(t *testing.T) {
		lister, err := NewPagedLister(listerTestKind, func(context.Context, metav1.ListOptions) (runtime.Object, error) {
			return listerObj("a"), nil
		}, PagedListerOptions{})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.Error(t, err)
	})

	t.Run("an item the kind cannot be built from fails the list", func(t *testing.T) {
		lister, err := NewPagedLister(listerTestKind, func(context.Context, metav1.ListOptions) (runtime.Object, error) {
			return &unstructured.UnstructuredList{
				Items: []unstructured.Unstructured{{Object: map[string]any{
					"apiVersion": "example.grafana.app/v1",
					"kind":       "Widget",
					"metadata":   map[string]any{"name": "a"},
				}}},
			}, nil
		}, PagedListerOptions{})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.Error(t, err)
	})

	t.Run("defaults the page size", func(t *testing.T) {
		var limit int64
		lister, err := NewPagedLister(listerTestKind, func(_ context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			limit = opts.Limit
			return listerPage("1", ""), nil
		}, PagedListerOptions{})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(DefaultListerPageSize), limit)
	})
}
