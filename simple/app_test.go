package simple

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

func TestNewApp(t *testing.T) {
	t.Run("empty managed kind", func(t *testing.T) {
		a, err := NewApp(AppConfig{
			ManagedKinds: []AppManagedKind{{}},
		})
		assert.Nil(t, a)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot manage an empty kind")
	})
	t.Run("empty unmanaged kind", func(t *testing.T) {
		a, err := NewApp(AppConfig{
			UnmanagedKinds: []AppUnmanagedKind{{}},
		})
		assert.Nil(t, a)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot watch an empty kind")
	})
	t.Run("success", func(t *testing.T) {
		a, err := NewApp(AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: testKind(),
			}},
		})
		assert.NoError(t, err)
		assert.NotNil(t, a)
	})
}

func TestApp_Convert(t *testing.T) {
	t.Run("no converter", func(t *testing.T) {
		a := createTestApp(t, AppConfig{})
		ret, err := a.Convert(context.TODO(), app.ConversionRequest{
			SourceGVK: schema.GroupVersionKind{
				Group:   "foo",
				Version: "v1",
				Kind:    "bar",
			},
			TargetGVK: schema.GroupVersionKind{
				Group:   "foo",
				Version: "v2",
				Kind:    "bar",
			},
			Raw: app.RawObject{},
		})
		assert.Nil(t, ret)
		assert.Equal(t, app.ErrNotImplemented, err)
	})

	t.Run("conversion error", func(t *testing.T) {
		expectedErr := errors.New("conversion error")
		req := app.ConversionRequest{
			SourceGVK: schema.GroupVersionKind{
				Group:   "foo",
				Version: "v1",
				Kind:    "baz",
			},
			TargetGVK: schema.GroupVersionKind{
				Group:   "foo",
				Version: "v2",
				Kind:    "baz",
			},
			Raw: app.RawObject{},
		}
		a := createTestApp(t, AppConfig{
			Converters: map[schema.GroupKind]Converter{{
				Group: req.SourceGVK.Group,
				Kind:  req.SourceGVK.Kind,
			}: &testConverter{
				func(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
					return nil, expectedErr
				},
			}},
		})
		_, err := a.Convert(context.TODO(), req)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("conversion success", func(t *testing.T) {
		converted := []byte(`{"foo":"bar"}`)
		req := app.ConversionRequest{
			SourceGVK: schema.GroupVersionKind{
				Group:   "foo",
				Version: "v1",
				Kind:    "baz",
			},
			TargetGVK: schema.GroupVersionKind{
				Group:   "foo",
				Version: "v2",
				Kind:    "baz",
			},
			Raw: app.RawObject{},
		}
		a := createTestApp(t, AppConfig{
			Converters: map[schema.GroupKind]Converter{{
				Group: req.SourceGVK.Group,
				Kind:  req.SourceGVK.Kind,
			}: &testConverter{
				func(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
					return converted, nil
				},
			}},
		})
		ret, err := a.Convert(context.TODO(), req)
		assert.Nil(t, err)
		assert.Equal(t, converted, ret.Raw)
	})
}

func TestApp_CallCustomRoute(t *testing.T) {
	kind := testKind()
	id := resource.FullIdentifier{
		Group:     kind.Group(),
		Version:   kind.Version(),
		Kind:      kind.Kind(),
		Namespace: "foz",
		Name:      "baz",
	}

	t.Run("no kind", func(t *testing.T) {
		a := createTestApp(t, AppConfig{})
		rw := httptest.NewRecorder()
		err := a.CallCustomRoute(context.TODO(), rw, &app.CustomRouteRequest{
			ResourceIdentifier: id,
			Path:               "foo",
			Method:             http.MethodPost,
		})
		assert.Equal(t, app.ErrCustomRouteNotFound, err)
	})

	t.Run("no method", func(t *testing.T) {
		a, err := NewApp(AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: AppCustomRouteHandlers{
					AppCustomRoute{
						Path: "baz",
					}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
						return errors.New("error")
					},
				},
			}},
		})
		assert.Nil(t, a)
		require.NotNil(t, err)
		assert.Equal(t, "custom route cannot have an empty method", err.Error())
	})

	t.Run("no path", func(t *testing.T) {
		a, err := NewApp(AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: AppCustomRouteHandlers{
					AppCustomRoute{
						Method: AppCustomRouteMethodGet,
					}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
						return errors.New("error")
					},
				},
			}},
		})
		assert.Nil(t, a)
		require.NotNil(t, err)
		assert.Equal(t, "custom route cannot have an empty path", err.Error())
	})

	t.Run("no handler", func(t *testing.T) {
		a, err := NewApp(AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: AppCustomRouteHandlers{
					AppCustomRoute{
						Method: AppCustomRouteMethodGet,
						Path:   "baz",
					}: nil,
				},
			}},
		})
		assert.Nil(t, a)
		require.NotNil(t, err)
		assert.Equal(t, "custom route cannot have a nil handler", err.Error())
	})

	// TODO: can't error on duplicate entries in map, and compiler doesn't prevent it
	/*
		t.Run("duplicate routes", func(t *testing.T) {
			a, err := NewApp(AppConfig{
				ManagedKinds: []AppManagedKind{{
					Kind: kind,
					Routes: AppCustomRouteHandlers{
						AppCustomRoute{
							Method: AppCustomRouteMethodGet,
							Path:   "baz",
						}: func(ctx context.Context, request *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error) {
							return nil, nil
						},
						AppCustomRoute{
							Method: AppCustomRouteMethodGet,
							Path:   "baz",
						}: func(ctx context.Context, request *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error) {
							return nil, nil
						},
					},
				}},
			})
			assert.Nil(t, a)
			require.NotNil(t, err)
			assert.Equal(t, "custom route 'POST baz' already exists", err.Error())
		})
	*/

	t.Run("no subresource route", func(t *testing.T) {
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: AppCustomRouteHandlers{
					AppCustomRoute{
						Method: AppCustomRouteMethodGet,
						Path:   "baz",
					}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
						return errors.New("error")
					},
				},
			}},
		})
		rw := httptest.NewRecorder()
		err := a.CallCustomRoute(context.TODO(), rw, &app.CustomRouteRequest{
			ResourceIdentifier: id,
			Path:               "foo",
			Method:             http.MethodPost,
		})
		assert.Equal(t, app.ErrCustomRouteNotFound, err)
	})

	t.Run("incorrect method", func(t *testing.T) {
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: AppCustomRouteHandlers{
					AppCustomRoute{
						Method: AppCustomRouteMethodGet,
						Path:   "baz",
					}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
						return errors.New("error")
					},
				},
			}},
		})
		rw := httptest.NewRecorder()
		err := a.CallCustomRoute(context.TODO(), rw, &app.CustomRouteRequest{
			ResourceIdentifier: id,
			Path:               "baz",
			Method:             http.MethodPost,
		})
		assert.Equal(t, app.ErrCustomRouteNotFound, err)
	})

	t.Run("success", func(t *testing.T) {
		expectedErr := errors.New("error")
		expectedStatus := http.StatusConflict
		expectedBody := []byte("foo")
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: AppCustomRouteHandlers{
					AppCustomRoute{
						Method: AppCustomRouteMethodPost,
						Path:   "baz",
					}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
						writer.WriteHeader(expectedStatus)
						writer.Write(expectedBody)
						return expectedErr
					},
				},
			}},
		})
		rw := httptest.NewRecorder()
		err := a.CallCustomRoute(context.TODO(), rw, &app.CustomRouteRequest{
			ResourceIdentifier: id,
			Path:               "baz",
			Method:             http.MethodPost,
		})
		assert.Equal(t, expectedErr, err)
		assert.Equal(t, expectedStatus, rw.Result().StatusCode)
		resBody, err := io.ReadAll(rw.Result().Body)
		assert.Nil(t, err)
		assert.Equal(t, expectedBody, resBody)
	})
	t.Run("success, plural instead of kind", func(t *testing.T) {
		expectedErr := errors.New("error")
		expectedStatus := http.StatusConflict
		expectedBody := []byte("foo")
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: AppCustomRouteHandlers{
					AppCustomRoute{
						Method: AppCustomRouteMethodPost,
						Path:   "baz",
					}: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
						writer.WriteHeader(expectedStatus)
						writer.Write(expectedBody)
						return expectedErr
					},
				},
			}},
		})
		rw := httptest.NewRecorder()
		err := a.CallCustomRoute(context.TODO(), rw, &app.CustomRouteRequest{
			ResourceIdentifier: resource.FullIdentifier{
				Name:      id.Name,
				Namespace: id.Namespace,
				Group:     kind.Group(),
				Version:   kind.Version(),
				Plural:    kind.Plural(),
			},
			Path:   "baz",
			Method: http.MethodPost,
		})
		require.Equal(t, expectedErr, err)
		assert.Equal(t, expectedStatus, rw.Result().StatusCode)
		resBody, err := io.ReadAll(rw.Result().Body)
		assert.Nil(t, err)
		assert.Equal(t, expectedBody, resBody)
	})
}

func TestApp_ManagedKinds(t *testing.T) {
	kinds := []resource.Kind{testKind()}
	managed := make([]AppManagedKind, 0)
	for _, k := range kinds {
		managed = append(managed, AppManagedKind{Kind: k})
	}
	a := createTestApp(t, AppConfig{ManagedKinds: managed})
	assert.ElementsMatch(t, kinds, a.ManagedKinds())
}

func TestApp_Mutate(t *testing.T) {
	kind := testKind()
	req := &app.AdmissionRequest{
		Action:   resource.AdmissionActionCreate,
		Group:    kind.Group(),
		Version:  kind.Version(),
		Kind:     kind.Kind(),
		UserInfo: resource.AdmissionUserInfo{},
		Object: &resource.UntypedObject{
			Spec: map[string]any{
				"foo": "bar",
			},
		},
	}
	t.Run("missing kind", func(t *testing.T) {
		a := createTestApp(t, AppConfig{})
		ret, err := a.Mutate(context.TODO(), req)
		assert.Nil(t, ret)
		assert.Equal(t, app.ErrNotImplemented, err)
	})

	t.Run("no mutator", func(t *testing.T) {
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
			}},
		})
		ret, err := a.Mutate(context.TODO(), req)
		assert.Nil(t, ret)
		assert.Equal(t, app.ErrNotImplemented, err)
	})

	t.Run("mutator error", func(t *testing.T) {
		expectedErr := errors.New("error")
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				Mutator: &Mutator{
					MutateFunc: func(ctx context.Context, request *app.AdmissionRequest) (*app.MutatingResponse, error) {
						assert.Equal(t, req, request)
						return nil, expectedErr
					},
				},
			}},
		})
		ret, err := a.Mutate(context.TODO(), req)
		assert.Nil(t, ret)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("mutator success", func(t *testing.T) {
		expectedResp := &app.MutatingResponse{
			UpdatedObject: &resource.UntypedObject{
				Spec: map[string]any{
					"foo": "bar2",
				},
			},
		}
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				Mutator: &Mutator{
					MutateFunc: func(ctx context.Context, request *app.AdmissionRequest) (*app.MutatingResponse, error) {
						assert.Equal(t, req, request)
						return expectedResp, nil
					},
				},
			}},
		})
		ret, err := a.Mutate(context.TODO(), req)
		assert.Nil(t, err)
		assert.Equal(t, expectedResp, ret)
	})
}

func TestApp_Validate(t *testing.T) {
	kind := testKind()
	req := &app.AdmissionRequest{
		Action:   resource.AdmissionActionCreate,
		Group:    kind.Group(),
		Version:  kind.Version(),
		Kind:     kind.Kind(),
		UserInfo: resource.AdmissionUserInfo{},
		Object: &resource.UntypedObject{
			Spec: map[string]any{
				"foo": "bar",
			},
		},
	}
	t.Run("missing kind", func(t *testing.T) {
		a := createTestApp(t, AppConfig{})
		err := a.Validate(context.TODO(), req)
		assert.Equal(t, app.ErrNotImplemented, err)
	})

	t.Run("no validator", func(t *testing.T) {
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
			}},
		})
		err := a.Validate(context.TODO(), req)
		assert.Equal(t, app.ErrNotImplemented, err)
	})

	t.Run("validator error", func(t *testing.T) {
		expectedErr := errors.New("error")
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				Validator: &Validator{
					ValidateFunc: func(ctx context.Context, request *app.AdmissionRequest) error {
						assert.Equal(t, req, request)
						return expectedErr
					},
				},
			}},
		})
		err := a.Validate(context.TODO(), req)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("validator success", func(t *testing.T) {
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				Validator: &Validator{
					ValidateFunc: func(ctx context.Context, request *app.AdmissionRequest) error {
						assert.Equal(t, req, request)
						return nil
					},
				},
			}},
		})
		err := a.Validate(context.TODO(), req)
		assert.Nil(t, err)
	})
}

func TestApp_ManageKind(t *testing.T) {
	t.Run("error creating informer", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		a, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
					return nil, expected
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
			}},
		})
		assert.Nil(t, a)
		assert.Equal(t, expected, err)
	})
	t.Run("custom InformerSupplier", func(t *testing.T) {
		supplierCalled := false
		kind := testKind()
		options := operator.ListWatchOptions{
			Namespace:      "ns",
			FieldSelectors: []string{"f1", "f2"},
			LabelFilters:   []string{"l1", "l2"},
		}
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: func(k resource.Kind, clients resource.ClientGenerator, opts operator.InformerOptions) (operator.Informer, error) {
					supplierCalled = true
					assert.Equal(t, kind, k)
					assert.Equal(t, options, opts.ListWatchOptions)
					return &mockInformer{}, nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       kind,
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					Namespace:      options.Namespace,
					LabelFilters:   options.LabelFilters,
					FieldSelectors: options.FieldSelectors,
				},
			}},
		})
		assert.True(t, supplierCalled, "custom InformerSupplier was not called")
	})
	t.Run("reconciler shard filter skips unassigned object", func(t *testing.T) {
		var handler operator.ResourceWatcher
		reconcilerCalled := false
		filterCalled := false
		kind := testKind()

		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
					return &mockInformer{
						AddEventHandlerFunc: func(h operator.ResourceWatcher) error {
							handler = h
							return nil
						},
					}, nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				Reconciler: &operator.SimpleReconciler{
					ReconcileFunc: func(context.Context, operator.ReconcileRequest) (operator.ReconcileResult, error) {
						reconcilerCalled = true
						return operator.ReconcileResult{}, nil
					},
				},
				ReconcileOptions: BasicReconcileOptions{
					ShardFilter: testShardFilter(func(context.Context, resource.Object) (bool, error) {
						filterCalled = true
						return false, nil
					}),
				},
			}},
		})

		require.NotNil(t, handler)
		err := handler.Add(context.Background(), testObjectForKind(kind, "default", "test"))
		require.NoError(t, err)
		assert.True(t, filterCalled)
		assert.False(t, reconcilerCalled)
	})
	t.Run("watcher shard filter delegates assigned object", func(t *testing.T) {
		var handler operator.ResourceWatcher
		watcherCalled := false
		filterCalled := false
		kind := testKind()

		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
					return &mockInformer{
						AddEventHandlerFunc: func(h operator.ResourceWatcher) error {
							handler = h
							return nil
						},
					}, nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				Watcher: &operator.SimpleWatcher{
					AddFunc: func(context.Context, resource.Object) error {
						watcherCalled = true
						return nil
					},
				},
				ReconcileOptions: BasicReconcileOptions{
					UsePlain: true,
					ShardFilter: testShardFilter(func(context.Context, resource.Object) (bool, error) {
						filterCalled = true
						return true, nil
					}),
				},
			}},
		})

		require.NotNil(t, handler)
		err := handler.Add(context.Background(), testObjectForKind(kind, "default", "test"))
		require.NoError(t, err)
		assert.True(t, filterCalled)
		assert.True(t, watcherCalled)
	})
	t.Run("watcher shard filter returns errors", func(t *testing.T) {
		var handler operator.ResourceWatcher
		expectedErr := errors.New("lookup failed")
		var handledErr error
		kind := testKind()

		a := createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerOptions: operator.InformerOptions{
					ErrorHandler: func(context.Context, error) {},
				},
				InformerSupplier: func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
					return &mockInformer{
						AddEventHandlerFunc: func(h operator.ResourceWatcher) error {
							handler = h
							return nil
						},
					}, nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:    kind,
				Watcher: &operator.SimpleWatcher{},
				ReconcileOptions: BasicReconcileOptions{
					UsePlain: true,
					ShardFilter: testShardFilter(func(context.Context, resource.Object) (bool, error) {
						return false, expectedErr
					}),
				},
			}},
		})
		a.informerController.ErrorHandler = func(_ context.Context, err error) {
			handledErr = err
		}

		require.NotNil(t, handler)
		err := handler.Add(context.Background(), testObjectForKind(kind, "default", "test"))
		require.NoError(t, err)
		require.ErrorIs(t, handledErr, expectedErr)
	})
	t.Run("watcher shard filter preserves sync for opinionated watchers", func(t *testing.T) {
		var handler operator.ResourceWatcher
		kind := testKind()
		addCalled := false
		syncCalled := false

		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: func(kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions) (operator.Informer, error) {
					return &mockInformer{
						AddEventHandlerFunc: func(h operator.ResourceWatcher) error {
							handler = h
							return nil
						},
					}, nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				Watcher: &Watcher{
					AddFunc: func(context.Context, resource.Object) error {
						addCalled = true
						return nil
					},
					SyncFunc: func(context.Context, resource.Object) error {
						syncCalled = true
						return nil
					},
				},
				ReconcileOptions: BasicReconcileOptions{
					ShardFilter: testShardFilter(func(context.Context, resource.Object) (bool, error) {
						return true, nil
					}),
				},
			}},
		})

		obj := testObjectForKind(kind, "default", "test")
		obj.SetFinalizers([]string{kind.Plural() + "-finalizer"})
		require.NotNil(t, handler)
		err := handler.Add(context.Background(), obj)
		require.NoError(t, err)
		assert.False(t, addCalled)
		assert.True(t, syncCalled)
	})
	t.Run("reconciler shard filter skip does not invoke opinionated finalizer handling", func(t *testing.T) {
		kind := testKind()
		client := &mockPatchClient{}
		reconcilerCalled := false
		inner, err := operator.NewOpinionatedReconciler(client, kind.Plural()+"-finalizer")
		require.NoError(t, err)
		inner.Wrap(&operator.SimpleReconciler{
			ReconcileFunc: func(context.Context, operator.ReconcileRequest) (operator.ReconcileResult, error) {
				reconcilerCalled = true
				return operator.ReconcileResult{}, nil
			},
		})
		reconciler := newShardFilteredReconciler(kind, newShardFilterDecisions(""), testShardFilter(func(context.Context, resource.Object) (bool, error) {
			return false, nil
		}), inner)

		obj := testObjectForKind(kind, "default", "test")
		now := metav1.Now()
		obj.SetDeletionTimestamp(&now)
		obj.SetFinalizers([]string{kind.Plural() + "-finalizer"})

		_, err = reconciler.Reconcile(context.Background(), operator.ReconcileRequest{
			Action: operator.ReconcileActionUpdated,
			Object: obj,
		})
		require.NoError(t, err)
		assert.False(t, reconcilerCalled)
		assert.Zero(t, client.patchCount)
	})
	t.Run("watcher shard filter skip does not invoke opinionated finalizer handling", func(t *testing.T) {
		kind := testKind()
		client := &mockPatchClient{}
		deleteCalled := false
		inner, err := operator.NewOpinionatedWatcher(kind, client, operator.OpinionatedWatcherConfig{
			Finalizer: func(resource.Schema) string { return kind.Plural() + "-finalizer" },
		})
		require.NoError(t, err)
		inner.Wrap(&operator.SimpleWatcher{
			DeleteFunc: func(context.Context, resource.Object) error {
				deleteCalled = true
				return nil
			},
		}, true)
		watcher := newShardFilteredWatcher(kind, newShardFilterDecisions(""), testShardFilter(func(context.Context, resource.Object) (bool, error) {
			return false, nil
		}), inner)

		obj := testObjectForKind(kind, "default", "test")
		now := metav1.Now()
		obj.SetDeletionTimestamp(&now)
		obj.SetFinalizers([]string{kind.Plural() + "-finalizer"})

		err = watcher.Add(context.Background(), obj)
		require.NoError(t, err)
		assert.False(t, deleteCalled)
		assert.Zero(t, client.patchCount)
	})
	t.Run("watcher shard filter update falls back to source object when target is nil", func(t *testing.T) {
		kind := testKind()
		src := testObjectForKind(kind, "default", "source")

		var filteredObj resource.Object
		updateCalled := false
		watcher := newShardFilteredWatcher(kind, newShardFilterDecisions(""), testShardFilter(func(_ context.Context, obj resource.Object) (bool, error) {
			filteredObj = obj
			return true, nil
		}), &operator.SimpleWatcher{
			UpdateFunc: func(_ context.Context, gotSrc, gotTgt resource.Object) error {
				updateCalled = true
				assert.Same(t, src, gotSrc)
				assert.Nil(t, gotTgt)
				return nil
			},
		})

		err := watcher.Update(context.Background(), src, nil)
		require.NoError(t, err)
		assert.True(t, updateCalled)
		assert.Same(t, src, filteredObj)
	})
	t.Run("watcher shard filter delete skip does not delegate", func(t *testing.T) {
		kind := testKind()
		deleteCalled := false
		watcher := newShardFilteredWatcher(kind, newShardFilterDecisions(""), testShardFilter(func(context.Context, resource.Object) (bool, error) {
			return false, nil
		}), &operator.SimpleWatcher{
			DeleteFunc: func(context.Context, resource.Object) error {
				deleteCalled = true
				return nil
			},
		})

		err := watcher.Delete(context.Background(), testObjectForKind(kind, "default", "test"))
		require.NoError(t, err)
		assert.False(t, deleteCalled)
	})
	t.Run("shard filter metrics count decisions", func(t *testing.T) {
		kind := testKind()
		reconciler := newShardFilteredReconciler(kind, newShardFilterDecisions(""), testShardFilter(func(context.Context, resource.Object) (bool, error) {
			return false, nil
		}), &operator.SimpleReconciler{})

		_, err := reconciler.Reconcile(context.Background(), operator.ReconcileRequest{
			Action: operator.ReconcileActionCreated,
			Object: testObjectForKind(kind, "default", "test"),
		})
		require.NoError(t, err)

		provider, ok := reconciler.(interface{ PrometheusCollectors() []prometheus.Collector })
		require.True(t, ok)

		var metric dto.Metric
		found := false
		for _, collector := range provider.PrometheusCollectors() {
			ch := make(chan prometheus.Metric, 16)
			go func(c prometheus.Collector) {
				c.Collect(ch)
				close(ch)
			}(collector)
			for collected := range ch {
				if err := collected.Write(&metric); err != nil {
					t.Fatal(err)
				}
				if metric.Counter == nil {
					continue
				}
				labels := metric.GetLabel()
				if hasMetricLabel(labels, "decision", shardFilterDecisionSkipped) &&
					hasMetricLabel(labels, "event_type", string(operator.ResourceActionCreate)) &&
					hasMetricLabel(labels, "group", kind.Group()) &&
					hasMetricLabel(labels, "version", kind.Version()) &&
					hasMetricLabel(labels, "resource", kind.Plural()) {
					found = true
				}
			}
		}
		assert.True(t, found)
	})
}

func TestApp_Runner(t *testing.T) {
	// TODO
}

func createTestApp(t *testing.T, cfg AppConfig) *App {
	a, err := NewApp(cfg)
	require.Nil(t, err)
	return a
}

func testKind() resource.Kind {
	sch := resource.NewSimpleSchema("foo", "v1", &resource.UntypedObject{}, &resource.UntypedList{}, resource.WithKind("Bar"))
	return resource.Kind{
		Schema: sch,
		Codecs: map[resource.KindEncoding]resource.Codec{
			resource.KindEncodingJSON: resource.NewJSONCodec(),
		},
	}
}

type testConverter struct {
	convertFunc func(obj k8s.RawKind, targetAPIVersion string) ([]byte, error)
}

func (c *testConverter) Convert(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
	if c.convertFunc != nil {
		return c.convertFunc(obj, targetAPIVersion)
	}
	return nil, nil
}

type mockInformer struct {
	RunFunc             func(context.Context) error
	AddEventHandlerFunc func(operator.ResourceWatcher) error
}

func (m mockInformer) Run(ctx context.Context) error {
	if m.RunFunc != nil {
		return m.RunFunc(ctx)
	}
	return nil
}

func (m mockInformer) AddEventHandler(handler operator.ResourceWatcher) error {
	if m.AddEventHandlerFunc != nil {
		return m.AddEventHandlerFunc(handler)
	}
	return nil
}

func (m mockInformer) WaitForSync(ctx context.Context) error {
	return nil
}

type testShardFilter func(context.Context, resource.Object) (bool, error)

func (t testShardFilter) ShouldProcess(ctx context.Context, obj resource.Object) (bool, error) {
	return t(ctx, obj)
}

func testObjectForKind(kind resource.Kind, namespace, name string) resource.Object {
	obj := kind.ZeroValue()
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetGroupVersionKind(kind.GroupVersionKind())
	obj.SetCreationTimestamp(metav1.Now())
	return obj
}

type mockPatchClient struct {
	patchCount int
}

func (m *mockPatchClient) PatchInto(ctx context.Context, identifier resource.Identifier, req resource.PatchRequest, options resource.PatchOptions, into resource.Object) error {
	m.patchCount++
	finalizers := make([]string, 0)
	for _, op := range req.Operations {
		if op.Path == "/metadata/finalizers" {
			if cast, ok := op.Value.([]string); ok {
				finalizers = append(finalizers, cast...)
			}
		}
	}
	into.SetFinalizers(finalizers)
	return nil
}

func (*mockPatchClient) GetInto(context.Context, resource.Identifier, resource.Object) error {
	return nil
}

func hasMetricLabel(labels []*dto.LabelPair, key string, value string) bool {
	for _, label := range labels {
		if label.GetName() == key && label.GetValue() == value {
			return true
		}
	}
	return false
}
