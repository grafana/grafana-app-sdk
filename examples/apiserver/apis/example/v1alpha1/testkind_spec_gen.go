// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type TestKindFoo struct {
	Foo string      `json:"foo"`
	Bar TestKindBar `json:"bar"`
}

// NewTestKindFoo creates a new TestKindFoo object.
func NewTestKindFoo() *TestKindFoo {
	return &TestKindFoo{
		Foo: "foo",
		Bar: *NewTestKindBar(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for TestKindFoo.
func (TestKindFoo) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.TestKindFoo"
}

// +k8s:openapi-gen=true
type TestKindBar struct {
	Value string      `json:"value"`
	Baz   TestKindBaz `json:"baz"`
}

// NewTestKindBar creates a new TestKindBar object.
func NewTestKindBar() *TestKindBar {
	return &TestKindBar{
		Value: "bar",
		Baz:   *NewTestKindBaz(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for TestKindBar.
func (TestKindBar) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.TestKindBar"
}

// +k8s:openapi-gen=true
type TestKindBaz struct {
	Value int64 `json:"value"`
}

// NewTestKindBaz creates a new TestKindBaz object.
func NewTestKindBaz() *TestKindBaz {
	return &TestKindBaz{
		Value: 10,
	}
}

// OpenAPIModelName returns the OpenAPI model name for TestKindBaz.
func (TestKindBaz) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.TestKindBaz"
}

// +k8s:openapi-gen=true
type TestKindSpec struct {
	TestField string      `json:"testField"`
	Foo       TestKindFoo `json:"foo"`
}

// NewTestKindSpec creates a new TestKindSpec object.
func NewTestKindSpec() *TestKindSpec {
	return &TestKindSpec{
		TestField: "default value",
		Foo:       *NewTestKindFoo(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for TestKindSpec.
func (TestKindSpec) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.TestKindSpec"
}
