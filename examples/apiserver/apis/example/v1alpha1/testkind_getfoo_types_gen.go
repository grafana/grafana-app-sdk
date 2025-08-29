// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:openapi-gen=true
type GetFooBody struct {
	Status string `json:"status"`
}

// NewGetFooBody creates a new GetFooBody object.
func NewGetFooBody() *GetFooBody {
	return &GetFooBody{}
}

type GetFoo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	GetFooBody        `json:",inline"`
}

func NewGetFoo() *GetFoo {
	return &GetFoo{}
}
