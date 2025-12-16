// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// Test type for go naming conflicts
type GetFoobarRequestSharedType struct {
	Bar string `json:"bar"`
}

// NewGetFoobarRequestSharedType creates a new GetFoobarRequestSharedType object.
func NewGetFoobarRequestSharedType() *GetFoobarRequestSharedType {
	return &GetFoobarRequestSharedType{}
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
