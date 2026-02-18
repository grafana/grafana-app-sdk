// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type GetRecursiveResponseRecursiveType struct {
	Message string                             `json:"message"`
	Next    *GetRecursiveResponseRecursiveType `json:"next,omitempty"`
}

// NewGetRecursiveResponseRecursiveType creates a new GetRecursiveResponseRecursiveType object.
func NewGetRecursiveResponseRecursiveType() *GetRecursiveResponseRecursiveType {
	return &GetRecursiveResponseRecursiveType{}
}

// OpenAPIModelName returns the OpenAPI model name for GetRecursiveResponseRecursiveType.
func (GetRecursiveResponseRecursiveType) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetRecursiveResponseRecursiveType"
}

// +k8s:openapi-gen=true
type GetRecursiveResponseBody struct {
	Message string                             `json:"message"`
	Next    *GetRecursiveResponseRecursiveType `json:"next,omitempty"`
}

// NewGetRecursiveResponseBody creates a new GetRecursiveResponseBody object.
func NewGetRecursiveResponseBody() *GetRecursiveResponseBody {
	return &GetRecursiveResponseBody{}
}

// OpenAPIModelName returns the OpenAPI model name for GetRecursiveResponseBody.
func (GetRecursiveResponseBody) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetRecursiveResponseBody"
}
