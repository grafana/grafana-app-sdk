package operator

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestNewClientLister_Validation(t *testing.T) {
	_, err := NewClientLister(nil, relistTestKind, ClientListerOptions{})
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
					page.Items = []resource.Object{relistObj("a")}
				case "page-2":
					// Later pages repeat the pinned version; only the first page's is used.
					page.ResourceVersion = "150"
					page.Items = []resource.Object{relistObj("b")}
				default:
					return fmt.Errorf("unexpected continue token %q", options.Continue)
				}
				return nil
			},
		}
		lister, err := NewClientLister(client, relistTestKind, ClientListerOptions{
			ListWatchOptions: ListWatchOptions{Namespace: relistTestNamespace, LabelFilters: []string{"app=test"}},
			PageSize:         1,
		})
		require.NoError(t, err)

		objs, listRV, err := lister.List(context.Background())
		require.NoError(t, err)

		assert.Equal(t, []string{"a", "b"}, relistNames(objs))
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
				page.Items = []resource.Object{relistObj("a")}
				return nil
			},
		}
		lister, err := NewClientLister(client, relistTestKind, ClientListerOptions{})
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.Error(t, err)
		assert.Nil(t, objs, "a partial snapshot is never returned")
	})

	t.Run("records a request per page and the projection", func(t *testing.T) {
		m := newTestRelistMetrics()
		kindLabel := relistTestKind.GroupVersionKind().String()
		client := mockListWatchClient{
			ListIntoFunc: func(_ context.Context, _ string, options resource.ListOptions, into resource.ListObject) error {
				page := into.(*resource.UntypedList)
				if options.Continue == "" {
					page.Continue = "page-2"
				}
				return nil
			},
		}
		lister, err := NewClientLister(client, relistTestKind, ClientListerOptions{Recorder: m.RelistRecorder(relistTestKind)})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.NoError(t, err)

		assert.Equal(t, float64(2), relistMetricValue(t, m, "relist_requests", kindLabel))
		assert.Equal(t, float64(1), relistMetricValue(t, m, "relist_projection", kindLabel, string(ProjectionObjects)),
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
		lister, err := NewClientLister(client, relistTestKind, ClientListerOptions{})
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, DefaultListerPageSize, limit)
	})
}

// keyListerFunc adapts a function to ResourceKeyLister.
type keyListerFunc func(ctx context.Context) (string, iter.Seq2[ResourceKey, error])

func (f keyListerFunc) ListKeys(ctx context.Context) (string, iter.Seq2[ResourceKey, error]) {
	return f(ctx)
}

func staticKeys(listRV string, keys ...ResourceKey) ResourceKeyLister {
	return keyListerFunc(func(context.Context) (string, iter.Seq2[ResourceKey, error]) {
		return listRV, func(yield func(ResourceKey, error) bool) {
			for _, key := range keys {
				if !yield(key, nil) {
					return
				}
			}
		}
	})
}

func TestNewIdentityLister_Validation(t *testing.T) {
	_, err := NewIdentityLister(relistTestKind, nil, IdentityListerOptions{})
	assert.Error(t, err, "a nil key lister is rejected")

	_, err = NewIdentityLister(resource.Kind{}, staticKeys(""), IdentityListerOptions{})
	assert.Error(t, err, "an empty kind is rejected")
}

func TestIdentityLister_List(t *testing.T) {
	t.Run("builds identity objects from keys", func(t *testing.T) {
		m := newTestRelistMetrics()
		kindLabel := relistTestKind.GroupVersionKind().String()
		lister, err := NewIdentityLister(relistTestKind, staticKeys("150",
			ResourceKey{Namespace: relistTestNamespace, Name: "a", ResourceVersion: "11"},
			ResourceKey{Namespace: relistTestNamespace, Name: "b", ResourceVersion: "22"},
		), IdentityListerOptions{Recorder: m.RelistRecorder(relistTestKind)})
		require.NoError(t, err)

		objs, listRV, err := lister.List(context.Background())
		require.NoError(t, err)

		assert.Equal(t, []string{"a", "b"}, relistNames(objs))
		assert.Equal(t, "150", listRV)
		assert.Equal(t, "22", objs[1].GetResourceVersion(), "the version the snapshot arbitrates on is carried")
		assert.Equal(t, relistTestKind.GroupVersionKind(), objs[1].GroupVersionKind())
		assert.Equal(t, float64(1), relistMetricValue(t, m, "relist_requests", kindLabel))
		assert.Equal(t, float64(1), relistMetricValue(t, m, "relist_projection", kindLabel, string(ProjectionIdentities)))
	})

	// A stream which fails part-way fails the re-list: the informer diffs the snapshot whole, so a truncated one
	// reads as a mass deletion.
	t.Run("a partial stream fails the re-list", func(t *testing.T) {
		keys := keyListerFunc(func(context.Context) (string, iter.Seq2[ResourceKey, error]) {
			return "150", func(yield func(ResourceKey, error) bool) {
				if !yield(ResourceKey{Namespace: relistTestNamespace, Name: "a"}, nil) {
					return
				}
				yield(ResourceKey{}, errors.New("stream broke"))
			}
		})
		lister, err := NewIdentityLister(relistTestKind, keys, IdentityListerOptions{})
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.Error(t, err)
		assert.Nil(t, objs, "a partial snapshot is never returned")
	})

	// An identity carries no labels and no fields, so a lister which does not apply them server-side cannot serve a
	// scoped or filtered re-list at all.
	t.Run("reports unsupported for a scope it cannot honor", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			opts ListWatchOptions
		}{
			{"namespaced", ListWatchOptions{Namespace: "ns"}},
			{"label filtered", ListWatchOptions{LabelFilters: []string{"app=test"}}},
			{"field selected", ListWatchOptions{FieldSelectors: []string{"spec.x=1"}}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				lister, err := NewIdentityLister(relistTestKind, staticKeys("150"), IdentityListerOptions{
					ListWatchOptions: tt.opts,
				})
				require.NoError(t, err)

				_, _, err = lister.List(context.Background())
				assert.ErrorIs(t, err, ErrProjectionUnsupported)
			})
		}
	})

	t.Run("a lister which honors the scope serves it", func(t *testing.T) {
		lister, err := NewIdentityLister(relistTestKind, staticKeys("150",
			ResourceKey{Namespace: "ns", Name: "a"},
		), IdentityListerOptions{
			ListWatchOptions:       ListWatchOptions{Namespace: "ns"},
			HonorsListWatchOptions: true,
		})
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"a"}, relistNames(objs))
	})
}

func TestNewFallbackLister_Validation(t *testing.T) {
	_, err := NewFallbackLister(nil, staticLister())
	assert.Error(t, err)
	_, err = NewFallbackLister(staticLister(), nil)
	assert.Error(t, err)
}

func TestFallbackLister_List(t *testing.T) {
	unsupported := listerFunc(func(context.Context) ([]resource.Object, string, error) {
		return nil, "", fmt.Errorf("server is old: %w", ErrProjectionUnsupported)
	})

	t.Run("uses the primary when it serves the projection", func(t *testing.T) {
		var fellBack bool
		fallback := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			fellBack = true
			return nil, "", nil
		})
		lister, err := NewFallbackLister(staticLister(relistObj("a")), fallback)
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"a"}, relistNames(objs))
		assert.False(t, fellBack)
	})

	t.Run("falls back when the projection is unsupported", func(t *testing.T) {
		lister, err := NewFallbackLister(unsupported, staticLister(relistObj("a")))
		require.NoError(t, err)

		objs, _, err := lister.List(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"a"}, relistNames(objs), "the full list is still a correct snapshot")
	})

	// Any other error fails the re-list, so a transient outage is retried on the next tick rather than quietly
	// served by the more expensive path on every pass.
	t.Run("any other error fails the re-list", func(t *testing.T) {
		expected := errors.New("api unavailable")
		failing := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			return nil, "", expected
		})
		var fellBack bool
		fallback := listerFunc(func(context.Context) ([]resource.Object, string, error) {
			fellBack = true
			return nil, "", nil
		})
		lister, err := NewFallbackLister(failing, fallback)
		require.NoError(t, err)

		_, _, err = lister.List(context.Background())
		assert.ErrorIs(t, err, expected)
		assert.False(t, fellBack)
	})
}

func TestNewIdentityObject(t *testing.T) {
	obj := NewIdentityObject(relistTestKind, "ns", "name", "11")
	assert.Equal(t, "ns", obj.GetNamespace())
	assert.Equal(t, "name", obj.GetName())
	assert.Equal(t, "11", obj.GetResourceVersion())
	assert.Equal(t, relistTestKind.GroupVersionKind(), obj.GroupVersionKind())
}

// The composition an adopter actually wires: a key lister whose server does not serve the projection reports it
// through the sequence, the identity lister passes it up, and the fallback re-lists full objects.
func TestIdentityListerFallsBackWhenTheServerDoesNotServeTheProjection(t *testing.T) {
	keys := keyListerFunc(func(context.Context) (string, iter.Seq2[ResourceKey, error]) {
		return "", func(yield func(ResourceKey, error) bool) {
			yield(ResourceKey{}, fmt.Errorf("404 from keys endpoint: %w", ErrProjectionUnsupported))
		}
	})
	identities, err := NewIdentityLister(relistTestKind, keys, IdentityListerOptions{})
	require.NoError(t, err)
	lister, err := NewFallbackLister(identities, staticLister(relistObj("a")))
	require.NoError(t, err)

	objs, _, err := lister.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, relistNames(objs))
}
