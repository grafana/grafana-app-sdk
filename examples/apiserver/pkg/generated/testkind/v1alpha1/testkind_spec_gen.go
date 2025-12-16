// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type Foo struct {
	Foo string                    `json:"foo"`
	Bar TestKindv1alpha1SchemaBar `json:"bar"`
}

// NewFoo creates a new Foo object.
func NewFoo() *Foo {
	return &Foo{
		Foo: "foo",
		Bar: *NewTestKindv1alpha1SchemaBar(),
	}
}

// +k8s:openapi-gen=true
type TestKindv1alpha1SchemaBar struct {
	Value string                    `json:"value"`
	Baz   TestKindv1alpha1SchemaBaz `json:"baz"`
}

// NewTestKindv1alpha1SchemaBar creates a new TestKindv1alpha1SchemaBar object.
func NewTestKindv1alpha1SchemaBar() *TestKindv1alpha1SchemaBar {
	return &TestKindv1alpha1SchemaBar{
		Value: "bar",
		Baz:   *NewTestKindv1alpha1SchemaBaz(),
	}
}

// +k8s:openapi-gen=true
type TestKindv1alpha1SchemaBaz struct {
	Value int64 `json:"value"`
}

// NewTestKindv1alpha1SchemaBaz creates a new TestKindv1alpha1SchemaBaz object.
func NewTestKindv1alpha1SchemaBaz() *TestKindv1alpha1SchemaBaz {
	return &TestKindv1alpha1SchemaBaz{
		Value: 10,
	}
}

// +k8s:openapi-gen=true
type Spec struct {
	TestField string `json:"testField"`
	Foo       Foo    `json:"foo"`
}

// NewSpec creates a new Spec object.
func NewSpec() *Spec {
	return &Spec{
		TestField: "default value",
		Foo:       *NewFoo(),
	}
}
