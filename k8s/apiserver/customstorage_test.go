package apiserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/grafana/grafana-app-sdk/resource"
)

func withNamespace(ctx context.Context, namespace string) context.Context {
	return request.WithNamespace(ctx, namespace)
}

func newTestObject(namespace, name string) *resource.UntypedObject {
	obj := &resource.UntypedObject{}
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

func TestCustomStorage_NamespaceScoped(t *testing.T) {
	storage := NewCustomStorage[*resource.UntypedObject, *resource.UntypedList](TestKind, newFakeBackend())
	scoper, ok := storage.(rest.Scoper)
	require.True(t, ok)
	assert.True(t, scoper.NamespaceScoped())
}

func TestCustomStorage_Get(t *testing.T) {
	backend := newFakeBackend()
	obj := newTestObject("default", "foo")
	_, err := backend.Create(context.Background(), obj)
	require.NoError(t, err)

	storage := NewCustomStorage[*resource.UntypedObject, *resource.UntypedList](TestKind, backend)
	getter, ok := storage.(rest.Getter)
	require.True(t, ok)

	got, err := getter.Get(withNamespace(context.Background(), "default"), "foo", &metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, obj, got)

	_, err = getter.Get(withNamespace(context.Background(), "default"), "missing", &metav1.GetOptions{})
	assert.Error(t, err)
}

func TestCustomStorage_List(t *testing.T) {
	backend := newFakeBackend()
	_, err := backend.Create(context.Background(), newTestObject("default", "foo"))
	require.NoError(t, err)
	_, err = backend.Create(context.Background(), newTestObject("other", "bar"))
	require.NoError(t, err)

	storage := NewCustomStorage[*resource.UntypedObject, *resource.UntypedList](TestKind, backend)
	lister, ok := storage.(rest.Lister)
	require.True(t, ok)

	result, err := lister.List(withNamespace(context.Background(), "default"), &metainternalversion.ListOptions{})
	require.NoError(t, err)
	list, ok := result.(*resource.UntypedList)
	require.True(t, ok)
	require.Len(t, list.Items, 1)
	assert.Equal(t, "foo", list.Items[0].GetName())
}

func TestCustomStorage_Create(t *testing.T) {
	backend := newFakeBackend()
	storage := NewCustomStorage[*resource.UntypedObject, *resource.UntypedList](TestKind, backend)
	creater, ok := storage.(rest.Creater)
	require.True(t, ok)

	obj := newTestObject("default", "foo")
	created, err := creater.Create(withNamespace(context.Background(), "default"), obj, rest.ValidateAllObjectFunc, &metav1.CreateOptions{})
	require.NoError(t, err)
	assert.Equal(t, obj, created)

	stored, err := backend.Get(context.Background(), resource.Identifier{Namespace: "default", Name: "foo"})
	require.NoError(t, err)
	assert.Equal(t, obj, stored)
}

type staticUpdatedObjectInfo struct {
	obj *resource.UntypedObject
}

func (s *staticUpdatedObjectInfo) Preconditions() *metav1.Preconditions {
	return nil
}

func (s *staticUpdatedObjectInfo) UpdatedObject(_ context.Context, _ runtime.Object) (runtime.Object, error) {
	return s.obj, nil
}

func TestCustomStorage_Update(t *testing.T) {
	backend := newFakeBackend()
	existing := newTestObject("default", "foo")
	_, err := backend.Create(context.Background(), existing)
	require.NoError(t, err)

	storage := NewCustomStorage[*resource.UntypedObject, *resource.UntypedList](TestKind, backend)
	updater, ok := storage.(rest.Updater)
	require.True(t, ok)

	updated := newTestObject("default", "foo")
	updated.Spec = map[string]any{"changed": true}
	result, _, err := updater.Update(withNamespace(context.Background(), "default"), "foo", &staticUpdatedObjectInfo{obj: updated}, rest.ValidateAllObjectFunc, rest.ValidateAllObjectUpdateFunc, false, &metav1.UpdateOptions{})
	require.NoError(t, err)
	assert.Equal(t, updated, result)
}

func TestCustomStorage_Delete(t *testing.T) {
	backend := newFakeBackend()
	obj := newTestObject("default", "foo")
	_, err := backend.Create(context.Background(), obj)
	require.NoError(t, err)

	storage := NewCustomStorage[*resource.UntypedObject, *resource.UntypedList](TestKind, backend)
	deleter, ok := storage.(rest.GracefulDeleter)
	require.True(t, ok)

	deleted, immediate, err := deleter.Delete(withNamespace(context.Background(), "default"), "foo", rest.ValidateAllObjectFunc, &metav1.DeleteOptions{})
	require.NoError(t, err)
	assert.True(t, immediate)
	assert.Equal(t, obj, deleted)

	_, err = backend.Get(context.Background(), resource.Identifier{Namespace: "default", Name: "foo"})
	assert.Error(t, err)
}

func TestCustomStorage_Watch(t *testing.T) {
	backend := newFakeBackend()
	storage := NewCustomStorage[*resource.UntypedObject, *resource.UntypedList](TestKind, backend)
	watcher, ok := storage.(rest.Watcher)
	require.True(t, ok)

	w, err := watcher.Watch(withNamespace(context.Background(), "default"), &metainternalversion.ListOptions{})
	require.NoError(t, err)
	w.Stop()
}

func TestUnsupportedWriteBackend(t *testing.T) {
	backend := UnsupportedWriteBackend[*resource.UntypedObject, *resource.UntypedList]{Kind: TestKind}

	_, err := backend.Create(context.Background(), newTestObject("default", "foo"))
	assert.True(t, apierrors.IsMethodNotSupported(err))

	_, err = backend.Update(context.Background(), newTestObject("default", "foo"))
	assert.True(t, apierrors.IsMethodNotSupported(err))

	err = backend.Delete(context.Background(), resource.Identifier{Namespace: "default", Name: "foo"})
	assert.True(t, apierrors.IsMethodNotSupported(err))

	_, err = backend.Watch(context.Background(), "default", resource.WatchOptions{})
	assert.True(t, apierrors.IsMethodNotSupported(err))
}
