package simple

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

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
