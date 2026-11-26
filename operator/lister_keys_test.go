package operator

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

type keyListFunc func(context.Context) (string, iter.Seq2[ResourceKey, error])

func (f keyListFunc) ListKeys(ctx context.Context) (string, iter.Seq2[ResourceKey, error]) {
	return f(ctx)
}

func TestIdentityListerFallback(t *testing.T) {
	pageErr := errors.New("second page failed")
	created := metav1.NewTime(time.Unix(123, 0))
	for _, tc := range []struct {
		name     string
		err      error
		want     []string
		fallback bool
	}{
		{name: "complete scan", want: []string{"first", "second"}},
		{name: "unsupported", err: ErrProjectionUnsupported, want: []string{"fallback"}, fallback: true},
		{name: "later page failed", err: pageErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := keyListFunc(func(context.Context) (string, iter.Seq2[ResourceKey, error]) {
				return "123", func(yield func(ResourceKey, error) bool) {
					if !yield(ResourceKey{Name: "first", Namespace: "a", ResourceVersion: "110"}, nil) {
						return
					}
					if tc.err != nil {
						yield(ResourceKey{}, tc.err)
						return
					}
					yield(ResourceKey{Name: "second"}, nil)
				}
			})
			fallbackCalled := false
			fallback := listerFunc(func(context.Context) ([]resource.Object, string, error) {
				fallbackCalled = true
				item := listerObj("fallback")
				item.SetCreationTimestamp(created)
				return []resource.Object{item}, "150", nil
			})
			lister, err := NewIdentityLister(listerTestKind, keys, fallback)
			require.NoError(t, err)
			objs, rv, err := lister.List(t.Context())
			require.Equal(t, tc.fallback, fallbackCalled)
			if tc.err == pageErr {
				require.ErrorIs(t, err, pageErr)
				require.Empty(t, rv)
				require.Nil(t, objs)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, listerNames(objs))
			if tc.fallback {
				require.Equal(t, "150", rv)
				require.Equal(t, created, objs[0].GetCreationTimestamp())
			} else {
				require.Equal(t, "123", rv)
				require.Equal(t, "110", objs[0].GetResourceVersion())
				require.Equal(t, "a", objs[0].GetNamespace())
			}
		})
	}
}

type keyListerClientGenerator struct {
	resource.ClientGenerator
	resource.Client
}

func (g keyListerClientGenerator) ClientFor(resource.Kind) (resource.Client, error) {
	return g.Client, nil
}

type keyListerFallbackClient struct {
	resource.Client
	list func(context.Context, string, resource.ListOptions, resource.ListObject) error
}

func (c keyListerFallbackClient) ListInto(ctx context.Context, ns string, opts resource.ListOptions, into resource.ListObject) error {
	return c.list(ctx, ns, opts, into)
}

func TestKeysPeriodicMechanismFactory(t *testing.T) {
	query := ListWatchOptions{Namespace: listerTestNamespace, LabelFilters: []string{"app=one"}, FieldSelectors: []string{"metadata.name=item"}}
	options := InformerOptions{
		ListWatchOptions:  query,
		WatchListPageSize: 5,
		WatchNeeds:        WatchOptions{Resync: &WatchResyncOptions{Detail: WatchDetailIdentity, ListPageSize: 7}},
	}
	for _, tc := range []struct {
		name       string
		factoryErr error
	}{
		{name: "unsupported", factoryErr: ErrProjectionUnsupported},
		{name: "authorization error", factoryErr: errors.New("forbidden")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := keyListerFallbackClient{list: func(_ context.Context, ns string, opts resource.ListOptions, into resource.ListObject) error {
				calls++
				require.Equal(t, query.Namespace, ns)
				require.Equal(t, query.LabelFilters, opts.LabelFilters)
				require.Equal(t, query.FieldSelectors, opts.FieldSelectors)
				require.Equal(t, 7, opts.Limit)
				item := listerObj("item")
				item.Spec = map[string]any{"body": "omitted"}
				into.SetItems([]resource.Object{item})
				return nil
			}}
			factoryCalled := false
			mechanism := NewPeriodicKeysList(func(kind resource.Kind, opts ClientListerOptions) (ResourceKeyLister, error) {
				factoryCalled = true
				require.Equal(t, listerTestKind, kind)
				require.Equal(t, ClientListerOptions{ListWatchOptions: query, PageSize: 7}, opts)
				return nil, tc.factoryErr
			}, PeriodicListOptions{PageSize: 3})
			supplier := NewSelectingInformerSupplier(nil, mechanism)
			informer, err := supplier(listerTestKind, keyListerClientGenerator{Client: client}, options)
			require.True(t, factoryCalled)
			if !errors.Is(tc.factoryErr, ErrProjectionUnsupported) {
				require.ErrorIs(t, err, tc.factoryErr)
				require.Zero(t, calls)
				return
			}
			require.NoError(t, err)
			var received []resource.Object
			require.NoError(t, informer.AddEventHandler(&SimpleWatcher{AddFunc: func(_ context.Context, obj resource.Object) error {
				received = append(received, obj)
				return nil
			}}))
			require.NoError(t, informer.(*PeriodicInformer).sweep(t.Context()))
			require.Equal(t, 1, calls)
			require.Equal(t, []string{"item"}, listerNames(received))
			require.Empty(t, received[0].GetSpec())
		})
	}
}
