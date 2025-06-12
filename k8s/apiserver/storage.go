package apiserver

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"

	"github.com/grafana/grafana-app-sdk/resource"
)

func newGenericStoreForKind(scheme *runtime.Scheme, kind resource.Kind, optsGetter generic.RESTOptionsGetter) (*genericregistry.Store, error) {
	strategy := newStrategy(scheme, kind)

	store := &genericregistry.Store{
		NewFunc: func() runtime.Object {
			return kind.ZeroValue()
		},
		NewListFunc: func() runtime.Object {
			return kind.ZeroListValue()
		},
		PredicateFunc:             matchKind,
		DefaultQualifiedResource:  kind.GroupVersionResource().GroupResource(),
		SingularQualifiedResource: kind.GroupVersionResource().GroupResource(),

		CreateStrategy: strategy,
		UpdateStrategy: strategy,
		DeleteStrategy: strategy,
		TableConvertor: rest.NewDefaultTableConvertor(kind.GroupVersionResource().GroupResource()),
	}

	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: getAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, fmt.Errorf("failed completing storage options for %s: %w", kind.Kind(), err)
	}

	return store, nil
}

func getAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	resourceObj, ok := obj.(resource.Object)
	if !ok {
		return nil, nil, fmt.Errorf("object (%T) is not a resource.Object", obj)
	}
	m := metav1.ObjectMeta{
		Name:                       resourceObj.GetName(),
		Namespace:                  resourceObj.GetNamespace(),
		Labels:                     resourceObj.GetLabels(),
		Annotations:                resourceObj.GetAnnotations(),
		OwnerReferences:            resourceObj.GetOwnerReferences(),
		Finalizers:                 resourceObj.GetFinalizers(),
		ResourceVersion:            resourceObj.GetResourceVersion(),
		UID:                        resourceObj.GetUID(),
		Generation:                 resourceObj.GetGeneration(),
		CreationTimestamp:          resourceObj.GetCreationTimestamp(),
		DeletionTimestamp:          resourceObj.GetDeletionTimestamp(),
		DeletionGracePeriodSeconds: resourceObj.GetDeletionGracePeriodSeconds(),
		ManagedFields:              resourceObj.GetManagedFields(),
	}
	return labels.Set(m.Labels), generic.ObjectMetaFieldsSet(&m, true), nil
}

func matchKind(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: getAttrs,
	}
}
