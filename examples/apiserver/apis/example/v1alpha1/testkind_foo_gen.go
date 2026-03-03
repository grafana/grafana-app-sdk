// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// If we don't include the Subresource prefix this will generate conflicting types with the #Foo type used in spec
// +k8s:openapi-gen=true
type TestKindSubresourceFoo struct {
	SomeVal string `json:"someVal"`
}

// NewTestKindSubresourceFoo creates a new TestKindSubresourceFoo object.
func NewTestKindSubresourceFoo() *TestKindSubresourceFoo {
	return &TestKindSubresourceFoo{}
}

// OpenAPIModelName returns the OpenAPI model name for TestKindSubresourceFoo.
func (TestKindSubresourceFoo) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.TestKindSubresourceFoo"
}
