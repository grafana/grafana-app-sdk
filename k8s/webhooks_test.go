package k8s

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewWebhookServer(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		srv, err := NewWebhookServer(WebhookServerConfig{})
		assert.Nil(t, srv)
		assert.Equal(t, fmt.Errorf("config.Port must be a valid port number (between 1 and 65536)"), err)
	})

	t.Run("empty TLSConfig", func(t *testing.T) {
		srv, err := NewWebhookServer(WebhookServerConfig{
			Port: 1234,
		})
		assert.Nil(t, srv)
		assert.Equal(t, fmt.Errorf("config.TLSConfig.CertPath is required"), err)
	})

	t.Run("empty TLSConfig.KeyPath", func(t *testing.T) {
		srv, err := NewWebhookServer(WebhookServerConfig{
			Port: 1234,
			TLSConfig: TLSConfig{
				CertPath: "foo",
			},
		})
		assert.Nil(t, srv)
		assert.Equal(t, fmt.Errorf("config.TLSConfig.KeyPath is required"), err)
	})

	t.Run("minimum config", func(t *testing.T) {
		srv, err := NewWebhookServer(WebhookServerConfig{
			Port: 1234,
			TLSConfig: TLSConfig{
				CertPath: "foo",
				KeyPath:  "bar",
			},
		})
		assert.Nil(t, err)
		assert.Equal(t, 1234, srv.port)
		assert.Equal(t, TLSConfig{
			CertPath: "foo",
			KeyPath:  "bar",
		}, srv.tlsConfig)
	})

	t.Run("set controllers", func(t *testing.T) {
		defVal := &testValidatingAdmissionController{}
		defMut := &testMutatingAdmissionController{}
		schVal := &testValidatingAdmissionController{}
		schMut := &testMutatingAdmissionController{}
		srv, err := NewWebhookServer(WebhookServerConfig{
			Port: 1234,
			TLSConfig: TLSConfig{
				CertPath: "foo",
				KeyPath:  "bar",
			},
			DefaultValidatingController: defVal,
			DefaultMutatingController:   defMut,
			ValidatingControllers: map[*resource.Kind]resource.ValidatingAdmissionController{
				&testKind: schVal,
			},
			MutatingControllers: map[*resource.Kind]resource.MutatingAdmissionController{
				&testKind: schMut,
			},
		})

		assert.Nil(t, err)
		assert.Equal(t, defVal, srv.DefaultValidatingController)
		assert.Equal(t, defMut, srv.DefaultMutatingController)
		assert.Equal(t, map[string]validatingAdmissionControllerTuple{
			gvk(&metav1.GroupVersionKind{Group: testSchema.Group(), Version: testSchema.Version(), Kind: testSchema.Kind()}): {
				schema:     testKind,
				controller: schVal,
			},
		}, srv.validatingControllers)
		assert.Equal(t, map[string]mutatingAdmissionControllerTuple{
			gvk(&metav1.GroupVersionKind{Group: testSchema.Group(), Version: testSchema.Version(), Kind: testSchema.Kind()}): {
				schema:     testKind,
				controller: schMut,
			},
		}, srv.mutatingControllers)
	})
}

func TestWebhookServer_AddMutatingAdmissionController(t *testing.T) {
	srv, err := NewWebhookServer(WebhookServerConfig{
		Port: 1234,
		TLSConfig: TLSConfig{
			CertPath: "foo",
			KeyPath:  "bar",
		},
	})
	require.Nil(t, err)
	c1 := &testMutatingAdmissionController{}
	c2 := &testMutatingAdmissionController{}
	c3 := &testMutatingAdmissionController{}
	sch1 := resource.Kind{
		Schema: resource.NewSimpleSchema("foo", "v1", &TestResourceObject{}, &TestResourceObjectList{}, resource.WithKind("bar")),
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
	sch2 := resource.Kind{
		Schema: resource.NewSimpleSchema("bar", "v1", &TestResourceObject{}, &TestResourceObjectList{}, resource.WithKind("foo")),
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}

	assert.Empty(t, srv.mutatingControllers)
	srv.AddMutatingAdmissionController(c1, sch1)
	srv.AddMutatingAdmissionController(c2, sch2)
	assert.Equal(t, map[string]mutatingAdmissionControllerTuple{
		gvk(&metav1.GroupVersionKind{Group: "foo", Version: "v1", Kind: "bar"}): {
			schema:     sch1,
			controller: c1,
		},
		gvk(&metav1.GroupVersionKind{Group: "bar", Version: "v1", Kind: "foo"}): {
			schema:     sch2,
			controller: c2,
		},
	}, srv.mutatingControllers)
	// Overwrite
	srv.AddMutatingAdmissionController(c3, sch1)
	assert.Equal(t, map[string]mutatingAdmissionControllerTuple{
		gvk(&metav1.GroupVersionKind{Group: "foo", Version: "v1", Kind: "bar"}): {
			schema:     sch1,
			controller: c3,
		},
		gvk(&metav1.GroupVersionKind{Group: "bar", Version: "v1", Kind: "foo"}): {
			schema:     sch2,
			controller: c2,
		},
	}, srv.mutatingControllers)
}

func TestWebhookServer_AddValidatingAdmissionController(t *testing.T) {
	srv, err := NewWebhookServer(WebhookServerConfig{
		Port: 1234,
		TLSConfig: TLSConfig{
			CertPath: "foo",
			KeyPath:  "bar",
		},
	})
	require.Nil(t, err)
	c1 := &testValidatingAdmissionController{}
	c2 := &testValidatingAdmissionController{}
	c3 := &testValidatingAdmissionController{}
	sch1 := resource.Kind{
		Schema: resource.NewSimpleSchema("foo", "v1", &TestResourceObject{}, &TestResourceObjectList{}, resource.WithKind("bar")),
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
	sch2 := resource.Kind{
		Schema: resource.NewSimpleSchema("bar", "v1", &TestResourceObject{}, &TestResourceObjectList{}, resource.WithKind("foo")),
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}

	assert.Empty(t, srv.validatingControllers)
	srv.AddValidatingAdmissionController(c1, sch1)
	srv.AddValidatingAdmissionController(c2, sch2)
	assert.Equal(t, map[string]validatingAdmissionControllerTuple{
		gvk(&metav1.GroupVersionKind{Group: "foo", Version: "v1", Kind: "bar"}): {
			schema:     sch1,
			controller: c1,
		},
		gvk(&metav1.GroupVersionKind{Group: "bar", Version: "v1", Kind: "foo"}): {
			schema:     sch2,
			controller: c2,
		},
	}, srv.validatingControllers)
	// Overwrite
	srv.AddValidatingAdmissionController(c3, sch1)
	assert.Equal(t, map[string]validatingAdmissionControllerTuple{
		gvk(&metav1.GroupVersionKind{Group: "foo", Version: "v1", Kind: "bar"}): {
			schema:     sch1,
			controller: c3,
		},
		gvk(&metav1.GroupVersionKind{Group: "bar", Version: "v1", Kind: "foo"}): {
			schema:     sch2,
			controller: c2,
		},
	}, srv.validatingControllers)
}

var admissionRequestObject = &TestResourceObject{}
var admissionRequestObjectBytes = &bytes.Buffer{}
var _ = resource.NewJSONCodec().Write(admissionRequestObjectBytes, admissionRequestObject)
var admissionRequestBytes = []byte(`{
	"request": {
		"uid": "foo",
		"requestKind": {
			"group": "foo",
			"version": "v1",
			"kind": "bar"
		},
		"object": ` + admissionRequestObjectBytes.String() + `
	}
}`)
var admissionRequestBytesNoDefaults = []byte(`{
	"request": {
		"uid": "foo",
		"requestKind": {
			"group": "foo",
			"version": "v1",
			"kind": "bar"
		},
		"object": {
			"kind": "Test",
			"apiVersion": "foo/v1",
			"metadata": {
				"creationTimestamp": "2023-07-06T20:49:10Z"
			},
			"spec": {
				"foo": "bar"
			}
		}
	}
}`)

func TestWebhookServer_HandleMutateHTTP(t *testing.T) {
	tests := []struct {
		name               string
		serverConfig       WebhookServerConfig
		reqMethod          string
		payload            []byte
		expectedResponse   []byte
		expectedStatusCode int
	}{
		{
			name:               "HTTP GET",
			reqMethod:          http.MethodGet,
			expectedStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:               "HTTP PUT",
			reqMethod:          http.MethodPut,
			expectedStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:               "HTTP PATCH",
			reqMethod:          http.MethodPatch,
			expectedStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:               "HTTP DELETE",
			reqMethod:          http.MethodDelete,
			expectedStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:               "no default",
			serverConfig:       WebhookServerConfig{},
			reqMethod:          http.MethodPost,
			payload:            admissionRequestBytes,
			expectedResponse:   []byte(`no mutating admission controller defined for group 'foo' and kind 'bar'`),
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "use default",
			serverConfig: WebhookServerConfig{
				DefaultMutatingController: &testMutatingAdmissionController{
					MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
						obj := request.Object
						cmd := obj.GetCommonMetadata()
						cmd.CreatedBy = "me"
						cmd.UpdatedBy = "you"
						cmd.UpdateTimestamp = cmd.CreationTimestamp.UTC()
						obj.SetCommonMetadata(cmd)
						return &resource.MutatingResponse{
							UpdatedObject: obj,
						}, nil
					},
				},
			},
			reqMethod: http.MethodPost,
			payload:   admissionRequestBytesNoDefaults,
			// Patch is base64-encoded
			// [{"op":"add","path":"/metadata/annotations","value":{"grafana.com/createdBy":"me","grafana.com/updateTimestamp":"2023-07-06T16:49:10-04:00","grafana.com/updatedBy":"you"}}] =>
			// W3sib3AiOiJhZGQiLCJwYXRoIjoiL21ldGFkYXRhL2Fubm90YXRpb25zIiwidmFsdWUiOnsiZ3JhZmFuYS5jb20vY3JlYXRlZEJ5IjoibWUiLCJncmFmYW5hLmNvbS91cGRhdGVUaW1lc3RhbXAiOiIyMDIzLTA3LTA2VDE2OjQ5OjEwLTA0OjAwIiwiZ3JhZmFuYS5jb20vdXBkYXRlZEJ5IjoieW91In19XQ==
			expectedResponse:   []byte(`{"response":{"uid":"foo","allowed":true,"patchType":"JSONPatch","patch":"W3sib3AiOiJhZGQiLCJwYXRoIjoiL21ldGFkYXRhL2Fubm90YXRpb25zIiwidmFsdWUiOnsiZ3JhZmFuYS5jb20vY3JlYXRlZEJ5IjoibWUiLCJncmFmYW5hLmNvbS91cGRhdGVUaW1lc3RhbXAiOiIyMDIzLTA3LTA2VDIwOjQ5OjEwWiIsImdyYWZhbmEuY29tL3VwZGF0ZWRCeSI6InlvdSJ9fV0="}}`),
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "use schema-specific",
			serverConfig: WebhookServerConfig{
				DefaultMutatingController: &testMutatingAdmissionController{
					MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
						obj := request.Object.(*TestResourceObject)
						obj.Spec.StringField = "foobar"
						return &resource.MutatingResponse{
							UpdatedObject: obj,
						}, nil
					},
				},
				MutatingControllers: map[*resource.Kind]resource.MutatingAdmissionController{
					&resource.Kind{
						Schema: resource.NewSimpleSchema("foo", "v1", &TestResourceObject{}, &TestResourceObjectList{}, resource.WithKind("bar")),
						Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
					}: &testMutatingAdmissionController{
						MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
							return nil, NewAdmissionError(fmt.Errorf("I AM ERROR"), http.StatusConflict, "err_reason")
						},
					},
				},
			},
			reqMethod:          http.MethodPost,
			payload:            admissionRequestBytes,
			expectedResponse:   []byte(`{"response":{"uid":"foo","allowed":false,"status":{"metadata":{},"status":"Failure","message":"I AM ERROR","reason":"err_reason","code":409}}}`),
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "schema-specific success with patch",
			serverConfig: WebhookServerConfig{
				DefaultMutatingController: &testMutatingAdmissionController{
					MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
						return nil, NewAdmissionError(fmt.Errorf("I AM ERROR"), http.StatusConflict, "err_reason")
					},
				},
				MutatingControllers: map[*resource.Kind]resource.MutatingAdmissionController{
					&resource.Kind{
						Schema: resource.NewSimpleSchema("foo", "v1", &TestResourceObject{}, &TestResourceObjectList{}, resource.WithKind("bar")),
						Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
					}: &testMutatingAdmissionController{
						MutateFunc: func(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
							obj := request.Object.(*TestResourceObject)
							obj.Spec.StringField = "foobar"
							return &resource.MutatingResponse{
								UpdatedObject: obj,
							}, nil
						},
					},
				},
			},
			reqMethod: http.MethodPost,
			payload:   admissionRequestBytes,
			// Patch is base64-encoded
			// [{"op":"replace","path":"/spec/stringField","value":"foobar"}] => W3sib3AiOiJyZXBsYWNlIiwicGF0aCI6Ii9zcGVjL3N0cmluZ0ZpZWxkIiwidmFsdWUiOiJmb29iYXIifV0=
			expectedResponse:   []byte(`{"response":{"uid":"foo","allowed":true,"patchType":"JSONPatch","patch":"W3sib3AiOiJyZXBsYWNlIiwicGF0aCI6Ii9zcGVjL3N0cmluZ0ZpZWxkIiwidmFsdWUiOiJmb29iYXIifV0="}}`),
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "malformed request body: bad JSON",
			reqMethod:          http.MethodPost,
			payload:            []byte("{"),
			expectedStatusCode: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := test.serverConfig
			cfg.TLSConfig = TLSConfig{
				CertPath: "foo",
				KeyPath:  "bar",
			}
			cfg.Port = 8443
			srv, err := NewWebhookServer(cfg)
			require.Nil(t, err)
			req := httptest.NewRequest(test.reqMethod, "http://localhost/mutate", bytes.NewBuffer(test.payload))
			resp := httptest.NewRecorder()
			srv.HandleMutateHTTP(resp, req)

			if test.expectedStatusCode == http.StatusOK {
				assert.JSONEq(t, string(test.expectedResponse), resp.Body.String())
			} else {
				assert.Equal(t, test.expectedResponse, resp.Body.Bytes())
			}
			assert.Equal(t, test.expectedStatusCode, resp.Code)
		})
	}
}

func TestWebhookServer_HandleValidateHTTP(t *testing.T) {
	tests := []struct {
		name               string
		serverConfig       WebhookServerConfig
		reqMethod          string
		payload            []byte
		expectedResponse   []byte
		expectedStatusCode int
	}{
		{
			name:               "HTTP GET",
			reqMethod:          http.MethodGet,
			expectedStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:               "HTTP PUT",
			reqMethod:          http.MethodPut,
			expectedStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:               "HTTP PATCH",
			reqMethod:          http.MethodPatch,
			expectedStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:               "HTTP DELETE",
			reqMethod:          http.MethodDelete,
			expectedStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:               "no default",
			serverConfig:       WebhookServerConfig{},
			reqMethod:          http.MethodPost,
			payload:            admissionRequestBytes,
			expectedResponse:   []byte(`no validating admission controller defined for group 'foo' and kind 'bar'`),
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "use default",
			serverConfig: WebhookServerConfig{
				DefaultValidatingController: &testValidatingAdmissionController{
					ValidateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
						return NewAdmissionError(fmt.Errorf("I AM ERROR"), http.StatusConflict, "err_reason")
					},
				},
			},
			reqMethod:          http.MethodPost,
			payload:            admissionRequestBytes,
			expectedResponse:   []byte(`{"response":{"uid":"foo","allowed":false,"status":{"metadata":{},"status":"Failure","message":"I AM ERROR","reason":"err_reason","code":409}}}`),
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "use schema-specific",
			serverConfig: WebhookServerConfig{
				DefaultValidatingController: &testValidatingAdmissionController{
					ValidateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
						return NewAdmissionError(fmt.Errorf("I AM ERROR"), http.StatusConflict, "err_reason")
					},
				},
				ValidatingControllers: map[*resource.Kind]resource.ValidatingAdmissionController{
					&resource.Kind{
						Schema: resource.NewSimpleSchema("foo", "v1", &TestResourceObject{}, &TestResourceObjectList{}, resource.WithKind("bar")),
						Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
					}: &testValidatingAdmissionController{
						ValidateFunc: func(ctx context.Context, request *resource.AdmissionRequest) error {
							return nil
						},
					},
				},
			},
			reqMethod:          http.MethodPost,
			payload:            admissionRequestBytes,
			expectedResponse:   []byte(`{"response":{"uid":"foo","allowed":true}}`),
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "malformed request body: bad JSON",
			reqMethod:          http.MethodPost,
			payload:            []byte("{"),
			expectedStatusCode: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := test.serverConfig
			cfg.TLSConfig = TLSConfig{
				CertPath: "foo",
				KeyPath:  "bar",
			}
			cfg.Port = 8443
			srv, err := NewWebhookServer(cfg)
			require.Nil(t, err)
			req := httptest.NewRequest(test.reqMethod, "http://localhost/validate", bytes.NewBuffer(test.payload))
			resp := httptest.NewRecorder()
			srv.HandleValidateHTTP(resp, req)

			if test.expectedStatusCode == http.StatusOK {
				assert.JSONEq(t, string(test.expectedResponse), resp.Body.String())
			} else {
				assert.Equal(t, test.expectedResponse, resp.Body.Bytes())
			}
			assert.Equal(t, test.expectedStatusCode, resp.Code)
		})
	}
}

type testValidatingAdmissionController struct {
	ValidateFunc func(context.Context, *resource.AdmissionRequest) error
}

func (tvac *testValidatingAdmissionController) Validate(ctx context.Context, request *resource.AdmissionRequest) error {
	if tvac.ValidateFunc != nil {
		return tvac.ValidateFunc(ctx, request)
	}
	return nil
}

type testMutatingAdmissionController struct {
	MutateFunc func(context.Context, *resource.AdmissionRequest) (*resource.MutatingResponse, error)
}

func (tmac *testMutatingAdmissionController) Mutate(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
	if tmac.MutateFunc != nil {
		return tmac.MutateFunc(ctx, request)
	}
	return nil, nil
}
