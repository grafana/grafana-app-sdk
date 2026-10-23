package operator

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestNewOpinionatedWatcher(t *testing.T) {
	ex := &resource.TypedSpecObject[string]{}
	schema := resource.NewSimpleSchema("group", "version", ex, &resource.TypedList[*resource.TypedSpecObject[string]]{}, resource.WithKind("my-crd"))
	client := &mockPatchClient{}

	t.Run("nil args", func(t *testing.T) {
		o, err := NewOpinionatedWatcher(nil, nil, OpinionatedWatcherConfig{})
		assert.Nil(t, o)
		assert.Equal(t, fmt.Errorf("schema cannot be nil"), err)

		o, err = NewOpinionatedWatcher(schema, nil, OpinionatedWatcherConfig{})
		assert.Nil(t, o)
		assert.Equal(t, fmt.Errorf("client cannot be nil"), err)
	})

	t.Run("success", func(t *testing.T) {
		o, err := NewOpinionatedWatcher(schema, client, OpinionatedWatcherConfig{})
		assert.NoError(t, err)
		assert.NotNil(t, o)
		assert.Equal(t, "operator.version.my-crd.group", o.finalizer)
	})

	t.Run("success with custom finalizer", func(t *testing.T) {
		finalizer := "custom-finalizer"
		o, err := NewOpinionatedWatcher(schema, client, OpinionatedWatcherConfig{
			Finalizer: func(_ resource.Schema) string {
				return finalizer
			}})
		assert.NoError(t, err)
		assert.NotNil(t, o)
		assert.Equal(t, finalizer, o.finalizer)
	})

	t.Run("finalizer too long", func(t *testing.T) {
		finalizer := "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789"
		o, err := NewOpinionatedWatcher(schema, client, OpinionatedWatcherConfig{
			Finalizer: func(_ resource.Schema) string {
				return finalizer
			}})
		assert.Equal(t, fmt.Errorf("finalizer length cannot exceed 63 chars: %s", finalizer), err)
		assert.Nil(t, o)
	})
}

func TestOpinionatedWatcher_Wrap(t *testing.T) {
	simple := &SimpleWatcher{}
	simple.AddFunc = func(ctx context.Context, object resource.Object) error {
		fmt.Println("add")
		return nil
	}
	simple.UpdateFunc = func(ctx context.Context, object resource.Object, object2 resource.Object) error {
		fmt.Println("update")
		return nil
	}
	simple.DeleteFunc = func(ctx context.Context, object resource.Object) error {
		fmt.Println("delete")
		return nil
	}

	t.Run("nil watcher", func(t *testing.T) {
		w, err := NewOpinionatedWatcher(resource.NewSimpleSchema("", "", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{}), &mockPatchClient{}, OpinionatedWatcherConfig{})
		assert.Nil(t, err)
		w.Wrap(nil, false)
		assert.Nil(t, w.AddFunc)
		assert.Nil(t, w.UpdateFunc)
		assert.Nil(t, w.DeleteFunc)
		assert.Nil(t, w.SyncFunc)
	})

	t.Run("syncToAdd=false", func(t *testing.T) {
		w, err := NewOpinionatedWatcher(resource.NewSimpleSchema("", "", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{}), &mockPatchClient{}, OpinionatedWatcherConfig{})
		assert.Nil(t, err)
		w.Wrap(simple, false)
		assert.NotNil(t, w.AddFunc)
		assert.NotNil(t, w.UpdateFunc)
		assert.NotNil(t, w.DeleteFunc)
		assert.Nil(t, w.SyncFunc)
	})

	t.Run("syncToAdd=true", func(t *testing.T) {
		w, err := NewOpinionatedWatcher(resource.NewSimpleSchema("", "", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{}), &mockPatchClient{}, OpinionatedWatcherConfig{})
		assert.Nil(t, err)
		w.Wrap(simple, true)
		assert.NotNil(t, w.AddFunc)
		assert.NotNil(t, w.UpdateFunc)
		assert.NotNil(t, w.DeleteFunc)
		assert.NotNil(t, w.SyncFunc)
	})
}

func TestOpinionatedWatcher_Add(t *testing.T) {
	ex := &resource.TypedSpecObject[string]{}
	schema := resource.NewSimpleSchema("group", "version", ex, &resource.TypedList[*resource.TypedSpecObject[string]]{})
	client := &mockPatchClient{}
	o, err := NewOpinionatedWatcher(schema, client, OpinionatedWatcherConfig{})
	assert.Nil(t, err)

	t.Run("nil object", func(t *testing.T) {
		err := o.Add(context.TODO(), nil)
		assert.Equal(t, fmt.Errorf("object cannot be nil"), err)
	})

	t.Run("deleted, pending us, patch error", func(t *testing.T) {
		obj := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		obj.SetDeletionTimestamp(&dt)
		obj.SetFinalizers([]string{o.finalizer})
		patchErr := fmt.Errorf("I AM ERROR")
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			assert.Equal(t, resource.PatchOpReplace, request.Operations[0].Operation)
			return patchErr
		}
		err := o.Add(context.TODO(), obj)
		expectedErr := NewFinalizerOperationError(patchErr, resource.PatchRequest{Operations: []resource.PatchOperation{{Path: "/metadata/finalizers", Operation: resource.PatchOpReplace, Value: []string{}}, {Path: "/metadata/resourceVersion", Operation: resource.PatchOpReplace, Value: obj.GetResourceVersion()}}})
		assert.Equal(t, expectedErr, err)
	})

	t.Run("deleted, pending us, delete error", func(t *testing.T) {
		obj := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		obj.SetDeletionTimestamp(&dt)
		obj.SetFinalizers([]string{o.finalizer})
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			assert.Fail(t, "patch should not be called if delete call fails")
			return nil
		}
		deleteErr := fmt.Errorf("JE SUIS ERROR")
		o.DeleteFunc = func(c context.Context, object resource.Object) error {
			assert.Equal(t, obj, object)
			return deleteErr
		}
		err := o.Add(context.TODO(), obj)
		assert.Equal(t, deleteErr, err)
	})

	t.Run("deleted, in-progress add, delete error", func(t *testing.T) {
		obj := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		obj.SetDeletionTimestamp(&dt)
		obj.SetFinalizers([]string{o.addPendingFinalizer})
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			assert.Fail(t, "patch should not be called if delete call fails")
			return nil
		}
		deleteErr := fmt.Errorf("JE SUIS ERROR")
		o.DeleteFunc = func(c context.Context, object resource.Object) error {
			assert.Equal(t, obj, object)
			return deleteErr
		}
		err := o.Add(context.TODO(), obj)
		assert.Equal(t, deleteErr, err)
	})

	t.Run("deleted, pending us, success", func(t *testing.T) {
		obj := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		obj.SetDeletionTimestamp(&dt)
		obj.SetFinalizers([]string{o.finalizer})
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			return nil
		}
		o.DeleteFunc = func(c context.Context, object resource.Object) error {
			assert.Equal(t, obj, object)
			return nil
		}
		err := o.Add(context.TODO(), obj)
		assert.Nil(t, err)
	})

	t.Run("deleted, in-progress add, success", func(t *testing.T) {
		obj := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		obj.SetDeletionTimestamp(&dt)
		obj.SetFinalizers([]string{o.addPendingFinalizer})
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			return nil
		}
		o.DeleteFunc = func(c context.Context, object resource.Object) error {
			assert.Equal(t, obj, object)
			return nil
		}
		err := o.Add(context.TODO(), obj)
		assert.Nil(t, err)
	})

	t.Run("deleted, not waiting on us", func(t *testing.T) {
		obj := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		obj.SetDeletionTimestamp(&dt)
		obj.SetFinalizers([]string{o.finalizer + "_not"})
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			assert.Fail(t, "patch should not be called, our finalizer isn't in the list")
			return nil
		}
		o.DeleteFunc = func(c context.Context, object resource.Object) error {
			assert.Fail(t, "delete should not be called, our finalizer isn't in the list")
			return nil
		}
		err := o.Add(context.TODO(), obj)
		assert.Nil(t, err)
	})

	t.Run("sync", func(t *testing.T) {
		obj := schema.ZeroValue()
		obj.SetFinalizers([]string{o.finalizer})
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			assert.Fail(t, "patch should not be called, our finalizer isn't in the list")
			return nil
		}
		o.DeleteFunc = func(c context.Context, object resource.Object) error {
			assert.Fail(t, "delete should not be called, the object isn't deleted")
			return nil
		}
		o.AddFunc = func(c context.Context, object resource.Object) error {
			assert.Fail(t, "add should not be called, the object is not new (already has our finalizer)")
			return nil
		}
		syncErr := fmt.Errorf("I AM ERROR")
		o.SyncFunc = func(ctx context.Context, object resource.Object) error {
			return syncErr
		}
		err := o.Add(context.TODO(), obj)
		assert.Equal(t, syncErr, err)
	})

	t.Run("add error", func(t *testing.T) {
		obj := schema.ZeroValue()
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			// Patch should be for the WIP finalizer, not the real one
			assert.Equal(t, []string{o.addPendingFinalizer}, request.Operations[0].Value)
			return nil
		}
		addErr := fmt.Errorf("ICH BIN ERROR")
		o.AddFunc = func(c context.Context, object resource.Object) error {
			return addErr
		}
		err := o.Add(context.TODO(), obj)
		assert.Equal(t, addErr, err)
	})

	t.Run("add finalizer patch error", func(t *testing.T) {
		obj := schema.ZeroValue()
		patchErr := fmt.Errorf("SOY ERROR")
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			return patchErr
		}
		o.AddFunc = func(c context.Context, object resource.Object) error {
			return fmt.Errorf("should not be called")
		}
		err := o.Add(context.TODO(), obj)
		assert.Contains(t, err.Error(), patchErr.Error())
	})

	t.Run("add finalizer patch RV mismatch", func(t *testing.T) {
		obj := schema.ZeroValue()
		attempts := 0
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			attempts++
			if attempts <= 2 {
				return &apierrors.StatusError{
					ErrStatus: metav1.Status{
						Code: http.StatusConflict,
					},
				}
			}
			return nil
		}
		client.GetIntoFunc = func(ctx context.Context, i resource.Identifier, o resource.Object) error {
			o.SetResourceVersion(strconv.Itoa(attempts))
			return nil
		}
		addCalled := false
		o.AddFunc = func(c context.Context, object resource.Object) error {
			addCalled = true
			return nil
		}
		err := o.Add(context.TODO(), obj)
		assert.NoError(t, err)
		assert.Equal(t, 4, attempts)                   // 3 attempts for the WIP finalizer, then 1 for the final finalizer
		assert.Equal(t, "2", obj.GetResourceVersion()) // After the second attempt, we don't call GetInto anymore because the patch isn't rejected
		assert.True(t, addCalled)
	})

	t.Run("add finalizer, second patch error", func(t *testing.T) {
		obj := schema.ZeroValue()
		patchErr := fmt.Errorf("SOY ERROR")
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			if request.Operations[0].Operation == resource.PatchOpReplace {
				return patchErr
			}
			if request.Operations[0].Operation == resource.PatchOpAdd {
				f := append(object.GetFinalizers(), request.Operations[0].Value.([]string)...)
				object.SetFinalizers(f)
			}
			return nil
		}
		o.AddFunc = func(c context.Context, object resource.Object) error {
			return nil
		}
		err := o.Add(context.TODO(), obj)
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), patchErr.Error())
	})

	t.Run("success", func(t *testing.T) {
		obj := schema.ZeroValue()
		req := 0
		client.PatchIntoFunc = func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			assert.Len(t, request.Operations, 2) // The finalizer operation, and the RV check
			if req == 0 {
				assert.Equal(t, resource.PatchOpAdd, request.Operations[0].Operation)
				assert.Equal(t, []string{o.addPendingFinalizer}, request.Operations[0].Value)
				f := append(object.GetFinalizers(), request.Operations[0].Value.([]string)...)
				object.SetFinalizers(f)
			} else {
				assert.Equal(t, resource.PatchOpReplace, request.Operations[0].Operation)
				assert.Equal(t, o.finalizer, request.Operations[0].Value)
			}
			req++
			return nil
		}
		o.AddFunc = func(c context.Context, object resource.Object) error {
			return nil
		}
		err := o.Add(context.TODO(), obj)
		assert.Nil(t, err)
	})
}

func TestOpinionatedWatcher_Update(t *testing.T) {
	ex := &resource.TypedSpecObject[string]{}
	schema := resource.NewSimpleSchema("group", "version", ex, &resource.TypedList[*resource.TypedSpecObject[string]]{})
	client := &mockPatchClient{}
	o, err := NewOpinionatedWatcher(schema, client, OpinionatedWatcherConfig{})
	assert.Nil(t, err)

	t.Run("nil old", func(t *testing.T) {
		err := o.Update(context.TODO(), nil, schema.ZeroValue())
		assert.Equal(t, fmt.Errorf("old cannot be nil"), err)
	})

	t.Run("nil new", func(t *testing.T) {
		err := o.Update(context.TODO(), schema.ZeroValue(), nil)
		assert.Equal(t, fmt.Errorf("new cannot be nil"), err)
	})

	t.Run("same generation", func(t *testing.T) {
		o.UpdateFunc = func(ctx context.Context, old resource.Object, new resource.Object) error {
			assert.Fail(t, "update should not be called")
			return nil
		}
		o.DeleteFunc = func(ctx context.Context, object resource.Object) error {
			assert.Fail(t, "delete should not be called")
			return nil
		}
		old := schema.ZeroValue()
		new := schema.ZeroValue()
		old.SetGeneration(1)
		new.SetGeneration(1)
		err := o.Update(context.TODO(), old, new)
		assert.Nil(t, err)
	})

	t.Run("delete, not waiting on us", func(t *testing.T) {
		o.UpdateFunc = func(ctx context.Context, old resource.Object, new resource.Object) error {
			assert.Fail(t, "update should not be called")
			return nil
		}
		o.DeleteFunc = func(ctx context.Context, object resource.Object) error {
			assert.Fail(t, "delete should not be called")
			return nil
		}
		new := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		new.SetDeletionTimestamp(&dt)
		new.SetFinalizers([]string{o.finalizer + "_not"})
		err := o.Update(context.TODO(), schema.ZeroValue(), new)
		assert.Nil(t, err)
	})

	t.Run("delete, delete error", func(t *testing.T) {
		obj := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		obj.SetDeletionTimestamp(&dt)
		obj.SetFinalizers([]string{o.finalizer})
		o.UpdateFunc = func(ctx context.Context, old resource.Object, new resource.Object) error {
			assert.Fail(t, "update should not be called")
			return nil
		}
		deleteErr := fmt.Errorf("I AM ERROR")
		o.DeleteFunc = func(c context.Context, object resource.Object) error {
			assert.Equal(t, obj, object)
			return deleteErr
		}
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			assert.Fail(t, "patch should not be called for a failed delete")
			return nil
		}
		err := o.Update(context.TODO(), schema.ZeroValue(), obj)
		assert.Equal(t, deleteErr, err)
	})

	t.Run("delete, patch error", func(t *testing.T) {
		obj := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		obj.SetDeletionTimestamp(&dt)
		obj.SetFinalizers([]string{o.finalizer})
		o.UpdateFunc = func(ctx context.Context, old resource.Object, new resource.Object) error {
			assert.Fail(t, "update should not be called")
			return nil
		}
		o.DeleteFunc = func(c context.Context, object resource.Object) error {
			assert.Equal(t, obj, object)
			return nil
		}
		patchErr := fmt.Errorf("ICH BIN ERROR")
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			assert.Equal(t, resource.PatchOpReplace, request.Operations[0].Operation)
			return patchErr
		}
		err := o.Update(context.TODO(), schema.ZeroValue(), obj)
		expectedErr := NewFinalizerOperationError(patchErr, resource.PatchRequest{Operations: []resource.PatchOperation{{Path: "/metadata/finalizers", Operation: resource.PatchOpReplace, Value: []string{}}, {Path: "/metadata/resourceVersion", Operation: resource.PatchOpReplace, Value: obj.GetResourceVersion()}}})
		assert.Equal(t, expectedErr, err)
	})

	t.Run("delete, patch RV mismatch", func(t *testing.T) {
		obj := schema.ZeroValue()
		dt := metav1.NewTime(time.Time{})
		obj.SetDeletionTimestamp(&dt)
		obj.SetFinalizers([]string{o.finalizer})
		attempts := 0
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			attempts++
			if attempts <= 2 {
				return &apierrors.StatusError{
					ErrStatus: metav1.Status{
						Code: http.StatusConflict,
					},
				}
			}
			return nil
		}
		client.GetIntoFunc = func(ctx context.Context, i resource.Identifier, o resource.Object) error {
			o.SetResourceVersion(strconv.Itoa(attempts))
			return nil
		}
		deleteCalled := false
		o.DeleteFunc = func(c context.Context, object resource.Object) error {
			deleteCalled = true
			return nil
		}
		err := o.Update(context.TODO(), schema.ZeroValue(), obj)
		assert.NoError(t, err)
		assert.Equal(t, 3, attempts)
		assert.Equal(t, "2", obj.GetResourceVersion()) // After the second attempt, we don't call GetInto anymore because the patch isn't rejected
		assert.True(t, deleteCalled)
	})

	t.Run("finalizer update event", func(t *testing.T) {
		obj := schema.ZeroValue()
		obj.SetFinalizers([]string{o.finalizer})

		o.UpdateFunc = func(ctx context.Context, old resource.Object, new resource.Object) error {
			assert.Fail(t, "update should not be called")
			return nil
		}
		o.DeleteFunc = func(ctx context.Context, object resource.Object) error {
			assert.Fail(t, "delete should not be called")
			return nil
		}

		err := o.Update(context.TODO(), schema.ZeroValue(), obj)
		assert.Nil(t, err)
	})

	t.Run("update", func(t *testing.T) {
		obj := schema.ZeroValue()
		obj.SetFinalizers([]string{o.finalizer})
		obj2 := schema.ZeroValue()
		obj2.SetFinalizers([]string{o.finalizer})

		updateErr := fmt.Errorf("SOY ERROR")
		o.UpdateFunc = func(c context.Context, old resource.Object, new resource.Object) error {
			assert.Equal(t, obj, old)
			assert.Equal(t, obj2, new)
			return updateErr
		}
		o.DeleteFunc = func(ctx context.Context, object resource.Object) error {
			assert.Fail(t, "delete should not be called")
			return nil
		}
		client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
			assert.Fail(t, "patch should not be called")
			return nil
		}

		err := o.Update(context.TODO(), obj, obj2)
		assert.Equal(t, updateErr, err)
	})
}

func TestOpinionatedWatcher_Delete(t *testing.T) {
	ex := &resource.TypedSpecObject[string]{}
	schema := resource.NewSimpleSchema("group", "version", ex, &resource.TypedList[*resource.TypedSpecObject[string]]{})
	client := &mockPatchClient{}
	o, err := NewOpinionatedWatcher(schema, client, OpinionatedWatcherConfig{})
	assert.Nil(t, err)

	// Delete should do nothing
	client.PatchIntoFunc = func(ctx context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
		assert.Fail(t, "patch should not be called")
		return nil
	}
	o.DeleteFunc = func(ctx context.Context, object resource.Object) error {
		assert.Fail(t, "delete should not be called")
		return nil
	}
	assert.Nil(t, o.Delete(context.TODO(), schema.ZeroValue()))
}

type mockPatchClient struct {
	PatchIntoFunc func(context.Context, resource.Identifier, resource.PatchRequest, resource.PatchOptions, resource.Object) error
	GetIntoFunc   func(context.Context, resource.Identifier, resource.Object) error
}

func (p *mockPatchClient) PatchInto(ctx context.Context, identifier resource.Identifier, patch resource.PatchRequest, options resource.PatchOptions, into resource.Object) error {
	if p.PatchIntoFunc != nil {
		return p.PatchIntoFunc(ctx, identifier, patch, options, into)
	}
	return nil
}

func (p *mockPatchClient) GetInto(ctx context.Context, identifier resource.Identifier, into resource.Object) error {
	if p.GetIntoFunc != nil {
		return p.GetIntoFunc(ctx, identifier, into)
	}
	return nil
}
