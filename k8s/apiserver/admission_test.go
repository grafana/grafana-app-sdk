package apiserver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
)

func TestAppAdmission_Admit(t *testing.T) {
	defaultReq := func() admission.Attributes {
		return admission.NewAttributesRecord(&resource.UntypedObject{}, &resource.UntypedObject{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Update, &metav1.UpdateOptions{}, false, nil)
	}
	badReq := admission.NewAttributesRecord(&metav1.Status{}, &metav1.Status{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Operation("foo"), &metav1.UpdateOptions{}, false, nil)
	manifestWithMutation := func(ops []app.AdmissionOperation) app.ManifestData {
		return app.ManifestData{
			Versions: []app.ManifestVersion{{
				Name:   TestKind.Version(),
				Served: true,
				Kinds: []app.ManifestVersionKind{{
					Kind:   TestKind.Kind(),
					Plural: TestKind.Plural(),
					Admission: &app.AdmissionCapabilities{
						Mutation: &app.MutationCapability{
							Operations: ops,
						},
					},
				}},
			}},
		}
	}
	appNoMutationCall := &MockApp{
		MutateFunc: func(ctx context.Context, request *app.AdmissionRequest) (*app.MutatingResponse, error) {
			assert.Fail(t, "should not be called")
			return nil, nil
		},
	}

	tests := []struct {
		name          string
		appGetter     func() app.App
		manifestData  app.ManifestData
		req           admission.Attributes
		expectedError error
		expectedObj   runtime.Object
	}{{
		name: "uninitialized app",
		appGetter: func() app.App {
			return nil
		},
		req:           defaultReq(),
		expectedError: admission.NewForbidden(defaultReq(), errors.New("app is not initialized")),
	}, {
		name: "no admission info",
		appGetter: func() app.App {
			return appNoMutationCall
		},
		req:           defaultReq(),
		expectedError: nil,
	}, {
		name: "no operation match",
		appGetter: func() app.App {
			return appNoMutationCall
		},
		manifestData:  manifestWithMutation([]app.AdmissionOperation{app.AdmissionOperationCreate, app.AdmissionOperationDelete, app.AdmissionOperationConnect}),
		req:           defaultReq(),
		expectedError: nil,
	}, {
		name: "can't translate admission request",
		appGetter: func() app.App {
			return appNoMutationCall
		},
		manifestData:  manifestWithMutation([]app.AdmissionOperation{app.AdmissionOperationAny}),
		req:           badReq,
		expectedError: admission.NewForbidden(badReq, fmt.Errorf("unknown admission operation: %v", badReq.GetOperation())),
	}, {
		name: "app mutation not implemented",
		appGetter: func() app.App {
			return &MockApp{
				MutateFunc: func(ctx context.Context, request *app.AdmissionRequest) (*app.MutatingResponse, error) {
					return nil, app.ErrNotImplemented
				},
			}
		},
		manifestData:  manifestWithMutation([]app.AdmissionOperation{app.AdmissionOperationUpdate}),
		req:           defaultReq(),
		expectedError: nil,
	}, {
		name: "app mutation error",
		appGetter: func() app.App {
			return &MockApp{
				MutateFunc: func(ctx context.Context, request *app.AdmissionRequest) (*app.MutatingResponse, error) {
					return nil, errors.New("I AM ERROR")
				},
			}
		},
		manifestData:  manifestWithMutation([]app.AdmissionOperation{app.AdmissionOperationUpdate}),
		req:           defaultReq(),
		expectedError: admission.NewForbidden(defaultReq(), errors.New("I AM ERROR")),
	}, {
		name: "successful mutation",
		appGetter: func() app.App {
			return &MockApp{
				MutateFunc: func(ctx context.Context, request *app.AdmissionRequest) (*app.MutatingResponse, error) {
					return &app.MutatingResponse{
						UpdatedObject: &resource.UntypedObject{
							Spec: map[string]any{"foo": "bar"},
						},
					}, nil
				},
			}
		},
		manifestData: manifestWithMutation([]app.AdmissionOperation{app.AdmissionOperationUpdate}),
		req:          defaultReq(),
		expectedObj: &resource.UntypedObject{
			Spec: map[string]any{"foo": "bar"},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adm := appAdmission{
				appGetter:    test.appGetter,
				manifestData: test.manifestData,
			}
			err := adm.Admit(context.Background(), test.req, nil)
			assert.Equal(t, test.expectedError, err)
			if test.expectedObj != nil {
				assert.Equal(t, test.expectedObj, test.req.GetObject())
			}
		})
	}
}

func TestAppAdmission_Validate(t *testing.T) {
	defaultReq := func() admission.Attributes {
		return admission.NewAttributesRecord(&resource.UntypedObject{}, &resource.UntypedObject{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Update, &metav1.UpdateOptions{}, false, nil)
	}
	badReq := admission.NewAttributesRecord(&metav1.Status{}, &metav1.Status{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Operation("foo"), &metav1.UpdateOptions{}, false, nil)
	manifestWithValidation := func(ops []app.AdmissionOperation) app.ManifestData {
		return app.ManifestData{
			Versions: []app.ManifestVersion{{
				Name:   TestKind.Version(),
				Served: true,
				Kinds: []app.ManifestVersionKind{{
					Kind:   TestKind.Kind(),
					Plural: TestKind.Plural(),
					Admission: &app.AdmissionCapabilities{
						Validation: &app.ValidationCapability{
							Operations: ops,
						},
					},
				}},
			}},
		}
	}
	appNoValidationCall := &MockApp{
		ValidateFunc: func(ctx context.Context, request *app.AdmissionRequest) error {
			assert.Fail(t, "should not be called")
			return nil
		},
	}

	tests := []struct {
		name          string
		appGetter     func() app.App
		manifestData  app.ManifestData
		req           admission.Attributes
		expectedError error
	}{{
		name: "uninitialized app",
		appGetter: func() app.App {
			return nil
		},
		req:           defaultReq(),
		expectedError: admission.NewForbidden(defaultReq(), errors.New("app is not initialized")),
	}, {
		name: "no admission info",
		appGetter: func() app.App {
			return appNoValidationCall
		},
		req:           defaultReq(),
		expectedError: nil,
	}, {
		name: "no operation match",
		appGetter: func() app.App {
			return appNoValidationCall
		},
		manifestData:  manifestWithValidation([]app.AdmissionOperation{app.AdmissionOperationCreate, app.AdmissionOperationDelete, app.AdmissionOperationConnect}),
		req:           defaultReq(),
		expectedError: nil,
	}, {
		name: "can't translate admission request",
		appGetter: func() app.App {
			return appNoValidationCall
		},
		manifestData:  manifestWithValidation([]app.AdmissionOperation{app.AdmissionOperationAny}),
		req:           badReq,
		expectedError: admission.NewForbidden(badReq, fmt.Errorf("unknown admission operation: %v", badReq.GetOperation())),
	}, {
		name: "app validation not implemented",
		appGetter: func() app.App {
			return &MockApp{
				ValidateFunc: func(ctx context.Context, request *app.AdmissionRequest) error {
					return app.ErrNotImplemented
				},
			}
		},
		manifestData:  manifestWithValidation([]app.AdmissionOperation{app.AdmissionOperationUpdate}),
		req:           defaultReq(),
		expectedError: nil,
	}, {
		name: "app validation error",
		appGetter: func() app.App {
			return &MockApp{
				ValidateFunc: func(ctx context.Context, request *app.AdmissionRequest) error {
					return errors.New("I AM ERROR")
				},
			}
		},
		manifestData:  manifestWithValidation([]app.AdmissionOperation{app.AdmissionOperationUpdate}),
		req:           defaultReq(),
		expectedError: admission.NewForbidden(defaultReq(), errors.New("I AM ERROR")),
	}, {
		name: "successful validation",
		appGetter: func() app.App {
			return &MockApp{
				ValidateFunc: func(ctx context.Context, request *app.AdmissionRequest) error {
					return nil
				},
			}
		},
		manifestData: manifestWithValidation([]app.AdmissionOperation{app.AdmissionOperationUpdate}),
		req:          defaultReq(),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adm := appAdmission{
				appGetter:    test.appGetter,
				manifestData: test.manifestData,
			}
			err := adm.Validate(context.Background(), test.req, nil)
			assert.Equal(t, test.expectedError, err)
		})
	}
}

func TestAppAdmission_Handles(t *testing.T) {
	// This just exists to break if we expand the Handles() logic to do anything besides always return true
	adm := appAdmission{}
	assert.True(t, adm.Handles(admission.Create))
	assert.True(t, adm.Handles(admission.Update))
	assert.True(t, adm.Handles(admission.Delete))
	assert.True(t, adm.Handles(admission.Connect))
}

func TestTranslateAdmissionOperation(t *testing.T) {
	badObjectAttrs := admission.NewAttributesRecord(&metav1.Status{}, &metav1.Status{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Create, &metav1.UpdateOptions{}, false, nil)
	badOldObjectAttrs := admission.NewAttributesRecord(&resource.UntypedObject{}, &metav1.Status{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Update, &metav1.UpdateOptions{}, false, nil)

	tests := []struct {
		name          string
		attributes    admission.Attributes
		expected      *app.AdmissionRequest
		expectedError error
	}{{
		name:          "nil input",
		attributes:    nil,
		expected:      nil,
		expectedError: errors.New("admission.Attributes cannot be nil"),
	}, {
		name:          "unknown admission operation",
		attributes:    admission.NewAttributesRecord(&metav1.Status{}, &metav1.Status{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Operation("foo"), &metav1.UpdateOptions{}, false, nil),
		expected:      nil,
		expectedError: errors.New("unknown admission operation: foo"),
	}, {
		name:          "object not resource.Object",
		attributes:    badObjectAttrs,
		expected:      nil,
		expectedError: admission.NewForbidden(badObjectAttrs, fmt.Errorf("object is not a resource.Object")),
	}, {
		name:          "old object not resource.Object",
		attributes:    badOldObjectAttrs,
		expected:      nil,
		expectedError: admission.NewForbidden(badOldObjectAttrs, fmt.Errorf("oldObject is not a resource.Object")),
	}, {
		name:       "nil user info",
		attributes: admission.NewAttributesRecord(&resource.UntypedObject{}, &resource.TypedSpecObject[string]{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Update, &metav1.UpdateOptions{}, false, nil),
		expected: &app.AdmissionRequest{
			Action:    resource.AdmissionActionUpdate,
			Kind:      TestKind.Kind(),
			Group:     TestKind.Group(),
			Version:   TestKind.Version(),
			UserInfo:  resource.AdmissionUserInfo{},
			Object:    &resource.UntypedObject{},
			OldObject: &resource.TypedSpecObject[string]{},
		},
	}, {
		name: "full translation",
		attributes: admission.NewAttributesRecord(&resource.UntypedObject{}, &resource.TypedSpecObject[string]{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Update, &metav1.UpdateOptions{}, false, &user.DefaultInfo{
			Name:   "foo",
			UID:    "bar",
			Groups: []string{"foobar"},
			Extra: map[string][]string{
				"foo": []string{"bar"},
			},
		}),
		expected: &app.AdmissionRequest{
			Action:  resource.AdmissionActionUpdate,
			Kind:    TestKind.Kind(),
			Group:   TestKind.Group(),
			Version: TestKind.Version(),
			UserInfo: resource.AdmissionUserInfo{
				Username: "foo",
				UID:      "bar",
				Groups:   []string{"foobar"},
				Extra: map[string]any{
					"foo": []string{"bar"},
				},
			},
			Object:    &resource.UntypedObject{},
			OldObject: &resource.TypedSpecObject[string]{},
		},
	}, {
		name:       "action: create",
		attributes: admission.NewAttributesRecord(&resource.UntypedObject{}, &resource.TypedSpecObject[string]{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Create, &metav1.CreateOptions{}, false, nil),
		expected: &app.AdmissionRequest{
			Action:    resource.AdmissionActionCreate,
			Kind:      TestKind.Kind(),
			Group:     TestKind.Group(),
			Version:   TestKind.Version(),
			UserInfo:  resource.AdmissionUserInfo{},
			Object:    &resource.UntypedObject{},
			OldObject: &resource.TypedSpecObject[string]{},
		},
	}, {
		name:       "action: update",
		attributes: admission.NewAttributesRecord(&resource.UntypedObject{}, &resource.TypedSpecObject[string]{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Update, &metav1.UpdateOptions{}, false, nil),
		expected: &app.AdmissionRequest{
			Action:    resource.AdmissionActionUpdate,
			Kind:      TestKind.Kind(),
			Group:     TestKind.Group(),
			Version:   TestKind.Version(),
			UserInfo:  resource.AdmissionUserInfo{},
			Object:    &resource.UntypedObject{},
			OldObject: &resource.TypedSpecObject[string]{},
		},
	}, {
		name:       "action: delete",
		attributes: admission.NewAttributesRecord(&resource.UntypedObject{}, &resource.TypedSpecObject[string]{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Delete, &metav1.DeleteOptions{}, false, nil),
		expected: &app.AdmissionRequest{
			Action:    resource.AdmissionActionDelete,
			Kind:      TestKind.Kind(),
			Group:     TestKind.Group(),
			Version:   TestKind.Version(),
			UserInfo:  resource.AdmissionUserInfo{},
			Object:    &resource.UntypedObject{},
			OldObject: &resource.TypedSpecObject[string]{},
		},
	}, {
		name:       "action: connect",
		attributes: admission.NewAttributesRecord(&resource.UntypedObject{}, &resource.TypedSpecObject[string]{}, TestKind.GroupVersionKind(), "default", "foo", TestKind.GroupVersionResource(), "", admission.Connect, &metav1.CreateOptions{}, false, nil),
		expected: &app.AdmissionRequest{
			Action:    resource.AdmissionActionConnect,
			Kind:      TestKind.Kind(),
			Group:     TestKind.Group(),
			Version:   TestKind.Version(),
			UserInfo:  resource.AdmissionUserInfo{},
			Object:    &resource.UntypedObject{},
			OldObject: &resource.TypedSpecObject[string]{},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			converted, err := translateAdmissionAttributes(test.attributes)
			require.Equal(t, test.expectedError, err)
			if test.expected != nil {
				require.NotNil(t, converted)
				assert.Equal(t, *test.expected, *converted)
			} else {
				require.Nil(t, converted)
			}
		})
	}
}

var TestKind = resource.Kind{
	Schema: resource.NewSimpleSchema("test.ext.grafana.app", "v1alpha1", &resource.UntypedObject{}, &resource.UntypedList{}, resource.WithKind("Test")),
	Codecs: map[resource.KindEncoding]resource.Codec{
		resource.KindEncodingJSON: resource.NewJSONCodec(),
	},
}

type MockApp struct {
	ValidateFunc        func(ctx context.Context, request *app.AdmissionRequest) error
	MutateFunc          func(ctx context.Context, request *app.AdmissionRequest) (*app.MutatingResponse, error)
	ConvertFunc         func(ctx context.Context, req app.ConversionRequest) (*app.RawObject, error)
	CallCustomRouteFunc func(ctx context.Context, responseWriter app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error
	ManagedKindsFunc    func() []resource.Kind
	RunnerFunc          func() app.Runnable
}

func (m *MockApp) Validate(ctx context.Context, request *app.AdmissionRequest) error {
	if m.ValidateFunc != nil {
		return m.ValidateFunc(ctx, request)
	}
	return nil
}

func (m *MockApp) Mutate(ctx context.Context, request *app.AdmissionRequest) (*app.MutatingResponse, error) {
	if m.MutateFunc != nil {
		return m.MutateFunc(ctx, request)
	}
	return nil, nil
}

func (m *MockApp) Convert(ctx context.Context, req app.ConversionRequest) (*app.RawObject, error) {
	if m.ConvertFunc != nil {
		return m.ConvertFunc(ctx, req)
	}
	return nil, nil
}

func (m *MockApp) CallCustomRoute(ctx context.Context, responseWriter app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
	if m.CallCustomRouteFunc != nil {
		return m.CallCustomRouteFunc(ctx, responseWriter, request)
	}
	return nil
}

func (m *MockApp) ManagedKinds() []resource.Kind {
	if m.ManagedKindsFunc != nil {
		return m.ManagedKindsFunc()
	}
	return nil
}

func (m *MockApp) Runner() app.Runnable {
	if m.RunnerFunc != nil {
		return m.RunnerFunc()
	}
	return nil
}
