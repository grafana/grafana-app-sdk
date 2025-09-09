// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	// +k8s:openapi-gen=true
	"github.com/grafana/grafana-app-sdk/resource"
)

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

func (b *GetFooBody) DeepCopyInto(dst *GetFooBody) {
	resource.CopyObjectInto(dst, b)
}

func (o *GetFoo) DeepCopyObject() runtime.Object {
	dst := NewGetFoo()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetFoo) DeepCopyInto(dst *GetFoo) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.ObjectMeta.DeepCopyInto(&dst.ObjectMeta)
	o.GetFooBody.DeepCopyInto(&dst.GetFooBody)
}

var _ runtime.Object = &GetFoo{}
