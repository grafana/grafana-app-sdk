package apiserver

import (
	"context"
	"fmt"

	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
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

func (s *strategy) GenerateName(base string) string {
	return fmt.Sprintf("%s-", base)
}

func (s *strategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
}

func (s *strategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

func (s *strategy) Canonicalize(obj runtime.Object) {
}

func (s *strategy) AllowCreateOnUpdate() bool {
	return false
}

func (s *strategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return nil
}

func (s *strategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
}

func (s *strategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

func (s *strategy) AllowUnconditionalUpdate() bool {
	return false
}

func (s *strategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return nil
}

func (s *strategy) PrepareForDelete(ctx context.Context, obj runtime.Object) {
}

func (s *strategy) WarningsOnDelete(ctx context.Context, obj runtime.Object) []string {
	return nil
}

func (s *strategy) ObjectKinds(obj runtime.Object) ([]schema.GroupVersionKind, bool, error) {
	return s.ObjectTyper.ObjectKinds(obj)
}

func (s *strategy) Recognizes(gvk schema.GroupVersionKind) bool {
	return gvk == s.kind.GroupVersionKind()
}

func (s *strategy) CheckGracefulDelete(ctx context.Context, obj runtime.Object, options *metav1.DeleteOptions) bool {
	return false
}

var _ rest.Scoper = &strategy{}
var _ rest.RESTCreateStrategy = &strategy{}
var _ rest.RESTUpdateStrategy = &strategy{}
var _ rest.RESTDeleteStrategy = &strategy{}
var _ rest.RESTGracefulDeleteStrategy = &strategy{}
