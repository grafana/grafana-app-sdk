// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:openapi-gen=true
type Clustergetfoobar struct {
	metav1.TypeMeta      `json:",inline"`
	ClustergetfoobarBody `json:",inline"`
}

func NewClustergetfoobar() *Clustergetfoobar {
	return &Clustergetfoobar{}
}

func (t *ClustergetfoobarBody) DeepCopyInto(dst *ClustergetfoobarBody) {
	_ = resource.CopyObjectInto(dst, t)
}

func (o *Clustergetfoobar) DeepCopyObject() runtime.Object {
	dst := NewClustergetfoobar()
	o.DeepCopyInto(dst)
	return dst
}

func (o *Clustergetfoobar) DeepCopyInto(dst *Clustergetfoobar) {
	dst.TypeMeta.APIVersion = o.TypeMeta.APIVersion
	dst.TypeMeta.Kind = o.TypeMeta.Kind
	o.ClustergetfoobarBody.DeepCopyInto(&dst.ClustergetfoobarBody)
}

var _ runtime.Object = NewClustergetfoobar()
