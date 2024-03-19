package apiserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
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
	Subresources map[string]SubresourceStorage
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

	restStorage := &RESTStorage{Store: store, Subresources: map[string]SubresourceStorage{}}
	// build storage for subresources
	for key, _ := range kind.ZeroValue().GetSubresources() {
		restStorage.Subresources[key] = NewSubresourceREST(scheme, kind, restStorage)
	}

	return restStorage, nil
}

func (r *RESTStorage) GetSubresources() map[string]SubresourceStorage {
	s := map[string]SubresourceStorage{}
	for k, v := range r.Subresources {
		s[k] = v
	}
	return s
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

func NewSubresourceREST(scheme *runtime.Scheme, kind resource.Kind, rest *RESTStorage) *SubresourceREST {
	strategy := NewSubresourceStatusStrategy(scheme, kind)

	statusStore := *rest.Store
	statusStore.CreateStrategy = nil
	statusStore.DeleteStrategy = nil
	statusStore.UpdateStrategy = strategy
	statusStore.ResetFieldsStrategy = strategy
	return &SubresourceREST{store: &statusStore, Kind: kind}
}

// SubresourceREST implements the REST endpoint for changing the status of a Resource.
type SubresourceREST struct {
	store *genericregistry.Store
	resource.Kind
}

var _ = rest.Patcher(&SubresourceREST{})

// New creates a new APIService object.
func (r *SubresourceREST) New() runtime.Object {
	return r.ZeroValue()
}

// Destroy cleans up resources on shutdown.
func (r *SubresourceREST) Destroy() {
	// Given that underlying store is shared with REST,
	// we don't destroy it here explicitly.
}

// Get retrieves the object from the storage. It is required to support Patch.
func (r *SubresourceREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, options)
}

// Update alters the status subset of an object.
func (r *SubresourceREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// We are explicitly setting forceAllowCreate to false in the call to the underlying storage because
	// subresources should never allow create on update.
	return r.store.Update(ctx, name, objInfo, createValidation, updateValidation, false, options)
}

// GetResetFields implements rest.ResetFieldsStrategy
func (r *SubresourceREST) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return r.store.GetResetFields()
}

type subresourceStatusStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
	resource.Kind
}

// NewSubresourceStatusStrategy creates a new subresourceStatusStrategy.
func NewSubresourceStatusStrategy(scheme *runtime.Scheme, kind resource.Kind) rest.UpdateResetFieldsStrategy {
	return &subresourceStatusStrategy{ObjectTyper: scheme, NameGenerator: names.SimpleNameGenerator, Kind: kind}
}

func (s *subresourceStatusStrategy) NamespaceScoped() bool {
	return s.NamespaceScoped()
}

func (s *subresourceStatusStrategy) GetResetFields() map[fieldpath.APIVersion]*fieldpath.Set {
	return nil
}

func (s *subresourceStatusStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	newObj := obj.(resource.Object)
	oldObj := old.(resource.Object)
	newObj.SetSpec(oldObj.GetSpec())
	newObj.SetLabels(oldObj.GetLabels())
	newObj.SetAnnotations(oldObj.GetAnnotations())
	newObj.SetFinalizers(oldObj.GetFinalizers())
	newObj.SetOwnerReferences(oldObj.GetOwnerReferences())
}

func (s *subresourceStatusStrategy) AllowCreateOnUpdate() bool {
	return false
}

func (s *subresourceStatusStrategy) AllowUnconditionalUpdate() bool {
	return false
}

// Canonicalize normalizes the object after validation.
func (s *subresourceStatusStrategy) Canonicalize(obj runtime.Object) {
}

// ValidateUpdate validates an update of subresourceStatusStrategy.
func (s *subresourceStatusStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

// WarningsOnUpdate returns warnings for the given update.
func (s *subresourceStatusStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return nil
}
