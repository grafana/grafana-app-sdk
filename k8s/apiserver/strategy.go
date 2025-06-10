package apiserver

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/grafana/grafana-app-sdk/resource"
)

type strategy struct {
	ObjectTyper runtime.ObjectTyper
	kind        resource.Kind
}

func newStrategy(scheme *runtime.Scheme, kind resource.Kind) *strategy {
	return &strategy{
		ObjectTyper: scheme,
		kind:        kind,
	}
}

func (s *strategy) NamespaceScoped() bool {
	return s.kind.Scope() == resource.NamespacedScope
}

func (*strategy) GenerateName(base string) string {
	return fmt.Sprintf("%s-", base)
}

func (*strategy) PrepareForCreate(_ context.Context, _ runtime.Object) {
}

func (*strategy) Validate(_ context.Context, _ runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

func (*strategy) Canonicalize(_ runtime.Object) {
}

func (*strategy) AllowCreateOnUpdate() bool {
	return false
}

func (*strategy) WarningsOnCreate(_ context.Context, _ runtime.Object) []string {
	return nil
}

func (*strategy) PrepareForUpdate(_ context.Context, _, _ runtime.Object) {
}

func (*strategy) ValidateUpdate(_ context.Context, _, _ runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

func (*strategy) AllowUnconditionalUpdate() bool {
	return false
}

func (*strategy) WarningsOnUpdate(_ context.Context, _, _ runtime.Object) []string {
	return nil
}

func (*strategy) PrepareForDelete(_ context.Context, _ runtime.Object) {
}

func (*strategy) WarningsOnDelete(_ context.Context, _ runtime.Object) []string {
	return nil
}

func (s *strategy) ObjectKinds(obj runtime.Object) ([]schema.GroupVersionKind, bool, error) {
	return s.ObjectTyper.ObjectKinds(obj)
}

func (s *strategy) Recognizes(gvk schema.GroupVersionKind) bool {
	return gvk == s.kind.GroupVersionKind()
}

func (*strategy) CheckGracefulDelete(_ context.Context, _ runtime.Object, _ *metav1.DeleteOptions) bool {
	return false
}

var _ rest.Scoper = &strategy{}
var _ rest.RESTCreateStrategy = &strategy{}
var _ rest.RESTUpdateStrategy = &strategy{}
var _ rest.RESTDeleteStrategy = &strategy{}
var _ rest.RESTGracefulDeleteStrategy = &strategy{}
