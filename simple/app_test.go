package simple

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

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

func TestApp_PrometheusCollectors(t *testing.T) {
	t.Run("default ClientGenerator", func(t *testing.T) {
		a, err := NewApp(AppConfig{})
		require.NoError(t, err)
		reg, ok := a.clientGenerator.(*k8s.ClientRegistry)
		require.True(t, ok)
		assert.Subset(t, a.PrometheusCollectors(), reg.PrometheusCollectors())
	})
	t.Run("provided ClientGenerator with metrics.Provider", func(t *testing.T) {
		reg := k8s.NewClientRegistry(rest.Config{}, k8s.DefaultClientConfig())
		a, err := NewApp(AppConfig{ClientGenerator: reg})
		require.NoError(t, err)
		assert.Subset(t, a.PrometheusCollectors(), reg.PrometheusCollectors())
	})
	t.Run("manual registration does not duplicate", func(t *testing.T) {
		reg := k8s.NewClientRegistry(rest.Config{}, k8s.DefaultClientConfig())
		a, err := NewApp(AppConfig{ClientGenerator: reg})
		require.NoError(t, err)
		before := len(a.PrometheusCollectors())
		a.RegisterMetricsCollectors(reg.PrometheusCollectors()...)
		assert.Len(t, a.PrometheusCollectors(), before)
		promReg := prometheus.NewRegistry()
		for _, c := range a.PrometheusCollectors() {
			assert.NoError(t, promReg.Register(c))
		}
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
}

func TestApp_WatchOptions(t *testing.T) {
	// Each mechanism is labelled so a kind served by the API watch is told apart from one served by an offer which
	// provides the same options.
	const (
		apiWatch     = "api-watch"
		eventBus     = "event-bus"
		hydratingBus = "hydrating-bus"
		reList       = "re-list"
	)

	provisions := map[string]operator.WatchOptions{
		apiWatch:     {Detail: operator.WatchDetailFullObject, Trigger: operator.WatchTriggerChange},
		eventBus:     {Detail: operator.WatchDetailIdentity, Trigger: operator.WatchTriggerChange},
		hydratingBus: {Detail: operator.WatchDetailFullObject, Trigger: operator.WatchTriggerChange},
		reList:       {Detail: operator.WatchDetailFullObject, Trigger: operator.WatchTriggerSchedule},
	}

	supplierOf := func(mechanism string, called *string) InformerSupplier {
		return func(resource.Kind, resource.ClientGenerator, operator.InformerOptions) (operator.Informer, error) {
			if called != nil {
				*called = mechanism
			}
			return &optionedInformer{options: provisions[mechanism]}, nil
		}
	}

	tests := []struct {
		name   string
		needs  operator.WatchOptions
		offers []string
		plain  bool
		want   string
	}{
		{
			name:   "an unset need is served by the API watch",
			offers: []string{reList, eventBus},
			plain:  true,
			want:   apiWatch,
		},
		{
			name:   "an identity need takes the event bus",
			needs:  operator.WatchOptions{Detail: operator.WatchDetailIdentity},
			offers: []string{reList, eventBus},
			plain:  true,
			want:   eventBus,
		},
		{
			name:   "a kind needing whole objects per change takes the hydrating bus over the API watch",
			offers: []string{reList, eventBus, hydratingBus},
			plain:  true,
			want:   hydratingBus,
		},
		{
			name:   "a kind needing whole objects on a schedule takes the re-list",
			needs:  operator.WatchOptions{Trigger: operator.WatchTriggerSchedule},
			offers: []string{reList, eventBus},
			plain:  true,
			want:   reList,
		},
		{
			name:   "a kind needing whole objects on a schedule passes over an identity bus offered first",
			needs:  operator.WatchOptions{Trigger: operator.WatchTriggerSchedule},
			offers: []string{eventBus, reList},
			plain:  true,
			want:   reList,
		},
		{
			name:   "a kind needing only an identity takes whichever is offered first",
			needs:  operator.WatchOptions{Detail: operator.WatchDetailIdentity, Trigger: operator.WatchTriggerSchedule},
			offers: []string{reList, eventBus},
			plain:  true,
			want:   reList,
		},
		{
			name:   "an identity need with no bus offered falls back to the API watch",
			needs:  operator.WatchOptions{Detail: operator.WatchDetailIdentity},
			offers: []string{reList},
			plain:  true,
			want:   apiWatch,
		},
		{
			name:   "an opinionated handler needs whole objects, so an identity bus is passed over",
			needs:  operator.WatchOptions{Detail: operator.WatchDetailIdentity},
			offers: []string{eventBus, hydratingBus},
			want:   hydratingBus,
		},
	}

	for _, tt := range tests {
		informerConfig := func(called *string) AppInformerConfig {
			offers := make([]InformerSupplierOffer, 0, len(tt.offers))
			for _, mechanism := range tt.offers {
				offers = append(offers, InformerSupplierOffer{
					Provides: provisions[mechanism],
					Supplier: supplierOf(mechanism, called),
				})
			}
			return AppInformerConfig{
				InformerSupplier:        supplierOf(apiWatch, called),
				InformerSupplierDecider: NewInformerSupplierDecider(offers...),
			}
		}

		t.Run("managed kind: "+tt.name, func(t *testing.T) {
			var called string
			createTestApp(t, AppConfig{
				InformerConfig: informerConfig(&called),
				ManagedKinds: []AppManagedKind{{
					Kind:       testKind(),
					Reconciler: &Reconciler{},
					ReconcileOptions: BasicReconcileOptions{
						WatchOptions: tt.needs,
						UsePlain:     tt.plain,
					},
				}},
			})
			assert.Equal(t, tt.want, called)
		})

		t.Run("unmanaged kind: "+tt.name, func(t *testing.T) {
			var called string
			createTestApp(t, AppConfig{
				InformerConfig: informerConfig(&called),
				UnmanagedKinds: []AppUnmanagedKind{{
					Kind:       testKind(),
					Reconciler: &Reconciler{},
					ReconcileOptions: UnmanagedKindReconcileOptions{
						WatchOptions:   tt.needs,
						UseOpinionated: !tt.plain,
					},
				}},
			})
			assert.Equal(t, tt.want, called)
		})
	}

	t.Run("an offer with no supplier is passed over", func(t *testing.T) {
		var called string
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: supplierOf(apiWatch, &called),
				InformerSupplierDecider: NewInformerSupplierDecider(
					InformerSupplierOffer{Provides: provisions[eventBus]},
					InformerSupplierOffer{Provides: provisions[hydratingBus], Supplier: supplierOf(hydratingBus, &called)},
				),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{Detail: operator.WatchDetailIdentity},
					UsePlain:     true,
				},
			}},
		})
		assert.Equal(t, hydratingBus, called)
	})

	t.Run("the decider is given the kind and what it needs", func(t *testing.T) {
		var decidedFor string
		var decidedNeeds operator.WatchOptions
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplierDecider: func(kind resource.Kind, needs operator.WatchOptions) InformerSupplier {
					decidedFor = kind.GroupVersionKind().String()
					decidedNeeds = needs
					return nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions: operator.WatchOptions{Detail: operator.WatchDetailIdentity},
					UsePlain:     true,
				},
			}},
		})
		assert.Equal(t, testKind().GroupVersionKind().String(), decidedFor)
		assert.Equal(t, operator.WatchOptions{
			Detail:  operator.WatchDetailIdentity,
			Trigger: operator.WatchTriggerChange,
		}, decidedNeeds)
	})

	t.Run("a decider which picks nothing leaves the kind to the API watch", func(t *testing.T) {
		var called string
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplier: supplierOf(apiWatch, &called),
				InformerSupplierDecider: func(resource.Kind, operator.WatchOptions) InformerSupplier {
					return nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{UsePlain: true},
			}},
		})
		assert.Equal(t, apiWatch, called)
	})

	t.Run("the per-kind supplier is used in place of the decision", func(t *testing.T) {
		deciderCalled := false
		var called string
		createTestApp(t, AppConfig{
			InformerConfig: AppInformerConfig{
				InformerSupplierDecider: func(resource.Kind, operator.WatchOptions) InformerSupplier {
					deciderCalled = true
					return nil
				},
			},
			ManagedKinds: []AppManagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{
					WatchOptions:     operator.WatchOptions{Detail: operator.WatchDetailIdentity},
					UsePlain:         true,
					InformerSupplier: supplierOf(eventBus, &called),
				},
			}},
		})
		assert.Equal(t, eventBus, called)
		assert.False(t, deciderCalled, "InformerSupplierDecider was called")
	})

	t.Run("an informer which provides less than the kind needs is refused", func(t *testing.T) {
		_, err := NewApp(AppConfig{
			InformerConfig: AppInformerConfig{
				// Offered as the hydrating bus, but its informer says it only re-lists.
				InformerSupplierDecider: NewInformerSupplierDecider(InformerSupplierOffer{
					Provides: provisions[hydratingBus],
					Supplier: supplierOf(reList, nil),
				}),
			},
			ManagedKinds: []AppManagedKind{{
				Kind:             testKind(),
				Reconciler:       &Reconciler{},
				ReconcileOptions: BasicReconcileOptions{UsePlain: true},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(),
			"provides detail=full-object trigger=schedule, but its reconciler or watcher needs detail=full-object trigger=change")
	})

	t.Run("an opinionated handler refuses an informer which provides only identities", func(t *testing.T) {
		_, err := NewApp(AppConfig{
			UnmanagedKinds: []AppUnmanagedKind{{
				Kind:       testKind(),
				Reconciler: &Reconciler{},
				ReconcileOptions: UnmanagedKindReconcileOptions{
					WatchOptions:     operator.WatchOptions{Detail: operator.WatchDetailIdentity},
					UseOpinionated:   true,
					InformerSupplier: supplierOf(eventBus, nil),
				},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "but its opinionated reconciler or watcher needs detail=full-object")
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

type optionedInformer struct {
	mockInformer
	options operator.WatchOptions
}

func (o *optionedInformer) WatchOptions() operator.WatchOptions {
	return o.options
}
