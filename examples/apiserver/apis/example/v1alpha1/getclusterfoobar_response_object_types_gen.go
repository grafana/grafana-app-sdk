// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetClusterFoobarResponse struct {
	metav1.TypeMeta      `json:",inline"`
	GetClusterFoobarBody `json:",inline"`
}

func NewGetClusterFoobarResponse() *GetClusterFoobarResponse {
	return &GetClusterFoobarResponse{}
}

func (t *GetClusterFoobarBody) DeepCopyInto(dst *GetClusterFoobarBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetClusterFoobarResponse) DeepCopyObject() runtime.Object {
	dst := NewGetClusterFoobarResponse()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetClusterFoobarResponse) DeepCopyInto(dst *GetClusterFoobarResponse) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetClusterFoobarBody.DeepCopyInto(&dst.GetClusterFoobarBody)
}

func (GetClusterFoobarResponse) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetClusterFoobarResponse"
}

var _ runtime.Object = NewGetClusterFoobarResponse()
