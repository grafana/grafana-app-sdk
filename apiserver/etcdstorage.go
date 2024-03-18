package apiserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
)

var _ rest.StandardStorage = &RESTStorage{}

type RESTStorageProvider struct {
	optsGetter generic.RESTOptionsGetter
}

func (r *RESTStorageProvider) StandardStorage(kind resource.Kind, scheme *runtime.Scheme) (rest.StandardStorage, error) {
	return NewRESTStorage(scheme, kind, r.optsGetter)
}

type RESTStorage struct {
	*genericregistry.Store
}

// NewRESTStorage returns a RESTStorage object that will work against API services.
func NewRESTStorage(scheme *runtime.Scheme, kind resource.Kind, optsGetter generic.RESTOptionsGetter) (*RESTStorage, error) {
	strategy := NewGenericStrategy(scheme, kind)

	store := &genericregistry.Store{
		NewFunc:                   func() runtime.Object { return kind.ZeroValue() },
		NewListFunc:               func() runtime.Object { return kind.ZeroListValue() },
		PredicateFunc:             MatchObjectFunc(kind),
		DefaultQualifiedResource:  schema.GroupResource{Group: kind.Group(), Resource: strings.ToLower(kind.Plural())},
		SingularQualifiedResource: schema.GroupResource{Group: kind.Group(), Resource: strings.ToLower(kind.Kind())},
		CreateStrategy:            strategy,
		UpdateStrategy:            strategy,
		DeleteStrategy:            strategy,

		// TODO: define table converter that exposes more than name/creation timestamp
		TableConvertor: rest.NewDefaultTableConvertor(schema.GroupResource{Group: kind.Group(), Resource: strings.ToLower(kind.Plural())}),
	}
	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: GetAttrsFunc(kind)}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, err
	}
	return &RESTStorage{store}, nil
}

// NewGenericStrategy creates and returns a genericStrategy instance
func NewGenericStrategy(typer runtime.ObjectTyper, kind resource.Kind) *genericStrategy {
	return &genericStrategy{typer, names.SimpleNameGenerator, kind}
}

func GetAttrsFunc(kind resource.Kind) func(obj runtime.Object) (labels.Set, fields.Set, error) {
	return func(obj runtime.Object) (labels.Set, fields.Set, error) {
		object, ok := obj.(resource.Object)
		if !ok {
			return nil, nil, fmt.Errorf("given object is not a resource.Object")
		}
		fields := make(fields.Set)
		fields["metadata.name"] = object.GetName()
		if kind.Scope() != resource.ClusterScope {
			fields["metadata.namespace"] = object.GetNamespace()
		}
		return labels.Set(object.GetLabels()), fields, nil
	}
}

func MatchObjectFunc(kind resource.Kind) func(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return func(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
		return storage.SelectionPredicate{
			Label:    label,
			Field:    field,
			GetAttrs: GetAttrsFunc(kind),
		}
	}
}

type genericStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
	resource.Kind
}

func (g *genericStrategy) NamespaceScoped() bool {
	return g.Kind.Scope() == resource.NamespacedScope
}

func (g *genericStrategy) PrepareForCreate(_ context.Context, _ runtime.Object) {
}

func (g *genericStrategy) PrepareForUpdate(_ context.Context, _, _ runtime.Object) {
}

func (g *genericStrategy) Validate(_ context.Context, _ runtime.Object) field.ErrorList {
	return []*field.Error{}
}

// WarningsOnCreate returns warnings for the creation of the given object.
func (g *genericStrategy) WarningsOnCreate(_ context.Context, _ runtime.Object) []string { return nil }

func (g *genericStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (g *genericStrategy) AllowUnconditionalUpdate() bool {
	return false
}

func (g *genericStrategy) Canonicalize(_ runtime.Object) {
}

func (g *genericStrategy) ValidateUpdate(_ context.Context, _, _ runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

// WarningsOnUpdate returns warnings for the given update.
func (g *genericStrategy) WarningsOnUpdate(_ context.Context, _, _ runtime.Object) []string {
	return nil
}
