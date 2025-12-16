// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
    "github.com/grafana/grafana-app-sdk/resource"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type GetFoobarRequestParamsObject struct {
    metav1.TypeMeta `json:",inline"`
    GetFoobarRequestParams `json:",inline"`
}

func NewGetFoobarRequestParamsObject() *GetFoobarRequestParamsObject {
    return &GetFoobarRequestParamsObject{}
}

func (o *GetFoobarRequestParamsObject) DeepCopyObject() runtime.Object {
    dst := NewGetFoobarRequestParamsObject()
    o.DeepCopyInto(dst)
    return dst
}

func (o *GetFoobarRequestParamsObject) DeepCopyInto(dst *GetFoobarRequestParamsObject) {
    dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
    dst.TypeMeta.Kind = o.TypeMeta.Kind
    dstGetFoobarRequestParams := GetFoobarRequestParams{}
    _ = resource.CopyObjectInto(&dstGetFoobarRequestParams, &o.GetFoobarRequestParams)
}

var _ runtime.Object = NewGetFoobarRequestParamsObject()