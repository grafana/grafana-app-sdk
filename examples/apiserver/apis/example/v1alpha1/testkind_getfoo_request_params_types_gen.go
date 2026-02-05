// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

type GetFooRequestParams struct {
	Foo string `json:"foo"`
}

// NewGetFooRequestParams creates a new GetFooRequestParams object.
func NewGetFooRequestParams() *GetFooRequestParams {
	return &GetFooRequestParams{}
}

// OpenAPIModelName returns the OpenAPI model name for GetFooRequestParams.
func (GetFooRequestParams) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetFooRequestParams"
}
