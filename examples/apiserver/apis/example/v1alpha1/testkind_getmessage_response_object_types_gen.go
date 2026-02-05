// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetMessageResponse struct {
	metav1.TypeMeta `json:",inline"`
	GetMessageBody  `json:",inline"`
}

func NewGetMessageResponse() *GetMessageResponse {
	return &GetMessageResponse{}
}

func (t *GetMessageBody) DeepCopyInto(dst *GetMessageBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetMessageResponse) DeepCopyObject() runtime.Object {
	dst := NewGetMessageResponse()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetMessageResponse) DeepCopyInto(dst *GetMessageResponse) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetMessageBody.DeepCopyInto(&dst.GetMessageBody)
}

func (GetMessageResponse) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetMessageResponse"
}

var _ runtime.Object = NewGetMessageResponse()
