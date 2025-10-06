// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type GetClusterFoobar struct {
	metav1.TypeMeta      `json:",inline"`
	GetClusterFoobarBody `json:",inline"`
}

func NewGetClusterFoobar() *GetClusterFoobar {
	return &GetClusterFoobar{}
}

func (t *GetClusterFoobarBody) DeepCopyInto(dst *GetClusterFoobarBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *GetClusterFoobar) DeepCopyObject() runtime.Object {
	dst := NewGetClusterFoobar()
	o.DeepCopyInto(dst)
	return dst
}

func (o *GetClusterFoobar) DeepCopyInto(dst *GetClusterFoobar) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.GetClusterFoobarBody.DeepCopyInto(&dst.GetClusterFoobarBody)
}

var _ runtime.Object = NewGetClusterFoobar()
