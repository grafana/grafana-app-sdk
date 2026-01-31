// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// Test type for go naming conflicts
// +k8s:openapi-gen=true
type GetFoobarSharedType struct {
	Bar string                   `json:"bar"`
	Dep []GetFoobarSharedTypeDep `json:"dep"`
}

// NewGetFoobarSharedType creates a new GetFoobarSharedType object.
func NewGetFoobarSharedType() *GetFoobarSharedType {
	return &GetFoobarSharedType{
		Dep: []GetFoobarSharedTypeDep{},
	}
}

// +k8s:openapi-gen=true
type GetFoobarSharedTypeDep struct {
	Value string `json:"value"`
}

// NewGetFoobarSharedTypeDep creates a new GetFoobarSharedTypeDep object.
func NewGetFoobarSharedTypeDep() *GetFoobarSharedTypeDep {
	return &GetFoobarSharedTypeDep{}
}

// +k8s:openapi-gen=true
type GetFoobarBody struct {
	Foo    string              `json:"foo"`
	Shared GetFoobarSharedType `json:"shared"`
}

// NewGetFoobarBody creates a new GetFoobarBody object.
func NewGetFoobarBody() *GetFoobarBody {
	return &GetFoobarBody{
		Shared: *NewGetFoobarSharedType(),
	}
}
func (GetFoobarSharedType) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetFoobarSharedType"
}
func (GetFoobarSharedTypeDep) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetFoobarSharedTypeDep"
}
func (GetFoobarBody) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetFoobarBody"
}
