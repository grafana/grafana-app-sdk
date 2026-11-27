package operator

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/grafana/grafana-app-sdk/resource"
)

// ErrProjectionUnsupported signals that the server or adapter cannot serve a
// keys projection for the requested kind and query.
var ErrProjectionUnsupported = errors.New("list projection unsupported")

// ResourceKey identifies one item in a keys-only snapshot.
type ResourceKey struct {
	Namespace       string
	Name            string
	ResourceVersion string
}

// ResourceKeyLister reads one complete snapshot, fetching every page as the
// iterator is consumed. A page failure must terminate the iterator with an error.
type ResourceKeyLister interface {
	ListKeys(context.Context) (snapshotVersion string, keys iter.Seq2[ResourceKey, error])
}

// KeyListerFactory builds a paginated keys lister for a kind and query. Return
// ErrProjectionUnsupported if the adapter cannot honor the namespace or selectors.
type KeyListerFactory func(resource.Kind, ClientListerOptions) (ResourceKeyLister, error)

// NewPeriodicKeysList schedules LIST sweeps for handlers that use only
// identities. It falls back to full-object LIST and projects those results
// when the keys route cannot serve the requested kind or query.
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

// NewIdentityLister returns identities from keys, or projects the fallback LIST
// when keys is nil or the adapter explicitly reports unsupported projection.
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
		identities = append(identities, l.kind.NewIdentityObject(obj.GetStaticMetadata().Identifier(), obj.GetResourceVersion()))
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
		id := resource.Identifier{Namespace: key.Namespace, Name: key.Name}
		items = append(items, l.kind.NewIdentityObject(id, key.ResourceVersion))
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return items, rv, nil
}
