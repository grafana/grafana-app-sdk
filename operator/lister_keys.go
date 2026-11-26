package operator

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/grafana/grafana-app-sdk/resource"
)

// ErrProjectionUnsupported means that a key lister cannot list only keys for the requested kind and query. The
// caller then falls back to a full LIST.
var ErrProjectionUnsupported = errors.New("list projection unsupported")

// ResourceKey identifies one object in a keys-only snapshot.
type ResourceKey struct {
	Namespace       string
	Name            string
	ResourceVersion string
}

// ResourceKeyLister lists the keys of every object of one kind. It reads pages while the iterator is consumed. If
// a page fails, the iterator must end with that error.
type ResourceKeyLister interface {
	ListKeys(context.Context) (snapshotVersion string, keys iter.Seq2[ResourceKey, error])
}

// KeyListerFactory builds a ResourceKeyLister for a kind and query. Return ErrProjectionUnsupported if it cannot
// handle the namespace or selectors.
type KeyListerFactory func(resource.Kind, ClientListerOptions) (ResourceKeyLister, error)

// NewPeriodicKeysList returns a mechanism like NewPeriodicList that delivers only identities. It lists keys with
// factory. If factory is nil, or returns ErrProjectionUnsupported, it does a full LIST and keeps only the
// identities.
func NewPeriodicKeysList(factory KeyListerFactory, defaults PeriodicListOptions) InformerMechanism {
	return NewInformerMechanism(periodicWatchOptions(WatchDetailIdentity), func(
		kind resource.Kind, clients resource.ClientGenerator, opts InformerOptions,
	) (Informer, error) {
		lister, err := identityClientLister(kind, clients, factory, listOptionsFor(opts, defaults.PageSize))
		if err != nil {
			return nil, err
		}
		return newPeriodicInformer(kind, lister, periodicOptions(opts, defaults), WatchDetailIdentity)
	})
}

func identityClientLister(kind resource.Kind, clients resource.ClientGenerator, factory KeyListerFactory, opts ClientListerOptions) (ResourceLister, error) {
	full, err := clientListerFor(kind, clients, opts)
	if err != nil {
		return nil, err
	}
	return keysListerFor(kind, factory, full, opts)
}

func keysListerFor(kind resource.Kind, factory KeyListerFactory, full ResourceLister, opts ClientListerOptions) (ResourceLister, error) {
	if factory == nil {
		return NewIdentityLister(kind, nil, full)
	}
	keys, err := factory(kind, opts)
	if errors.Is(err, ErrProjectionUnsupported) {
		return NewIdentityLister(kind, nil, full)
	}
	if err != nil {
		return nil, err
	}
	if keys == nil {
		return nil, errors.New("key lister factory returned no lister")
	}
	return NewIdentityLister(kind, keys, full)
}

// NewIdentityLister returns a ResourceLister that returns identities. It reads them from keys. If keys is nil, or
// returns ErrProjectionUnsupported, it uses fallback and keeps only the identities.
func NewIdentityLister(kind resource.Kind, keys ResourceKeyLister, fallback ResourceLister) (ResourceLister, error) {
	if kind.Schema == nil || fallback == nil {
		return nil, errors.New("identity lister requires a kind and fallback lister")
	}
	return identityLister{kind: kind, keys: keys, fallback: fallback}, nil
}

type identityLister struct {
	kind     resource.Kind
	keys     ResourceKeyLister
	fallback ResourceLister
}

func (l identityLister) List(ctx context.Context) ([]resource.Object, string, error) {
	if l.keys != nil {
		items, rv, err := l.listKeys(ctx)
		if !errors.Is(err, ErrProjectionUnsupported) {
			return items, rv, err
		}
	}

	items, rv, err := l.fallback.List(ctx)
	if err != nil {
		return nil, "", err
	}
	identities := make([]resource.Object, 0, len(items))
	for _, obj := range items {
		identities = append(identities, newIdentityObject(l.kind, obj.GetNamespace(), obj.GetName(), obj.GetResourceVersion()))
	}
	return identities, rv, nil
}

func (l identityLister) listKeys(ctx context.Context) ([]resource.Object, string, error) {
	rv, keys := l.keys.ListKeys(ctx)
	if keys == nil {
		return nil, "", errors.New("keys lister returned no iterator")
	}
	var items []resource.Object
	for key, err := range keys {
		if err != nil {
			return nil, "", err
		}
		if key.Name == "" {
			return nil, "", fmt.Errorf("keys snapshot at %q contains an empty name", rv)
		}
		items = append(items, newIdentityObject(l.kind, key.Namespace, key.Name, key.ResourceVersion))
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return items, rv, nil
}

func newIdentityObject(kind resource.Kind, namespace, name, rv string) resource.Object {
	obj := kind.ZeroValue()
	obj.SetGroupVersionKind(kind.GroupVersionKind())
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.SetResourceVersion(rv)
	return obj
}
