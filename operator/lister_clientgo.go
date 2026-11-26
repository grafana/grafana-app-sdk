package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/grafana/grafana-app-sdk/resource"
)

// PageFunc reads one client-go LIST page. The caller binds the kind, namespace
// and selectors. The lister supplies the limit and continuation token.
type PageFunc func(context.Context, metav1.ListOptions) (runtime.Object, error)

// PagedListerOptions sets the page size and whether to project to identities.
type PagedListerOptions struct {
	PageSize int64
	// Detail controls projection. Zero means WatchDetailFullObject.
	Detail WatchDetail
}

// NewPagedLister adapts a client-go LIST to a complete snapshot. Identity mode
// accepts any item with Kubernetes metadata and preserves its creation
// timestamp for age-based handlers. Full mode wraps a matching native object
// or decodes it with the kind's JSON codec.
func NewPagedLister(kind resource.Kind, read PageFunc, opts PagedListerOptions) (ResourceLister, error) {
	if kind.Schema == nil || read == nil {
		return nil, errors.New("paged lister requires a kind and page function")
	}
	if opts.Detail != "" && opts.Detail != WatchDetailIdentity && opts.Detail != WatchDetailFullObject {
		return nil, fmt.Errorf("unsupported LIST detail %q", opts.Detail)
	}
	return pagedLister{kind: kind, read: read, opts: opts}, nil
}

type pagedLister struct {
	kind resource.Kind
	read PageFunc
	opts PagedListerOptions
}

func (l pagedLister) List(ctx context.Context) ([]resource.Object, string, error) {
	size := l.opts.PageSize
	if size <= 0 {
		size = DefaultListerPageSize
	}
	return collectListPages(ctx, func(ctx context.Context, token string) (listPage, error) {
		page, err := l.read(ctx, metav1.ListOptions{Limit: size, Continue: token})
		if err != nil {
			return listPage{}, err
		}
		accessor, err := meta.ListAccessor(page)
		if err != nil {
			return listPage{}, err
		}
		items, err := meta.ExtractList(page)
		if err != nil {
			return listPage{}, err
		}
		objects := make([]resource.Object, 0, len(items))
		for _, item := range items {
			obj, err := l.convert(item)
			if err != nil {
				return listPage{}, err
			}
			objects = append(objects, obj)
		}
		return listPage{objects: objects, version: accessor.GetResourceVersion(), next: accessor.GetContinue()}, nil
	})
}

func (l pagedLister) convert(obj runtime.Object) (resource.Object, error) {
	if l.opts.Detail != WatchDetailIdentity {
		return l.fullObject(obj)
	}
	item, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	id := resource.Identifier{Namespace: item.GetNamespace(), Name: item.GetName()}
	projected := l.kind.NewIdentityObject(id, item.GetResourceVersion())
	projected.SetCreationTimestamp(item.GetCreationTimestamp())
	return projected, nil
}

type nativeObjectWrapper interface {
	WrapObject(runtime.Object) (resource.Object, bool)
}

func (l pagedLister) fullObject(obj runtime.Object) (resource.Object, error) {
	expected := l.kind.ZeroValue()
	converted, err := toResourceObject(obj, l.kind)
	if err == nil {
		if reflect.TypeOf(converted) == reflect.TypeOf(expected) {
			original := converted.GroupVersionKind()
			if gvk := withFallbackGVK(original, l.kind.GroupVersionKind()); gvk != original {
				converted = converted.Copy()
				converted.SetGroupVersionKind(gvk)
			}
			return converted, nil
		}
		obj = clientGoObject(converted)
	}
	if _, metadataErr := meta.Accessor(obj); metadataErr != nil {
		return nil, metadataErr
	}

	native := obj.DeepCopyObject()
	// Typed client-go list entries can omit TypeMeta.
	native.GetObjectKind().SetGroupVersionKind(withFallbackGVK(native.GetObjectKind().GroupVersionKind(), l.kind.GroupVersionKind()))
	if wrapper, ok := expected.(nativeObjectWrapper); ok {
		if wrapped, ok := wrapper.WrapObject(native); ok {
			return wrapped, nil
		}
	}

	encoded, err := json.Marshal(native)
	if err != nil {
		return nil, fmt.Errorf("encode native LIST object: %w", err)
	}
	converted, err = l.kind.Read(bytes.NewReader(encoded), resource.KindEncodingJSON)
	if err != nil {
		return nil, fmt.Errorf("decode native LIST object as %s: %w", l.kind.GroupVersionKind(), err)
	}
	converted.SetGroupVersionKind(withFallbackGVK(converted.GroupVersionKind(), l.kind.GroupVersionKind()))
	return converted, nil
}
