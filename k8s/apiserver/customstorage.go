package apiserver

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/grafana/grafana-app-sdk/resource"
)

// Backend is implemented by developer-provided code which backs a Kind's storage with something
// other than the SDK's generic, etcd/CRD-backed store (for example, a SQL database, an external
// API, or computed data). T is the Kind's Object type, and L is the Kind's ListObject type.
//
// Codegen produces a per-Kind interface with this exact method shape (with T and L substituted
// for the concrete generated types), so a generated `<Kind>Backend` interface satisfies
// Backend[*<Kind>, *<Kind>List] without any explicit conversion.
type Backend[T resource.Object, L resource.ListObject] interface {
	// Get retrieves a single object by its identifier.
	Get(ctx context.Context, identifier resource.Identifier) (T, error)
	// List retrieves a list of objects in the given namespace matching the provided options.
	List(ctx context.Context, namespace string, options resource.ListOptions) (L, error)
	// Create creates a new object, returning the created object as it is stored.
	Create(ctx context.Context, obj T) (T, error)
	// Update updates an existing object, returning the updated object as it is stored.
	Update(ctx context.Context, obj T) (T, error)
	// Delete deletes an existing object by its identifier.
	Delete(ctx context.Context, identifier resource.Identifier) error
	// Watch begins watching for changes to objects in the given namespace matching the provided options.
	Watch(ctx context.Context, namespace string, options resource.WatchOptions) (resource.WatchResponse, error)
}

// UnsupportedWriteBackend is a helper for building a read-only Backend[T, L]. Embed it in a
// struct which implements Get and List to get Create, Update, Delete and Watch implementations
// for free, all of which return an apierrors.NewMethodNotSupported error using the GroupResource
// from Kind. This is useful for Kinds whose data is computed on the fly or read from a source
// that should not be writable through this API.
//
//	type myReadOnlyBackend struct {
//		apiserver.UnsupportedWriteBackend[*MyKind, *MyKindList]
//		// ... fields needed for Get/List, e.g. a client for the underlying data source
//	}
//
//	func (b *myReadOnlyBackend) Get(ctx context.Context, identifier resource.Identifier) (*MyKind, error) {
//		// ...
//	}
//
//	func (b *myReadOnlyBackend) List(ctx context.Context, namespace string, opts resource.ListOptions) (*MyKindList, error) {
//		// ...
//	}
type UnsupportedWriteBackend[T resource.Object, L resource.ListObject] struct {
	Kind resource.Kind
}

// Create always returns an apierrors.NewMethodNotSupported error.
func (b UnsupportedWriteBackend[T, L]) Create(_ context.Context, _ T) (T, error) {
	var zero T
	return zero, apierrors.NewMethodNotSupported(b.Kind.GroupVersionResource().GroupResource(), "create")
}

// Update always returns an apierrors.NewMethodNotSupported error.
func (b UnsupportedWriteBackend[T, L]) Update(_ context.Context, _ T) (T, error) {
	var zero T
	return zero, apierrors.NewMethodNotSupported(b.Kind.GroupVersionResource().GroupResource(), "update")
}

// Delete always returns an apierrors.NewMethodNotSupported error.
func (b UnsupportedWriteBackend[T, L]) Delete(_ context.Context, _ resource.Identifier) error {
	return apierrors.NewMethodNotSupported(b.Kind.GroupVersionResource().GroupResource(), "delete")
}

// Watch always returns an apierrors.NewMethodNotSupported error.
func (b UnsupportedWriteBackend[T, L]) Watch(_ context.Context, _ string, _ resource.WatchOptions) (resource.WatchResponse, error) {
	return nil, apierrors.NewMethodNotSupported(b.Kind.GroupVersionResource().GroupResource(), "watch")
}

// NewCustomStorage returns a rest.Storage implementation which dispatches Kubernetes API server
// verb calls (Get/List/Create/Update/Delete/Watch) to the provided Backend, instead of the SDK's
// generic etcd/CRD-backed store. The returned storage can be supplied to
// defaultInstaller.SetCustomStorage to serve a Kind through the aggregated API server without it
// being backed by a CRD.
//
// The returned storage does not support subresources; a Kind using custom storage which declares
// subresources must supply separate rest.Storage for each subresource path.
func NewCustomStorage[T resource.Object, L resource.ListObject](kind resource.Kind, backend Backend[T, L]) rest.Storage {
	return &customStorage[T, L]{
		kind:    kind,
		backend: backend,
		table:   rest.NewDefaultTableConvertor(kind.GroupVersionResource().GroupResource()),
	}
}

type customStorage[T resource.Object, L resource.ListObject] struct {
	kind    resource.Kind
	backend Backend[T, L]
	table   rest.TableConvertor
}

var (
	_ rest.Storage         = (*customStorage[resource.Object, resource.ListObject])(nil)
	_ rest.Scoper          = (*customStorage[resource.Object, resource.ListObject])(nil)
	_ rest.Getter          = (*customStorage[resource.Object, resource.ListObject])(nil)
	_ rest.Lister          = (*customStorage[resource.Object, resource.ListObject])(nil)
	_ rest.CreaterUpdater  = (*customStorage[resource.Object, resource.ListObject])(nil)
	_ rest.GracefulDeleter = (*customStorage[resource.Object, resource.ListObject])(nil)
	_ rest.Watcher         = (*customStorage[resource.Object, resource.ListObject])(nil)
)

// New returns an empty object that can be used with Create and Update after request data has
// been put into it.
func (c *customStorage[T, L]) New() runtime.Object {
	return c.kind.ZeroValue()
}

// NewList returns an empty object that can be used with the List call.
func (c *customStorage[T, L]) NewList() runtime.Object {
	return c.kind.ZeroListValue()
}

// Destroy cleans up resources on shutdown. The Backend is owned by the caller, so there is
// nothing for customStorage itself to clean up.
func (*customStorage[T, L]) Destroy() {
}

// NamespaceScoped returns true if the storage is namespaced.
func (c *customStorage[T, L]) NamespaceScoped() bool {
	return c.kind.Scope() != resource.ClusterScope
}

// ConvertToTable implements rest.TableConvertor by delegating to the default table convertor
// for the Kind's GroupResource.
func (c *customStorage[T, L]) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return c.table.ConvertToTable(ctx, object, tableOptions)
}

// Get finds a resource in the Backend by name and returns it.
func (c *customStorage[T, L]) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	obj, err := c.backend.Get(ctx, identifierFromContext(ctx, name))
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// List selects resources in the Backend which match the provided options.
func (c *customStorage[T, L]) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	opts := resource.ListOptions{}
	if options != nil {
		if options.LabelSelector != nil {
			opts.LabelFilters = []string{options.LabelSelector.String()}
		}
		if options.FieldSelector != nil {
			opts.FieldSelectors = []string{options.FieldSelector.String()}
		}
		opts.ResourceVersion = options.ResourceVersion
		opts.Continue = options.Continue
		opts.Limit = int(options.Limit)
	}
	list, err := c.backend.List(ctx, namespaceFromContext(ctx), opts)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// Create creates a new version of a resource via the Backend.
func (c *customStorage[T, L]) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	typed, err := asT[T](obj)
	if err != nil {
		return nil, err
	}
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}
	created, err := c.backend.Create(ctx, typed)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Update finds a resource in the Backend and updates it.
//
//nolint:revive
func (c *customStorage[T, L]) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, _ *metav1.UpdateOptions) (runtime.Object, bool, error) {
	identifier := identifierFromContext(ctx, name)
	old, err := c.backend.Get(ctx, identifier)
	created := false
	if err != nil {
		if !forceAllowCreate {
			return nil, false, err
		}
		created = true
	}
	var oldRuntime runtime.Object
	if !created {
		oldRuntime = old
	}
	updatedRuntime, err := objInfo.UpdatedObject(ctx, oldRuntime)
	if err != nil {
		return nil, false, err
	}
	updated, err := asT[T](updatedRuntime)
	if err != nil {
		return nil, false, err
	}
	if created {
		if createValidation != nil {
			if err := createValidation(ctx, updated); err != nil {
				return nil, false, err
			}
		}
		result, err := c.backend.Create(ctx, updated)
		if err != nil {
			return nil, false, err
		}
		return result, true, nil
	}
	if updateValidation != nil {
		if err := updateValidation(ctx, updated, old); err != nil {
			return nil, false, err
		}
	}
	result, err := c.backend.Update(ctx, updated)
	if err != nil {
		return nil, false, err
	}
	return result, false, nil
}

// Delete finds a resource in the Backend and deletes it.
func (c *customStorage[T, L]) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, _ *metav1.DeleteOptions) (runtime.Object, bool, error) {
	identifier := identifierFromContext(ctx, name)
	obj, err := c.backend.Get(ctx, identifier)
	if err != nil {
		return nil, false, err
	}
	if deleteValidation != nil {
		if err := deleteValidation(ctx, obj); err != nil {
			return nil, false, err
		}
	}
	if err := c.backend.Delete(ctx, identifier); err != nil {
		return nil, false, err
	}
	return obj, true, nil
}

// Watch begins watching for changes to resources in the Backend which match the provided options.
func (c *customStorage[T, L]) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	opts := resource.WatchOptions{}
	if options != nil {
		if options.LabelSelector != nil {
			opts.LabelFilters = []string{options.LabelSelector.String()}
		}
		if options.FieldSelector != nil {
			opts.FieldSelectors = []string{options.FieldSelector.String()}
		}
		opts.ResourceVersion = options.ResourceVersion
		opts.ResourceVersionMatch = string(options.ResourceVersionMatch)
		opts.AllowWatchBookmarks = options.AllowWatchBookmarks
		opts.TimeoutSeconds = options.TimeoutSeconds
		opts.SendInitialEvents = options.SendInitialEvents
	}
	resp, err := c.backend.Watch(ctx, namespaceFromContext(ctx), opts)
	if err != nil {
		return nil, err
	}
	return &watchWrapper{response: resp}, nil
}

func identifierFromContext(ctx context.Context, name string) resource.Identifier {
	return resource.Identifier{Namespace: namespaceFromContext(ctx), Name: name}
}

// namespaceFromContext returns the namespace from the request context, or "" if the context
// carries no namespace (e.g. for cluster-scoped kinds).
func namespaceFromContext(ctx context.Context) string {
	ns, _ := request.NamespaceFrom(ctx)
	return ns
}

func asT[T resource.Object](obj runtime.Object) (T, error) {
	typed, ok := obj.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("object (%T) is not a %T", obj, zero)
	}
	return typed, nil
}

// watchWrapper adapts a resource.WatchResponse into a watch.Interface.
type watchWrapper struct {
	response resource.WatchResponse
	result   chan watch.Event
	started  bool
}

func (w *watchWrapper) Stop() {
	w.response.Stop()
}

func (w *watchWrapper) ResultChan() <-chan watch.Event {
	if !w.started {
		w.started = true
		w.result = make(chan watch.Event)
		go func() {
			defer close(w.result)
			for evt := range w.response.WatchEvents() {
				w.result <- watch.Event{
					Type:   watchEventType(evt.EventType),
					Object: evt.Object,
				}
			}
		}()
	}
	return w.result
}

func watchEventType(eventType string) watch.EventType {
	switch eventType {
	case "ADDED":
		return watch.Added
	case "MODIFIED":
		return watch.Modified
	case "DELETED":
		return watch.Deleted
	case "BOOKMARK":
		return watch.Bookmark
	case "ERROR":
		return watch.Error
	default:
		return watch.EventType(eventType)
	}
}
