package apiserver

import (
	"context"
	"fmt"

	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/apis/example"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
)

var _ rest.StandardStorage = &RESTStorage{}

type RESTStorage struct {
	*genericregistry.Store
}

// NewRESTStorage returns a RESTStorage object that will work against API services.
func NewRESTStorage(scheme *runtime.Scheme, kind resource.Kind, optsGetter generic.RESTOptionsGetter) (*RESTStorage, error) {
	strategy := NewGenericStrategy(scheme)

	store := &genericregistry.Store{
		NewFunc:                   func() runtime.Object { return kind.ZeroValue() },
		NewListFunc:               func() runtime.Object { return &resource.UntypedList{} },
		PredicateFunc:             MatchObject,
		DefaultQualifiedResource:  example.Resource("examples"),
		SingularQualifiedResource: example.Resource("example"),

		CreateStrategy: strategy,
		UpdateStrategy: strategy,
		DeleteStrategy: strategy,

		// TODO: define table converter that exposes more than name/creation timestamp
		TableConvertor: rest.NewDefaultTableConvertor(example.Resource("examples")),
	}
	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: GetAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, err
	}
	return &RESTStorage{store}, nil
}

// NewGenericStrategy creates and returns a genericStrategy instance
func NewGenericStrategy(typer runtime.ObjectTyper) genericStrategy {
	return genericStrategy{typer, names.SimpleNameGenerator}
}

// GetAttrs returns labels.Set, fields.Set, and error in case the given runtime.Object is not a resource.Object
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	object, ok := obj.(resource.Object)
	if !ok {
		return nil, nil, fmt.Errorf("given object is not a resource.Object")
	}
	return labels.Set(object.GetLabels()), fields.Set{
		"metadata.name":      object.GetName(),
		"metadata.namespace": object.GetNamespace(),
	}, nil
}

// MatchObject is the filter used by the generic etcd backend to watch events
// from etcd to clients of the apiserver only interested in specific labels/fields.
func MatchObject(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}

type genericStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

func (genericStrategy) NamespaceScoped() bool {
	return true
}

func (genericStrategy) PrepareForCreate(_ context.Context, _ runtime.Object) {
}

func (genericStrategy) PrepareForUpdate(_ context.Context, _, _ runtime.Object) {
}

func (genericStrategy) Validate(_ context.Context, _ runtime.Object) field.ErrorList {
	return []*field.Error{}
}

// WarningsOnCreate returns warnings for the creation of the given object.
func (genericStrategy) WarningsOnCreate(_ context.Context, _ runtime.Object) []string { return nil }

func (genericStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (genericStrategy) AllowUnconditionalUpdate() bool {
	return false
}

func (genericStrategy) Canonicalize(_ runtime.Object) {
}

func (genericStrategy) ValidateUpdate(_ context.Context, _, _ runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

// WarningsOnUpdate returns warnings for the given update.
func (genericStrategy) WarningsOnUpdate(_ context.Context, _, _ runtime.Object) []string {
	return nil
}
