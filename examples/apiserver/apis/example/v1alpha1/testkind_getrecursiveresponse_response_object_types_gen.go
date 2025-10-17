// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetRecursiveResponse struct {
	metav1.TypeMeta          `json:",inline"`
	GetRecursiveResponseBody `json:",inline"`
}

func NewGetRecursiveResponse() *GetRecursiveResponse {
	return &GetRecursiveResponse{}
}

func (t *GetRecursiveResponseBody) DeepCopyInto(dst *GetRecursiveResponseBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetRecursiveResponse) DeepCopyObject() runtime.Object {
	dst := NewGetRecursiveResponse()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetRecursiveResponse) DeepCopyInto(dst *GetRecursiveResponse) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetRecursiveResponseBody.DeepCopyInto(&dst.GetRecursiveResponseBody)
}

var _ runtime.Object = NewGetRecursiveResponse()
