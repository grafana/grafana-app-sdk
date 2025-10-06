// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// Test type for go naming conflicts
// +k8s:openapi-gen=true
type SharedType struct {
	Bar string `json:"bar"`
}

// NewSharedType creates a new SharedType object.
func NewSharedType() *SharedType {
	return &SharedType{}
}

// +k8s:openapi-gen=true
type GetFoobarBody struct {
	Foo    string     `json:"foo"`
	Shared SharedType `json:"shared"`
}

// NewGetFoobarBody creates a new GetFoobarBody object.
func NewGetFoobarBody() *GetFoobarBody {
	return &GetFoobarBody{
		Shared: *NewSharedType(),
	}
}
