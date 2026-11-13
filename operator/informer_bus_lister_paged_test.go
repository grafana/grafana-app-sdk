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

// busPage builds one page of a typed list, as a generated clientset's List returns.
func busPage(listRV, continueTok string, objs ...resource.Object) *resource.UntypedList {
	return &resource.UntypedList{
		ListMeta: metav1.ListMeta{ResourceVersion: listRV, Continue: continueTok},
		Items:    objs,
	}
}

func TestNewPagedLister_Validation(t *testing.T) {
	page := func(context.Context, metav1.ListOptions) (runtime.Object, error) { return nil, nil }

	_, err := NewPagedLister(busTestKind, nil, PagedListerOptions{})
	assert.Error(t, err, "a nil page function is rejected")

	_, err = NewPagedLister(resource.Kind{}, page, PagedListerOptions{})
	assert.Error(t, err, "an empty kind is rejected")
}

func TestPagedLister_List(t *testing.T) {
	t.Run("follows continue tokens to the end of the set", func(t *testing.T) {
		var requested []metav1.ListOptions
		lister, err := NewPagedLister(busTestKind, func(_ context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			requested = append(requested, opts)
			switch opts.Continue {
			case "":
				return busPage("150", "page-2", busObj("a")), nil
			case "page-2":
				// Later pages repeat the pinned version; only the first page's is used.
				return busPage("150", "", busObj("b")), nil
			default:
				return nil, fmt.Errorf("unexpected continue token %q", opts.Continue)
			}
		}, PagedListerOptions{PageSize: 1, Payload: PayloadHydrated})
		require.NoError(t, err)

		objs, listRV, err := lister.List(context.Background())
		require.NoError(t, err)

		assert.Equal(t, []string{"a", "b"}, busNames(objs))
		assert.Equal(t, "150", listRV, "the snapshot is dated by the first page")
		require.Len(t, requested, 2)
		assert.Equal(t, int64(1), requested[0].Limit)
		assert.Empty(t, requested[0].Continue)
		assert.Equal(t, "page-2", requested[1].Continue)
	})

	t.Run("an error on a later page fails the whole list", func(t *testing.T) {
		lister, err := NewPagedLister(busTestKind, func(_ context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			if opts.Continue != "" {
				// What an expired continue token looks like: a partial snapshot must never be returned, because
				// the informer would diff it as a mass deletion.
				return nil, errors.New("the continue parameter is too old")
			}
			return busPage("", "page-2", busObj("a")), nil
		}, PagedListerOptions{})
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.Error(t, err)
		assert.Nil(t, objs)
	})

	t.Run("a response which is not a list fails the re-list", func(t *testing.T) {
		lister, err := NewPagedLister(busTestKind, func(context.Context, metav1.ListOptions) (runtime.Object, error) {
			return busObj("a"), nil
		}, PagedListerOptions{})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.Error(t, err)
	})

	t.Run("the default page size is requested when none is configured", func(t *testing.T) {
		var limit int64
		lister, err := NewPagedLister(busTestKind, func(_ context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			limit = opts.Limit
			return busPage("1", ""), nil
		}, PagedListerOptions{})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(DefaultListerPageSize), limit)
	})

	// Every page is one request, and the server returned objects however the lister projected them.
	t.Run("records one request per page and the objects projection", func(t *testing.T) {
		m := newTestBusMetrics()
		var served bool
		lister, err := NewPagedLister(busTestKind, func(context.Context, metav1.ListOptions) (runtime.Object, error) {
			if served {
				return busPage("1", "", busObj("b")), nil
			}
			served = true
			return busPage("1", "next", busObj("a")), nil
		}, PagedListerOptions{Recorder: m.RelistRecorder(busTestKind)})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.NoError(t, err)

		assert.Equal(t, float64(2), busMetricValue(t, m, "relist_requests"))
		assert.Equal(t, float64(1), busMetricValue(t, m, "relist_projection", string(ProjectionObjects)))
		assert.Zero(t, busMetricValue(t, m, "relist_projection", string(ProjectionIdentities)),
			"a local projection is not the server serving identities")
	})
}

// The identity payload reads each item through the meta accessor, so it serves a client whose objects the SDK
// cannot convert — a dynamic client's unstructured ones.
func TestPagedLister_IdentityPayloadProjectsAnyListedObject(t *testing.T) {
	item := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.grafana.app/v1",
		"kind":       "Widget",
		"metadata": map[string]any{
			"name":            "a",
			"namespace":       busTestNamespace,
			"resourceVersion": "42",
			"labels":          map[string]any{"app": "test"},
			"generation":      int64(3),
		},
		"spec": map[string]any{"field": "dropped"},
	}}
	page := &unstructured.UnstructuredList{
		Object: map[string]any{"metadata": map[string]any{"resourceVersion": "150"}},
		Items:  []unstructured.Unstructured{item},
	}

	t.Run("projects identity and the metadata a live event can announce", func(t *testing.T) {
		lister, err := NewPagedLister(busTestKind, func(context.Context, metav1.ListOptions) (runtime.Object, error) {
			return page, nil
		}, PagedListerOptions{Payload: PayloadIdentity})
		require.NoError(t, err)

		objs, listRV, err := lister.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "150", listRV)
		require.Len(t, objs, 1)
		assert.Equal(t, "a", objs[0].GetName())
		assert.Equal(t, busTestNamespace, objs[0].GetNamespace())
		assert.Equal(t, "42", objs[0].GetResourceVersion())
		assert.Equal(t, map[string]string{"app": "test"}, objs[0].GetLabels(), "a selector has to have something to match on")
		assert.Equal(t, int64(3), objs[0].GetGeneration())
		assert.Equal(t, busTestKind.GroupVersionKind(), objs[0].GroupVersionKind())
	})

	t.Run("the hydrated payload refuses an object it cannot convert", func(t *testing.T) {
		lister, err := NewPagedLister(busTestKind, func(context.Context, metav1.ListOptions) (runtime.Object, error) {
			return page, nil
		}, PagedListerOptions{Payload: PayloadHydrated})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.Error(t, err)
	})
}
