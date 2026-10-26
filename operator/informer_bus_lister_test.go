package operator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestNewClientLister_Validation(t *testing.T) {
	_, err := NewClientLister(nil, busTestKind, ClientListerOptions{})
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
					page.Items = []resource.Object{busObj("a")}
				case "page-2":
					// Later pages repeat the pinned version; only the first page's is used.
					page.ResourceVersion = "150"
					page.Items = []resource.Object{busObj("b")}
				default:
					return fmt.Errorf("unexpected continue token %q", options.Continue)
				}
				return nil
			},
		}
		lister, err := NewClientLister(client, busTestKind, ClientListerOptions{
			ListWatchOptions: ListWatchOptions{Namespace: busTestNamespace, LabelFilters: []string{"app=test"}},
			PageSize:         1,
		})
		require.NoError(t, err)

		objs, listRV, err := lister.List(context.Background())
		require.NoError(t, err)

		assert.Equal(t, []string{"a", "b"}, busNames(objs))
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
					// What an expired continue token looks like: a partial snapshot must never be returned,
					// because the informer would diff it as a mass deletion.
					return errors.New("the continue parameter is too old")
				}
				page := into.(*resource.UntypedList)
				page.Continue = "page-2"
				page.Items = []resource.Object{busObj("a")}
				return nil
			},
		}
		lister, err := NewClientLister(client, busTestKind, ClientListerOptions{})
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.Error(t, err)
		assert.Nil(t, objs, "a partial snapshot is never returned")
	})

	t.Run("records a request per page and the projection", func(t *testing.T) {
		m := newTestBusMetrics()
		kindLabel := busTestKind.GroupVersionKind().String()
		client := mockListWatchClient{
			ListIntoFunc: func(_ context.Context, _ string, options resource.ListOptions, into resource.ListObject) error {
				page := into.(*resource.UntypedList)
				if options.Continue == "" {
					page.Continue = "page-2"
				}
				return nil
			},
		}
		lister, err := NewClientLister(client, busTestKind, ClientListerOptions{Recorder: m.RelistRecorder(busTestKind)})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.NoError(t, err)

		assert.Equal(t, float64(2), busMetricValue(t, m, "relist_requests", kindLabel))
		assert.Equal(t, float64(1), busMetricValue(t, m, "relist_projection", kindLabel, string(ProjectionObjects)),
			"the default lister serves full objects")
	})

	t.Run("defaults the page size", func(t *testing.T) {
		var limit int
		client := mockListWatchClient{
			ListIntoFunc: func(_ context.Context, _ string, options resource.ListOptions, _ resource.ListObject) error {
				limit = options.Limit
				return nil
			},
		}
		lister, err := NewClientLister(client, busTestKind, ClientListerOptions{})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, DefaultListerPageSize, limit)
	})
}
