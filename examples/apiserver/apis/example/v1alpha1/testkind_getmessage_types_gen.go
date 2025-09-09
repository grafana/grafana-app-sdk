// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	// +k8s:openapi-gen=true
	"github.com/grafana/grafana-app-sdk/resource"
)

type GetMessageBody struct {
	Message string `json:"message"`
}

// NewGetMessageBody creates a new GetMessageBody object.
func NewGetMessageBody() *GetMessageBody {
	return &GetMessageBody{}
}

type GetMessage struct {
	metav1.TypeMeta `json:",inline"`
	GetMessageBody  `json:",inline"`
}

func NewGetMessage() *GetMessage {
	return &GetMessage{}
}

func (b *GetMessageBody) DeepCopyInto(dst *GetMessageBody) {
	resource.CopyObjectInto(dst, b)
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

var _ runtime.Object = &GetMessage{}
