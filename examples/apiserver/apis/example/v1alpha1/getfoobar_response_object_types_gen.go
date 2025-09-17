// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetFoobar struct {
	metav1.TypeMeta `json:",inline"`
	GetFoobarBody   `json:",inline"`
}

func NewGetFoobar() *GetFoobar {
	return &GetFoobar{}
}

func (t *GetFoobarBody) DeepCopyInto(dst *GetFoobarBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetFoobar) DeepCopyObject() runtime.Object {
	dst := NewGetFoobar()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetFoobar) DeepCopyInto(dst *GetFoobar) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetFoobarBody.DeepCopyInto(&dst.GetFoobarBody)
}

var _ runtime.Object = NewGetFoobar()
