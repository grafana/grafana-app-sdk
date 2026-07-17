package operator

import (
	"context"
	"sync"
	"testing"
	"time"

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

func TestKubernetesBasedInformer_Run_TombstoneDelete(t *testing.T) {
	obj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "default",
			Name:            "foo",
			ResourceVersion: "1",
		},
	}
	initialEventsBookmark := func(resourceVersion string) *resource.UntypedObject {
		return &resource.UntypedObject{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: resourceVersion,
				Annotations: map[string]string{
					metav1.InitialEventsAnnotationKey: "true",
				},
			},
		}
	}

	mux := sync.Mutex{}
	listCalls := 0
	watchCalls := 0
	firstWatch := make(chan watch.Event, 2)
	client := &mockListWatchClient{
		ListIntoFunc: func(_ context.Context, _ string, _ resource.ListOptions, into resource.ListObject) error {
			mux.Lock()
			defer mux.Unlock()
			listCalls++
			if listCalls == 1 {
				into.SetResourceVersion("1")
				into.SetItems([]resource.Object{obj})
				return nil
			}
			// The object is gone server-side by the time the informer relists
			into.SetResourceVersion("2")
			return nil
		},
		WatchFunc: func(_ context.Context, _ string, options resource.WatchOptions) (resource.WatchResponse, error) {
			mux.Lock()
			defer mux.Unlock()
			watchCalls++
			if watchCalls == 1 {
				if options.SendInitialEvents != nil && *options.SendInitialEvents {
					// WatchList, the initial state is streamed as watch events followed by a bookmark
					firstWatch <- watch.Event{Type: watch.Added, Object: obj}
					firstWatch <- watch.Event{Type: watch.Bookmark, Object: initialEventsBookmark("1")}
				}
				return &mockWatch{events: firstWatch}, nil
			}
			events := make(chan watch.Event, 1)
			if options.SendInitialEvents != nil && *options.SendInitialEvents {
				// The object is gone server-side by the time the informer re-establishes the watch
				events <- watch.Event{Type: watch.Bookmark, Object: initialEventsBookmark("2")}
			}
			// Close the channel so re-established watches end immediately, forcing the reflector
			// to relist rather than hang on an empty watch
			close(events)
			return &mockWatch{events: events}, nil
		},
	}

	handlerErrs := make(chan error, 10)
	inf, err := NewKubernetesBasedInformer(untypedKind, client, InformerOptions{
		ErrorHandler: func(_ context.Context, err error) {
			handlerErrs <- err
		},
	})
	require.NoError(t, err)

	addCh := make(chan resource.Object, 2)
	deleteCh := make(chan resource.Object, 2)
	err = inf.AddEventHandler(&SimpleWatcher{
		AddFunc: func(_ context.Context, object resource.Object) error {
			addCh <- object
			return nil
		},
		DeleteFunc: func(_ context.Context, object resource.Object) error {
			deleteCh <- object
			return nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go inf.Run(ctx)
	select {
	case <-addCh:
	case <-time.After(time.Second * 10):
		t.Fatal("timed out waiting for the add event")
	}

	// Close the watch to force a relist: a watch that ends without delivering any events makes the
	// reflector restart its list/watch cycle with a fresh list. The object is missing from that list,
	// so DeltaFIFO delivers its delete as a cache.DeletedFinalStateUnknown tombstone (see issue #1352)
	close(firstWatch)
	select {
	case deleted := <-deleteCh:
		assert.Same(t, obj, deleted)
	case err := <-handlerErrs:
		t.Fatalf("informer dropped the delete event: %v", err)
	case <-time.After(time.Second * 10):
		t.Fatal("timed out waiting for the delete event")
	}
}

func TestToResourceObject(t *testing.T) {
	obj := &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "default",
			Name:            "foo",
			ResourceVersion: "1",
		},
	}

	tests := []struct {
		name    string
		input   any
		want    resource.Object
		wantErr bool
	}{{
		name:  "resource.Object passes through",
		input: obj,
		want:  obj,
	}, {
		name:    "nil errors",
		input:   nil,
		wantErr: true,
	}, {
		name:    "uncastable errors",
		input:   "not an object",
		wantErr: true,
	}, {
		// DeltaFIFO wraps deletes missed during a relist in a tombstone (see issue #1352)
		name:  "DeletedFinalStateUnknown is unwrapped",
		input: cache.DeletedFinalStateUnknown{Key: "default/foo", Obj: obj},
		want:  obj,
	}, {
		name:    "DeletedFinalStateUnknown wrapping an uncastable object errors",
		input:   cache.DeletedFinalStateUnknown{Key: "default/foo", Obj: "not an object"},
		wantErr: true,
	}, {
		name:    "DeletedFinalStateUnknown with no object errors",
		input:   cache.DeletedFinalStateUnknown{Key: "default/foo"},
		wantErr: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := toResourceObject(test.input, untypedKind)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Same(t, test.want, got)
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
