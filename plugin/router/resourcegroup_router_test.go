package router_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/grafana/grafana-app-sdk/plugin/router"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testResource = resource.Kind{
		Schema: resource.NewSimpleSchema("test.resource", "v1", &Test{}, &resource.UntypedList{}, resource.WithKind("Test")),
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
	testResourceGroup = &rg{[]resource.Kind{testResource}}
)

type Test struct {
	resource.TypedSpecStatusObject[TestModel, TestStatus]
}

// Copy: We have to overwrite this otherwise the returned underlying value is resource.TypedSpecStatusObject[TestModel, TestStatus] and not Test
func (t *Test) Copy() resource.Object {
	inner := t.TypedSpecStatusObject.Copy().(*resource.TypedSpecStatusObject[TestModel, TestStatus])
	return &Test{
		TypedSpecStatusObject: *inner,
	}
}

type rg struct {
	kinds []resource.Kind
}

func (r *rg) Kinds() []resource.Kind {
	return r.kinds
}

type TestModel struct {
	SomeInfo string `json:"some_info"`
}

type TestStatus struct {
	Status string `json:"status"`
}

type fakeStore struct {
	addFunc    func(ctx context.Context, obj resource.Object) (resource.Object, error)
	getFunc    func(ctx context.Context, kind string, identifier resource.Identifier) (resource.Object, error)
	listFunc   func(ctx context.Context, kind string, options resource.StoreListOptions) (resource.ListObject, error)
	updateFunc func(ctx context.Context, obj resource.Object) (resource.Object, error)
	deleteFunc func(ctx context.Context, kind string, identifier resource.Identifier) error
}

func (s fakeStore) Add(ctx context.Context, obj resource.Object) (resource.Object, error) {
	if s.addFunc != nil {
		return s.addFunc(ctx, obj)
	}

	return nil, nil
}

func (s fakeStore) Get(ctx context.Context, kind string, identifier resource.Identifier) (resource.Object, error) {
	if s.getFunc != nil {
		return s.getFunc(ctx, kind, identifier)
	}

	return nil, nil
}

func (s fakeStore) List(ctx context.Context, kind string, options resource.StoreListOptions) (resource.ListObject, error) {
	if s.listFunc != nil {
		return s.listFunc(ctx, kind, options)
	}

	return nil, nil
}

func (s fakeStore) Update(ctx context.Context, obj resource.Object) (resource.Object, error) {
	if s.updateFunc != nil {
		return s.updateFunc(ctx, obj)
	}

	return nil, nil
}

func (s fakeStore) Delete(ctx context.Context, kind string, identifier resource.Identifier) error {
	if s.deleteFunc != nil {
		return s.deleteFunc(ctx, kind, identifier)
	}

	return nil
}

type fakeSender struct {
	sendFunc func(*backend.CallResourceResponse) error
}

func (s fakeSender) Send(r *backend.CallResourceResponse) error {
	if s.sendFunc != nil {
		return s.sendFunc(r)
	}

	return nil
}

func TestResourceGroupRouter_Create(t *testing.T) {
	t.Run("returns error as store returns error", func(t *testing.T) {
		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			addFunc: func(ctx context.Context, obj resource.Object) (resource.Object, error) {
				test := obj.(*Test)
				require.Equal(t, "some_info", test.Spec.SomeInfo)

				return nil, errors.New("error")
			},
		})
		require.NoError(t, err)

		test := testResource.ZeroValue().(*Test)
		test.Spec.SomeInfo = "some_info"

		b, err := json.Marshal(test)
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests",
				Method: http.MethodPost,
				Body:   b,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					require.Equal(t, http.StatusInternalServerError, response.Status)

					return nil
				},
			},
		)
		require.NoError(t, err)
	})

	t.Run("returns 202 Accepted when resource is created correctly", func(t *testing.T) {
		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			addFunc: func(ctx context.Context, obj resource.Object) (resource.Object, error) {
				test := obj.(*Test)
				require.Equal(t, "some_info", test.Spec.SomeInfo)

				return test, nil
			},
		})
		require.NoError(t, err)

		test := testResource.ZeroValue().(*Test)
		test.Spec.SomeInfo = "some_info"

		b, err := json.Marshal(test)
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests",
				Method: http.MethodPost,
				Body:   b,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusAccepted, response.Status)
					return nil
				},
			},
		)
		require.NoError(t, err)
	})
}

func TestResourceGroupRouter_List(t *testing.T) {
	t.Run("returns error as store returns error", func(t *testing.T) {
		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			listFunc: func(ctx context.Context, kind string, options resource.StoreListOptions) (resource.ListObject, error) {
				require.Equal(t, "Test", kind)

				return nil, errors.New("error")
			},
		})
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests",
				Method: http.MethodGet,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusInternalServerError, response.Status)

					return nil
				},
			},
		)
		require.NoError(t, err)
	})

	t.Run("returns 200 OK when resources are retrieved correctly", func(t *testing.T) {
		var (
			firstResource  = testResource.ZeroValue().(*Test)
			secondResource = testResource.ZeroValue().(*Test)
		)
		firstResource.Spec.SomeInfo = "first_resource_info"
		secondResource.Spec.SomeInfo = "second_resource_info"

		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			listFunc: func(ctx context.Context, kind string, options resource.StoreListOptions) (resource.ListObject, error) {
				require.Equal(t, "Test", kind)

				list := resource.TypedList[*Test]{}
				list.Items = []*Test{firstResource, secondResource}

				return &list, nil
			},
		})
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests",
				Method: http.MethodGet,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusOK, response.Status)

					var resources resource.TypedList[*Test]
					require.NoError(t, json.Unmarshal(response.Body, &resources))

					assert.Len(t, resources.Items, 2)
					assert.Contains(t, resources.Items, firstResource)
					assert.Contains(t, resources.Items, secondResource)

					return nil
				},
			},
		)
		require.NoError(t, err)
	})
}

func TestResourceGroupRouter_Get(t *testing.T) {
	t.Run("returns error as store returns error", func(t *testing.T) {
		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			getFunc: func(ctx context.Context, kind string, identifier resource.Identifier) (resource.Object, error) {
				require.Equal(t, "Test", kind)
				require.Equal(t, "some_test", identifier.Name)

				return nil, errors.New("error")
			},
		})
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests/some_test",
				Method: http.MethodGet,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusInternalServerError, response.Status)

					return nil
				},
			},
		)
		require.NoError(t, err)
	})

	t.Run("returns 200 OK when resource is retrieved correctly", func(t *testing.T) {
		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			getFunc: func(ctx context.Context, kind string, identifier resource.Identifier) (resource.Object, error) {
				require.Equal(t, "Test", kind)
				require.Equal(t, "some_test", identifier.Name)

				test := testResource.ZeroValue().(*Test)
				test.Spec.SomeInfo = "some_info"
				test.ObjectMeta.Name = identifier.Name

				return test, nil
			},
		})
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests/some_test",
				Method: http.MethodGet,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusOK, response.Status)

					var resource Test
					require.NoError(t, json.Unmarshal(response.Body, &resource))

					assert.Equal(t, "some_test", resource.GetName())
					assert.Contains(t, "some_info", resource.Spec.SomeInfo)

					return nil
				},
			},
		)
		require.NoError(t, err)
	})
}

func TestResourceGroupRouter_Update(t *testing.T) {
	t.Run("returns error as store returns error", func(t *testing.T) {
		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			updateFunc: func(ctx context.Context, obj resource.Object) (resource.Object, error) {
				test := obj.(*Test)
				require.Equal(t, "some_info", test.Spec.SomeInfo)

				return nil, errors.New("error")
			},
		})
		require.NoError(t, err)

		test := testResource.ZeroValue().(*Test)
		test.Spec.SomeInfo = "some_info"

		b, err := json.Marshal(test)
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests/some_test",
				Method: http.MethodPut,
				Body:   b,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusInternalServerError, response.Status)

					return nil
				},
			},
		)
		require.NoError(t, err)
	})

	t.Run("returns 202 Accepted when resource is updated correctly", func(t *testing.T) {
		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			updateFunc: func(ctx context.Context, obj resource.Object) (resource.Object, error) {
				test := obj.(*Test)
				require.Equal(t, "some_info", test.Spec.SomeInfo)

				return test, nil
			},
		})
		require.NoError(t, err)

		test := testResource.ZeroValue().(*Test)
		test.Spec.SomeInfo = "some_info"

		b, err := json.Marshal(test)
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests/some_test",
				Method: http.MethodPut,
				Body:   b,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusAccepted, response.Status)

					return nil
				},
			},
		)
		require.NoError(t, err)
	})
}

func TestResourceGroupRouter_Delete(t *testing.T) {
	t.Run("returns error as store returns error", func(t *testing.T) {
		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			deleteFunc: func(ctx context.Context, kind string, identifier resource.Identifier) error {
				require.Equal(t, "Test", kind)
				require.Equal(t, "some_test", identifier.Name)

				return errors.New("error")
			},
		})
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests/some_test",
				Method: http.MethodDelete,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusInternalServerError, response.Status)

					return nil
				},
			},
		)
		require.NoError(t, err)
	})

	t.Run("returns 204 No Content when resource is deleted correctly", func(t *testing.T) {
		router, err := router.NewResourceGroupRouterWithStore(testResourceGroup, metav1.NamespaceDefault, fakeStore{
			deleteFunc: func(ctx context.Context, kind string, identifier resource.Identifier) error {
				require.Equal(t, "Test", kind)
				require.Equal(t, "some_test", identifier.Name)

				return nil
			},
		})
		require.NoError(t, err)

		err = router.CallResource(
			context.Background(),
			&backend.CallResourceRequest{
				Path:   "test.resource/v1/tests/some_test",
				Method: http.MethodDelete,
			},
			fakeSender{
				sendFunc: func(response *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusNoContent, response.Status)

					return nil
				},
			},
		)
		require.NoError(t, err)
	})
}
