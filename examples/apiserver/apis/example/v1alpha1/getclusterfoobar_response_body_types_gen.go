// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type GetClusterFoobarExtra struct {
	Foo string `json:"foo"`
}

// NewGetClusterFoobarExtra creates a new GetClusterFoobarExtra object.
func NewGetClusterFoobarExtra() *GetClusterFoobarExtra {
	return &GetClusterFoobarExtra{}
}

// OpenAPIModelName returns the OpenAPI model name for GetClusterFoobarExtra.
func (GetClusterFoobarExtra) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetClusterFoobarExtra"
}

// +k8s:openapi-gen=true
type GetClusterFoobarBody struct {
	Bar   string                           `json:"bar"`
	Extra map[string]GetClusterFoobarExtra `json:"extra"`
}

// NewGetClusterFoobarBody creates a new GetClusterFoobarBody object.
func NewGetClusterFoobarBody() *GetClusterFoobarBody {
	return &GetClusterFoobarBody{
		Extra: map[string]GetClusterFoobarExtra{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for GetClusterFoobarBody.
func (GetClusterFoobarBody) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetClusterFoobarBody"
}
