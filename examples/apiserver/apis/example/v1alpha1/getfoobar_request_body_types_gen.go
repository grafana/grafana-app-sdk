// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// Test type for go naming conflicts
type GetFoobarRequestSharedType struct {
	Bar string                          `json:"bar"`
	Dep []GetFoobarRequestSharedTypeDep `json:"dep"`
}

// NewGetFoobarRequestSharedType creates a new GetFoobarRequestSharedType object.
func NewGetFoobarRequestSharedType() *GetFoobarRequestSharedType {
	return &GetFoobarRequestSharedType{
		Dep: []GetFoobarRequestSharedTypeDep{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for GetFoobarRequestSharedType.
func (GetFoobarRequestSharedType) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetFoobarRequestSharedType"
}

type GetFoobarRequestSharedTypeDep struct {
	Value string `json:"value"`
}

// NewGetFoobarRequestSharedTypeDep creates a new GetFoobarRequestSharedTypeDep object.
func NewGetFoobarRequestSharedTypeDep() *GetFoobarRequestSharedTypeDep {
	return &GetFoobarRequestSharedTypeDep{}
}

// OpenAPIModelName returns the OpenAPI model name for GetFoobarRequestSharedTypeDep.
func (GetFoobarRequestSharedTypeDep) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetFoobarRequestSharedTypeDep"
}

type GetFoobarRequestBody struct {
	Input  string                     `json:"input"`
	Shared GetFoobarRequestSharedType `json:"shared"`
}

// NewGetFoobarRequestBody creates a new GetFoobarRequestBody object.
func NewGetFoobarRequestBody() *GetFoobarRequestBody {
	return &GetFoobarRequestBody{
		Shared: *NewGetFoobarRequestSharedType(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for GetFoobarRequestBody.
func (GetFoobarRequestBody) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetFoobarRequestBody"
}
