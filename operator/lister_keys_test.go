package operator

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/resource"
)

type keyListFunc func(context.Context) (string, iter.Seq2[ResourceKey, error])

func (f keyListFunc) ListKeys(ctx context.Context) (string, iter.Seq2[ResourceKey, error]) {
	return f(ctx)
}

func TestIdentityListerFallback(t *testing.T) {
	pageErr := errors.New("second page failed")
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
				return []resource.Object{listerObj("fallback")}, "150", nil
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
			} else {
				require.Equal(t, "123", rv)
				require.Equal(t, "110", objs[0].GetResourceVersion())
				require.Equal(t, "a", objs[0].GetNamespace())
			}
		})
	}
}
