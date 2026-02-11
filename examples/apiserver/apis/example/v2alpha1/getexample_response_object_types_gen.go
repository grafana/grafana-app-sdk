// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v2alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetExampleResponse struct {
	metav1.TypeMeta `json:",inline"`
	GetExampleBody  `json:",inline"`
}

func NewGetExampleResponse() *GetExampleResponse {
	return &GetExampleResponse{}
}

func (t *GetExampleBody) DeepCopyInto(dst *GetExampleBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetExampleResponse) DeepCopyObject() runtime.Object {
	dst := NewGetExampleResponse()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetExampleResponse) DeepCopyInto(dst *GetExampleResponse) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetExampleBody.DeepCopyInto(&dst.GetExampleBody)
}

func (GetExampleResponse) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v2alpha1.GetExampleResponse"
}

var _ runtime.Object = NewGetExampleResponse()
