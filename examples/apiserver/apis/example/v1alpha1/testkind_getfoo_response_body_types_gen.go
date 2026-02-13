// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type GetFooBody struct {
	Status string `json:"status"`
}

// NewGetFooBody creates a new GetFooBody object.
func NewGetFooBody() *GetFooBody {
	return &GetFooBody{}
}

// OpenAPIModelName returns the OpenAPI model name for GetFooBody.
func (GetFooBody) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetFooBody"
}
