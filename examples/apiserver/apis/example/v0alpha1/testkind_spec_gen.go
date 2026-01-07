// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v0alpha1

// +k8s:openapi-gen=true
type TestKindSpec struct {
	TestField int64 `json:"testField"`
}

// NewTestKindSpec creates a new TestKindSpec object.
func NewTestKindSpec() *TestKindSpec {
	return &TestKindSpec{}
}
func (TestKindSpec) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v0alpha1.TestKindSpec"
}
