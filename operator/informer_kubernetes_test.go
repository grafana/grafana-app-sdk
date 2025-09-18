package operator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
