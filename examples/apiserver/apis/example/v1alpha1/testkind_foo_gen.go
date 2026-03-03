// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type TestKindSubresourceFooFoo struct {
	Foo string                    `json:"foo"`
	Bar TestKindSubresourceFooBar `json:"bar"`
}

// NewTestKindSubresourceFooFoo creates a new TestKindSubresourceFooFoo object.
func NewTestKindSubresourceFooFoo() *TestKindSubresourceFooFoo {
	return &TestKindSubresourceFooFoo{
		Foo: "foo",
		Bar: *NewTestKindSubresourceFooBar(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for TestKindSubresourceFooFoo.
func (TestKindSubresourceFooFoo) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.TestKindSubresourceFooFoo"
}

// +k8s:openapi-gen=true
type TestKindSubresourceFooBar struct {
	Value string                    `json:"value"`
	Baz   TestKindSubresourceFooBaz `json:"baz"`
}

// NewTestKindSubresourceFooBar creates a new TestKindSubresourceFooBar object.
func NewTestKindSubresourceFooBar() *TestKindSubresourceFooBar {
	return &TestKindSubresourceFooBar{
		Value: "bar",
		Baz:   *NewTestKindSubresourceFooBaz(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for TestKindSubresourceFooBar.
func (TestKindSubresourceFooBar) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.TestKindSubresourceFooBar"
}

// +k8s:openapi-gen=true
type TestKindSubresourceFooBaz struct {
	Value int64 `json:"value"`
}

// NewTestKindSubresourceFooBaz creates a new TestKindSubresourceFooBaz object.
func NewTestKindSubresourceFooBaz() *TestKindSubresourceFooBaz {
	return &TestKindSubresourceFooBaz{
		Value: 10,
	}
}

// OpenAPIModelName returns the OpenAPI model name for TestKindSubresourceFooBaz.
func (TestKindSubresourceFooBaz) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.TestKindSubresourceFooBaz"
}

// If we don't include the Subresource prefix this will generate conflicting types with the #Foo type used in spec
// +k8s:openapi-gen=true
type TestKindSubresourceFoo struct {
	SomeVal string                    `json:"someVal"`
	Bar     TestKindSubresourceFooFoo `json:"bar"`
}

// NewTestKindSubresourceFoo creates a new TestKindSubresourceFoo object.
func NewTestKindSubresourceFoo() *TestKindSubresourceFoo {
	return &TestKindSubresourceFoo{
		Bar: *NewTestKindSubresourceFooFoo(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for TestKindSubresourceFoo.
func (TestKindSubresourceFoo) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.TestKindSubresourceFoo"
}
