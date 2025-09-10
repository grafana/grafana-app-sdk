// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetMessage struct {
	metav1.TypeMeta `json:",inline"`
	GetMessageBody  `json:",inline"`
}

func NewGetMessage() *GetMessage {
	return &GetMessage{}
}

func (t *GetMessageBody) DeepCopyInto(dst *GetMessageBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetMessage) DeepCopyObject() runtime.Object {
	dst := NewGetMessage()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetMessage) DeepCopyInto(dst *GetMessage) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetMessageBody.DeepCopyInto(&dst.GetMessageBody)
}

var _ runtime.Object = NewGetMessage()
