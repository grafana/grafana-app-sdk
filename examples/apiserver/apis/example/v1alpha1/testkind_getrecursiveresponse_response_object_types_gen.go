// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetRecursiveResponseResponse struct {
	metav1.TypeMeta          `json:",inline"`
	GetRecursiveResponseBody `json:",inline"`
}

func NewGetRecursiveResponseResponse() *GetRecursiveResponseResponse {
	return &GetRecursiveResponseResponse{}
}

func (t *GetRecursiveResponseBody) DeepCopyInto(dst *GetRecursiveResponseBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetRecursiveResponseResponse) DeepCopyObject() runtime.Object {
	dst := NewGetRecursiveResponseResponse()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetRecursiveResponseResponse) DeepCopyInto(dst *GetRecursiveResponseResponse) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetRecursiveResponseBody.DeepCopyInto(&dst.GetRecursiveResponseBody)
}

func (GetRecursiveResponseResponse) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetRecursiveResponseResponse"
}

var _ runtime.Object = NewGetRecursiveResponseResponse()
