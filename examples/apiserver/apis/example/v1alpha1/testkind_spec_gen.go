// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type TestKindFooBar struct {
	Foo string          `json:"foo"`
	Bar *TestKindFooBar `json:"bar,omitempty"`
}

// NewTestKindFooBar creates a new TestKindFooBar object.
func NewTestKindFooBar() *TestKindFooBar {
	return &TestKindFooBar{}
}

// +k8s:openapi-gen=true
type TestKindSpec struct {
	TestField string          `json:"testField"`
	Foobar    *TestKindFooBar `json:"foobar,omitempty"`
}

// NewTestKindSpec creates a new TestKindSpec object.
func NewTestKindSpec() *TestKindSpec {
	return &TestKindSpec{
		TestField: "foobar",
	}
}
