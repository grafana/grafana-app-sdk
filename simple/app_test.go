package simple

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
		a := createTestApp(t)
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
		a := createTestApp(t)
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
		a.RegisterKindConverter(schema.GroupKind{
			Group: req.SourceGVK.Group,
			Kind:  req.SourceGVK.Kind,
		}, &testConverter{
			func(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
				return nil, expectedErr
			},
		})
		_, err := a.Convert(context.TODO(), req)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("conversion success", func(t *testing.T) {
		a := createTestApp(t)
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
		a.RegisterKindConverter(schema.GroupKind{
			Group: req.SourceGVK.Group,
			Kind:  req.SourceGVK.Kind,
		}, &testConverter{
			func(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
				return converted, nil
			},
		})
		ret, err := a.Convert(context.TODO(), req)
		assert.Nil(t, err)
		assert.Equal(t, converted, ret.Raw)
	})
}

func TestApp_CallSubresource(t *testing.T) {
	kind := testKind()
	id := resource.FullIdentifier{
		Group:     kind.Group(),
		Version:   kind.Version(),
		Kind:      kind.Kind(),
		Namespace: "foz",
		Name:      "baz",
	}

	t.Run("no kind", func(t *testing.T) {
		a := createTestApp(t)
		writer := httptest.NewRecorder()
		err := a.CallSubresource(context.TODO(), writer, &app.SubresourceRequest{
			ResourceIdentifier: id,
			SubresourcePath:    "foo",
			Method:             http.MethodPost,
		})
		assert.Equal(t, app.ErrNotImplemented, err)
		assert.Equal(t, http.StatusNotFound, writer.Result().StatusCode)
	})

	t.Run("no subresource route", func(t *testing.T) {
		a := createTestApp(t)
		err := a.ManageKind(AppManagedKind{
			Kind: kind,
			SubresourceRoutes: map[string]func(ctx context.Context, writer http.ResponseWriter, req *app.SubresourceRequest) error{
				"baz": func(ctx context.Context, writer http.ResponseWriter, req *app.SubresourceRequest) error {
					return errors.New("error")
				},
			},
		})
		require.Nil(t, err)
		writer := httptest.NewRecorder()
		err = a.CallSubresource(context.TODO(), writer, &app.SubresourceRequest{
			ResourceIdentifier: id,
			SubresourcePath:    "foo",
			Method:             http.MethodPost,
		})
		assert.Nil(t, err)
		assert.Equal(t, http.StatusNotFound, writer.Result().StatusCode)
	})

	t.Run("success", func(t *testing.T) {
		a := createTestApp(t)
		expectedErr := errors.New("error")
		expectedStatus := http.StatusConflict
		expectedBody := []byte("foo")
		err := a.ManageKind(AppManagedKind{
			Kind: kind,
			SubresourceRoutes: map[string]func(ctx context.Context, writer http.ResponseWriter, req *app.SubresourceRequest) error{
				"baz": func(ctx context.Context, writer http.ResponseWriter, req *app.SubresourceRequest) error {
					writer.WriteHeader(expectedStatus)
					_, err := writer.Write(expectedBody)
					assert.Nil(t, err)
					return expectedErr
				},
			},
		})
		require.Nil(t, err)
		writer := httptest.NewRecorder()
		err = a.CallSubresource(context.TODO(), writer, &app.SubresourceRequest{
			ResourceIdentifier: id,
			SubresourcePath:    "baz",
			Method:             http.MethodPost,
		})
		assert.Equal(t, expectedErr, err)
		assert.Equal(t, expectedStatus, writer.Result().StatusCode)
		assert.Equal(t, expectedBody, writer.Body.Bytes())
	})
}

func TestApp_ManagedKinds(t *testing.T) {
	a := createTestApp(t)
	kinds := []resource.Kind{testKind()}
	for _, k := range kinds {
		assert.Nil(t, a.ManageKind(AppManagedKind{Kind: k}))
	}
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
		a := createTestApp(t)
		ret, err := a.Mutate(context.TODO(), req)
		assert.Nil(t, ret)
		assert.Equal(t, app.ErrNotImplemented, err)
	})

	t.Run("no mutator", func(t *testing.T) {
		a := createTestApp(t)
		err := a.ManageKind(AppManagedKind{
			Kind: kind,
		})
		assert.Nil(t, err)
		ret, err := a.Mutate(context.TODO(), req)
		assert.Nil(t, ret)
		assert.Equal(t, app.ErrNotImplemented, err)
	})

	t.Run("mutator error", func(t *testing.T) {
		a := createTestApp(t)
		expectedErr := errors.New("error")
		err := a.ManageKind(AppManagedKind{
			Kind: kind,
			Mutator: &resource.SimpleMutatingAdmissionController{
				MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
					assert.Equal(t, req, request)
					return nil, expectedErr
				},
			},
		})
		assert.Nil(t, err)
		ret, err := a.Mutate(context.TODO(), req)
		assert.Nil(t, ret)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("mutator success", func(t *testing.T) {
		a := createTestApp(t)
		expectedResp := &resource.MutatingResponse{
			UpdatedObject: &resource.UntypedObject{
				Spec: map[string]any{
					"foo": "bar2",
				},
			},
		}
		err := a.ManageKind(AppManagedKind{
			Kind: kind,
			Mutator: &resource.SimpleMutatingAdmissionController{
				MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
					assert.Equal(t, req, request)
					return expectedResp, nil
				},
			},
		})
		assert.Nil(t, err)
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
		a := createTestApp(t)
		err := a.Validate(context.TODO(), req)
		assert.Equal(t, app.ErrNotImplemented, err)
	})

	t.Run("no validator", func(t *testing.T) {
		a := createTestApp(t)
		err := a.ManageKind(AppManagedKind{
			Kind: kind,
		})
		assert.Nil(t, err)
		err = a.Validate(context.TODO(), req)
		assert.Equal(t, app.ErrNotImplemented, err)
	})

	t.Run("validator error", func(t *testing.T) {
		a := createTestApp(t)
		expectedErr := errors.New("error")
		err := a.ManageKind(AppManagedKind{
			Kind: kind,
			Validator: &resource.SimpleValidatingAdmissionController{
				ValidateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
					assert.Equal(t, req, request)
					return expectedErr
				},
			},
		})
		assert.Nil(t, err)
		err = a.Validate(context.TODO(), req)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("validator success", func(t *testing.T) {
		a := createTestApp(t)
		err := a.ManageKind(AppManagedKind{
			Kind: kind,
			Validator: &resource.SimpleValidatingAdmissionController{
				ValidateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
					assert.Equal(t, req, request)
					return nil
				},
			},
		})
		assert.Nil(t, err)
		err = a.Validate(context.TODO(), req)
		assert.Nil(t, err)
	})
}

func TestApp_Runner(t *testing.T) {
	// TODO
}

func createTestApp(t *testing.T) *App {
	a, err := NewApp(AppConfig{})
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
