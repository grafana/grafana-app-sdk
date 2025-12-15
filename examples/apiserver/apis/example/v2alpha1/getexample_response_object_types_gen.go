// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v2alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetExample struct {
	metav1.TypeMeta `json:",inline"`
	GetExampleBody  `json:",inline"`
}

func NewGetExample() *GetExample {
	return &GetExample{}
}

func (t *GetExampleBody) DeepCopyInto(dst *GetExampleBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetExample) DeepCopyObject() runtime.Object {
	dst := NewGetExample()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetExample) DeepCopyInto(dst *GetExample) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetExampleBody.DeepCopyInto(&dst.GetExampleBody)
}

var _ runtime.Object = NewGetExample()
