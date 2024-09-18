package simple

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func TestApp_CallResourceCustomRoute(t *testing.T) {
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
		resp, err := a.CallResourceCustomRoute(context.TODO(), &app.ResourceCustomRouteRequest{
			ResourceIdentifier: id,
			SubresourcePath:    "foo",
			Method:             http.MethodPost,
		})
		assert.Nil(t, resp)
		assert.Equal(t, app.ErrCustomRouteNotFound, err)
	})

	t.Run("no methods", func(t *testing.T) {
		a, err := NewApp(AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: []AppCustomRouteHandler{{
					Path: "baz",
					Handler: func(ctx context.Context, req *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error) {
						return nil, errors.New("error")
					},
				}},
			}},
		})
		assert.Nil(t, a)
		require.NotNil(t, err)
		assert.Equal(t, "custom route cannot have no Methods", err.Error())
	})

	t.Run("duplicate routes", func(t *testing.T) {
		a, err := NewApp(AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: []AppCustomRouteHandler{{
					Path:    "baz",
					Methods: []string{http.MethodGet, http.MethodPost},
					Handler: func(ctx context.Context, req *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error) {
						return nil, errors.New("error")
					},
				}, {
					Path:    "baz",
					Methods: []string{http.MethodDelete, http.MethodPost},
					Handler: func(ctx context.Context, req *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error) {
						return nil, errors.New("error")
					},
				}},
			}},
		})
		assert.Nil(t, a)
		require.NotNil(t, err)
		assert.Equal(t, "custom route 'baz' already has a handler for method 'POST'", err.Error())
	})

	t.Run("no subresource route", func(t *testing.T) {
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: []AppCustomRouteHandler{{
					Path:    "baz",
					Methods: []string{http.MethodGet},
					Handler: func(ctx context.Context, req *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error) {
						return nil, errors.New("error")
					},
				}},
			}},
		})
		resp, err := a.CallResourceCustomRoute(context.TODO(), &app.ResourceCustomRouteRequest{
			ResourceIdentifier: id,
			SubresourcePath:    "foo",
			Method:             http.MethodPost,
		})
		assert.Nil(t, resp)
		assert.Equal(t, app.ErrCustomRouteNotFound, err)
	})

	t.Run("incorrect method", func(t *testing.T) {
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: []AppCustomRouteHandler{{
					Path:    "baz",
					Methods: []string{http.MethodGet},
					Handler: func(ctx context.Context, req *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error) {
						return nil, errors.New("error")
					},
				}},
			}},
		})
		resp, err := a.CallResourceCustomRoute(context.TODO(), &app.ResourceCustomRouteRequest{
			ResourceIdentifier: id,
			SubresourcePath:    "baz",
			Method:             http.MethodPost,
		})
		assert.Nil(t, resp)
		assert.Equal(t, app.ErrCustomRouteNotFound, err)
	})

	t.Run("success", func(t *testing.T) {
		expectedErr := errors.New("error")
		expectedStatus := http.StatusConflict
		expectedBody := []byte("foo")
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				CustomRoutes: []AppCustomRouteHandler{{
					Path:    "baz",
					Methods: []string{http.MethodPost},
					Handler: func(ctx context.Context, req *app.ResourceCustomRouteRequest) (*app.ResourceCustomRouteResponse, error) {
						return &app.ResourceCustomRouteResponse{
							StatusCode: expectedStatus,
							Body:       expectedBody,
						}, expectedErr
					},
				}},
			}},
		})
		resp, err := a.CallResourceCustomRoute(context.TODO(), &app.ResourceCustomRouteRequest{
			ResourceIdentifier: id,
			SubresourcePath:    "baz",
			Method:             http.MethodPost,
		})
		assert.Equal(t, expectedErr, err)
		assert.NotNil(t, resp)
		assert.Equal(t, expectedStatus, resp.StatusCode)
		assert.Equal(t, expectedBody, resp.Body)
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
	req := &resource.AdmissionRequest{
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
				Mutator: &resource.SimpleMutatingAdmissionController{
					MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
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
		expectedResp := &resource.MutatingResponse{
			UpdatedObject: &resource.UntypedObject{
				Spec: map[string]any{
					"foo": "bar2",
				},
			},
		}
		a := createTestApp(t, AppConfig{
			ManagedKinds: []AppManagedKind{{
				Kind: kind,
				Mutator: &resource.SimpleMutatingAdmissionController{
					MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
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
	req := &resource.AdmissionRequest{
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
				Validator: &resource.SimpleValidatingAdmissionController{
					ValidateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
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
				Validator: &resource.SimpleValidatingAdmissionController{
					ValidateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
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
