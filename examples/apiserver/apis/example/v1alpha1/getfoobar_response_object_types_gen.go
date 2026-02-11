// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetFoobarResponse struct {
	metav1.TypeMeta `json:",inline"`
	GetFoobarBody   `json:",inline"`
}

func NewGetFoobarResponse() *GetFoobarResponse {
	return &GetFoobarResponse{}
}

func (t *GetFoobarBody) DeepCopyInto(dst *GetFoobarBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetFoobarResponse) DeepCopyObject() runtime.Object {
	dst := NewGetFoobarResponse()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetFoobarResponse) DeepCopyInto(dst *GetFoobarResponse) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetFoobarBody.DeepCopyInto(&dst.GetFoobarBody)
}

func (GetFoobarResponse) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetFoobarResponse"
}

var _ runtime.Object = NewGetFoobarResponse()
