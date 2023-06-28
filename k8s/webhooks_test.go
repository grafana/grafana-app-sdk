package k8s

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWebhookServer(t *testing.T) {

}

func TestWebhookServer_AddMutatingAdmissionController(t *testing.T) {

}

func TestWebhookServer_AddValidatingAdmissionController(t *testing.T) {

}

var admissionRequestObject = &TestResourceObject{}
var admissionRequestObjectBytes, _ = marshalJSON(admissionRequestObject, nil, ClientConfig{})
var admissionRequestBytes = []byte(`{
	"request": {
		"uid": "foo",
		"requestKind": {
			"group": "foo",
			"version": "v1",
			"kind": "bar"
		},
		"object": ` + string(admissionRequestObjectBytes) + `
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
					MutateFunc: func(request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
						return &resource.MutatingResponse{
							PatchOperations: []resource.PatchOperation{
								{
									Path:      "/foo/bar",
									Operation: resource.PatchOpReplace,
									Value:     "foobar",
								},
							},
						}, nil
					},
				},
			},
			reqMethod: http.MethodPost,
			payload:   admissionRequestBytes,
			// Patch is base64-encoded
			// [{"path":"/foo/bar","op":"replace","value":"foobar"}] => W3sicGF0aCI6Ii9mb28vYmFyIiwib3AiOiJyZXBsYWNlIiwidmFsdWUiOiJmb29iYXIifV0=
			expectedResponse:   []byte(`{"response":{"uid":"foo","allowed":true,"patchType":"JSONPatch","patch":"W3sicGF0aCI6Ii9mb28vYmFyIiwib3AiOiJyZXBsYWNlIiwidmFsdWUiOiJmb29iYXIifV0="}}`),
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "use schema-specific",
			serverConfig: WebhookServerConfig{
				DefaultMutatingController: &testMutatingAdmissionController{
					MutateFunc: func(request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
						return &resource.MutatingResponse{
							PatchOperations: []resource.PatchOperation{
								{
									Path:      "/foo/bar",
									Operation: resource.PatchOpReplace,
									Value:     "foobar",
								},
							},
						}, nil
					},
				},
				MutatingControllers: map[resource.Schema]resource.MutatingAdmissionController{
					resource.NewSimpleSchema("foo", "v1", &TestResourceObject{}, resource.WithKind("bar")): &testMutatingAdmissionController{
						MutateFunc: func(request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
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
					ValidateFunc: func(request *resource.AdmissionRequest) error {
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
					ValidateFunc: func(request *resource.AdmissionRequest) error {
						return NewAdmissionError(fmt.Errorf("I AM ERROR"), http.StatusConflict, "err_reason")
					},
				},
				ValidatingControllers: map[resource.Schema]resource.ValidatingAdmissionController{
					resource.NewSimpleSchema("foo", "v1", &TestResourceObject{}, resource.WithKind("bar")): &testValidatingAdmissionController{
						ValidateFunc: func(request *resource.AdmissionRequest) error {
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

func TestWebhookServer_Run(t *testing.T) {

}

type testValidatingAdmissionController struct {
	ValidateFunc func(*resource.AdmissionRequest) error
}

func (tvac *testValidatingAdmissionController) Validate(request *resource.AdmissionRequest) error {
	if tvac.ValidateFunc != nil {
		return tvac.ValidateFunc(request)
	}
	return nil
}

type testMutatingAdmissionController struct {
	MutateFunc func(*resource.AdmissionRequest) (*resource.MutatingResponse, error)
}

func (tmac *testMutatingAdmissionController) Mutate(request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
	if tmac.MutateFunc != nil {
		return tmac.MutateFunc(request)
	}
	return nil, nil
}
