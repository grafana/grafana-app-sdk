package operator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestKubernetesBasedInformer_HealthCheckName(t *testing.T) {
	tests := []struct {
		name     string
		kind     resource.Kind
		opts     InformerOptions
		expected string
	}{{
		name:     "simple",
		kind:     untypedKind,
		expected: "informer-tests.foo/bar",
	}, {
		name: "labels",
		kind: untypedKind,
		opts: InformerOptions{
			ListWatchOptions: ListWatchOptions{
				LabelFilters: []string{"foz=baz", "a=b"},
			},
		},
		expected: "informer-tests.foo/bar?labelSelector=foz=baz,a=b",
	}, {
		name: "fieldSelectors",
		kind: untypedKind,
		opts: InformerOptions{
			ListWatchOptions: ListWatchOptions{
				FieldSelectors: []string{"bar=foo", "b=a"},
			},
		},
		expected: "informer-tests.foo/bar?fieldSelector=bar=foo,b=a",
	}, {
		name: "labels and fieldSelectors",
		kind: untypedKind,
		opts: InformerOptions{
			ListWatchOptions: ListWatchOptions{
				LabelFilters:   []string{"foz=baz", "a=b"},
				FieldSelectors: []string{"bar=foo", "b=a"},
			},
		},
		expected: "informer-tests.foo/bar?labelSelector=foz=baz,a=b&fieldSelector=bar=foo,b=a",
	}, {
		name: "shardSelector",
		kind: untypedKind,
		opts: InformerOptions{
			ListWatchOptions: ListWatchOptions{
				ShardSelector: "shardRange(object.metadata.uid, '0x0', '0x8000000000000000')",
			},
		},
		expected: "informer-tests.foo/bar?shardSelector=shardRange(object.metadata.uid, '0x0', '0x8000000000000000')",
	}, {
		name: "labels and fieldSelectors and shardSelector",
		kind: untypedKind,
		opts: InformerOptions{
			ListWatchOptions: ListWatchOptions{
				LabelFilters:   []string{"foz=baz", "a=b"},
				FieldSelectors: []string{"bar=foo", "b=a"},
				ShardSelector:  "shardRange(object.metadata.uid, '0x0', '0x8000000000000000')",
			},
		},
		expected: "informer-tests.foo/bar?labelSelector=foz=baz,a=b&fieldSelector=bar=foo,b=a&shardSelector=shardRange(object.metadata.uid, '0x0', '0x8000000000000000')",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inf, err := NewKubernetesBasedInformer(test.kind, &mockListWatchClient{}, test.opts)
			require.Nil(t, err)
			assert.Equal(t, test.expected, inf.HealthCheckName())
		})
	}
}

var _ ListWatchClient = &mockListWatchClient{}

type mockListWatchClient struct {
	ListIntoFunc func(ctx context.Context, namespace string, options resource.ListOptions, into resource.ListObject) error
	WatchFunc    func(ctx context.Context, namespace string, options resource.WatchOptions) (resource.WatchResponse, error)
}

func (m mockListWatchClient) ListInto(ctx context.Context, namespace string, options resource.ListOptions, into resource.ListObject) error {
	if m.ListIntoFunc != nil {
		return m.ListIntoFunc(ctx, namespace, options, into)
	}
	return nil
}

func (m mockListWatchClient) Watch(ctx context.Context, namespace string, options resource.WatchOptions) (resource.WatchResponse, error) {
	if m.WatchFunc != nil {
		return m.WatchFunc(ctx, namespace, options)
	}
	return nil, nil
}

// fakeWatchResponse is a stub resource.WatchResponse that implements KubernetesCompatibleWatch
// so NewListerWatcher does not spin up an internal goroutine wrapping it.
type fakeWatchResponse struct {
	w *watch.FakeWatcher
}

func (f *fakeWatchResponse) Stop()                                     { f.w.Stop() }
func (f *fakeWatchResponse) WatchEvents() <-chan resource.WatchEvent   { return nil }
func (f *fakeWatchResponse) KubernetesWatch() watch.Interface          { return f.w }

func TestNewListerWatcher_PropagatesListWatchOptions(t *testing.T) {
	const shardExpr = "shardRange(object.metadata.uid, '0x0', '0x8000000000000000')"
	filterOpts := ListWatchOptions{
		Namespace:      "ns",
		LabelFilters:   []string{"a=b"},
		FieldSelectors: []string{"c=d"},
		ShardSelector:  shardExpr,
	}

	var (
		gotListOpts  resource.ListOptions
		gotWatchOpts resource.WatchOptions
	)
	mock := &mockListWatchClient{
		ListIntoFunc: func(_ context.Context, namespace string, options resource.ListOptions, _ resource.ListObject) error {
			assert.Equal(t, "ns", namespace)
			gotListOpts = options
			return nil
		},
		WatchFunc: func(_ context.Context, namespace string, options resource.WatchOptions) (resource.WatchResponse, error) {
			assert.Equal(t, "ns", namespace)
			gotWatchOpts = options
			return &fakeWatchResponse{w: watch.NewFake()}, nil
		},
	}

	lw, ok := NewListerWatcher(mock, untypedKind, filterOpts).(*cache.ListWatch)
	require.True(t, ok, "expected concrete *cache.ListWatch")

	ctx := context.Background()
	_, err := lw.ListWithContext(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, filterOpts.LabelFilters, gotListOpts.LabelFilters)
	assert.Equal(t, filterOpts.FieldSelectors, gotListOpts.FieldSelectors)
	assert.Equal(t, shardExpr, gotListOpts.ShardSelector)

	_, err = lw.WatchWithContext(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, filterOpts.LabelFilters, gotWatchOpts.LabelFilters)
	assert.Equal(t, filterOpts.FieldSelectors, gotWatchOpts.FieldSelectors)
	assert.Equal(t, shardExpr, gotWatchOpts.ShardSelector)
}
