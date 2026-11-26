package operator

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

type metricsSimpleWatcher struct {
	SimpleWatcher
	collectors []prometheus.Collector
}

func (m *metricsSimpleWatcher) PrometheusCollectors() []prometheus.Collector {
	return m.collectors
}

func TestOpinionatedWatcher_RegisterMetricsCollectors(t *testing.T) {
	t.Run("register collectors directly", func(t *testing.T) {
		w, err := NewOpinionatedWatcher(resource.NewSimpleSchema("", "", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{}), &mockPatchClient{}, OpinionatedWatcherConfig{})
		require.NoError(t, err)

		c1 := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_counter_1", Help: "test"})
		c2 := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_gauge_1", Help: "test"})

		w.RegisterMetricsCollectors(c1, c2)

		collectors := w.PrometheusCollectors()
		assert.Len(t, collectors, 2)
		assert.Contains(t, collectors, c1)
		assert.Contains(t, collectors, c2)
	})

	t.Run("additive calls", func(t *testing.T) {
		w, err := NewOpinionatedWatcher(resource.NewSimpleSchema("", "", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{}), &mockPatchClient{}, OpinionatedWatcherConfig{})
		require.NoError(t, err)

		c1 := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_counter_2", Help: "test"})
		c2 := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_gauge_2", Help: "test"})

		w.RegisterMetricsCollectors(c1)
		w.RegisterMetricsCollectors(c2)

		collectors := w.PrometheusCollectors()
		assert.Len(t, collectors, 2)
		assert.Contains(t, collectors, c1)
		assert.Contains(t, collectors, c2)
	})

	t.Run("combined with Wrap", func(t *testing.T) {
		w, err := NewOpinionatedWatcher(resource.NewSimpleSchema("", "", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{}), &mockPatchClient{}, OpinionatedWatcherConfig{})
		require.NoError(t, err)

		wrapperCollector := prometheus.NewCounter(prometheus.CounterOpts{Name: "wrapper_counter", Help: "test"})
		watcher := &metricsSimpleWatcher{
			collectors: []prometheus.Collector{wrapperCollector},
		}
		w.Wrap(watcher, false)

		directCollector := prometheus.NewGauge(prometheus.GaugeOpts{Name: "direct_gauge", Help: "test"})
		w.RegisterMetricsCollectors(directCollector)

		collectors := w.PrometheusCollectors()
		assert.Len(t, collectors, 2)
		assert.Contains(t, collectors, wrapperCollector)
		assert.Contains(t, collectors, directCollector)
	})

	t.Run("no-op with no arguments", func(t *testing.T) {
		w, err := NewOpinionatedWatcher(resource.NewSimpleSchema("", "", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{}), &mockPatchClient{}, OpinionatedWatcherConfig{})
		require.NoError(t, err)

		w.RegisterMetricsCollectors()

		collectors := w.PrometheusCollectors()
		assert.Empty(t, collectors)
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
		client.GetIntoFunc = newFinalizerAPIServer(obj).GetInto
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
		client.GetIntoFunc = newFinalizerAPIServer(obj).GetInto
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
		client.GetIntoFunc = newFinalizerAPIServer(obj).GetInto
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
		client.GetIntoFunc = newFinalizerAPIServer(obj).GetInto
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
		client.GetIntoFunc = newFinalizerAPIServer(obj).GetInto
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
		addedVersion := ""
		o.AddFunc = func(c context.Context, object resource.Object) error {
			addCalled = true
			addedVersion = object.GetResourceVersion()
			return nil
		}
		err := o.Add(context.TODO(), obj)
		assert.NoError(t, err)
		assert.Equal(t, 4, attempts)                            // 3 attempts for the WIP finalizer, then 1 for the final finalizer
		assert.Equal(t, obj.GetResourceVersion(), addedVersion) // The callback gets the version of the event payload.
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

	t.Run("same generation status update", func(t *testing.T) {
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
		old.SetResourceVersion("1")
		new.SetResourceVersion("2")
		old.SetFinalizers([]string{o.finalizer})
		new.SetFinalizers([]string{o.finalizer})
		o.SyncFunc = func(ctx context.Context, object resource.Object) error {
			assert.Fail(t, "sync should not be called for a status update")
			return nil
		}
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
		client.GetIntoFunc = newFinalizerAPIServer(obj).GetInto
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
		client.GetIntoFunc = newFinalizerAPIServer(obj).GetInto
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
			assert.Equal(t, strconv.Itoa(attempts), request.Operations[1].Value)
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
			copyFinalizerState(obj, o)
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
		assert.Empty(t, obj.GetResourceVersion(), "conflict retries must not mutate the informer event")
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

func TestOpinionatedWatcher_Update_SpecChangesDuringAdd(t *testing.T) {
	kind := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
	created := kind.ZeroValue()
	created.SetName("test")
	created.SetUID("test-uid")
	created.SetGeneration(1)
	created.SetResourceVersion("1")
	require.NoError(t, created.SetSpec("original"))
	watcher, server := newTestWatcherWithServer(t, kind, created)

	addCalls, updateCalls := 0, 0
	changeSpec := func(spec string, generation int64) {
		server.change(func(obj resource.Object) {
			require.NoError(t, obj.SetSpec(spec))
			obj.SetGeneration(generation)
		})
	}
	watcher.AddFunc = func(_ context.Context, obj resource.Object) error {
		addCalls++
		require.Equal(t, "original", obj.GetSpec())
		changeSpec("changed during add", 2)
		return nil
	}
	watcher.SyncFunc = func(context.Context, resource.Object) error {
		t.Error("the spec change should reach UpdateFunc")
		return nil
	}
	updateErr := fmt.Errorf("downstream unavailable")
	watcher.UpdateFunc = func(_ context.Context, src, tgt resource.Object) error {
		updateCalls++
		assert.Equal(t, "original", src.GetSpec())
		current := server.state()
		assert.Equal(t, current.GetSpec(), tgt.GetSpec())
		assert.Equal(t, current.GetResourceVersion(), tgt.GetResourceVersion(),
			"a status write must use the version after the finalizer patch")
		if updateCalls == 1 {
			return updateErr
		}
		return nil
	}

	require.NoError(t, watcher.Add(t.Context(), created))
	changes := server.changesFrom(0)
	require.Len(t, changes, 3, "expect the pending finalizer, spec change, and completed finalizer")
	require.NoError(t, watcher.Update(t.Context(), changes[0][0], changes[0][1]))
	require.ErrorIs(t, watcher.Update(t.Context(), changes[1][0], changes[1][1]), updateErr)
	require.NoError(t, watcher.Update(t.Context(), changes[2][0], changes[2][1]))
	require.Equal(t, 1, updateCalls, "finalizer-only notifications should not reach UpdateFunc")

	// Another writer changes the spec before the retry, but the watch has not
	// delivered that notification yet. Retrying the old update must read again.
	changeSpec("changed before retry", 3)
	require.NoError(t, watcher.Update(t.Context(), changes[1][0], changes[1][1]))
	assert.Equal(t, 1, addCalls)
	assert.Equal(t, 2, updateCalls)
}

func TestOpinionatedWatcher_Update_ConfigMapChangesDuringAdd(t *testing.T) {
	type wrapped = resource.WrappedObject[*corev1.ConfigMap]
	for _, test := range []struct {
		name        string
		change      func(*corev1.ConfigMap)
		relist      bool
		wantUpdates int
	}{
		{"data changes while Add is pending", func(cm *corev1.ConfigMap) { cm.Data["key"] = "new" }, false, 1},
		{"relist includes binary data and completed finalizer", func(cm *corev1.ConfigMap) { cm.BinaryData = map[string][]byte{"key": {1}} }, true, 1},
		{"only metadata changes", func(cm *corev1.ConfigMap) { cm.SetLabels(map[string]string{"updated": "true"}) }, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			obj := resource.NewWrappedObject(&corev1.ConfigMap{Data: map[string]string{"key": "old"}})
			obj.SetName("item")
			obj.SetUID("original")
			obj.SetResourceVersion("1")
			schema := resource.NewSimpleSchema("", "v1", obj, &resource.TypedList[*wrapped]{}, resource.WithKind("ConfigMap"))
			server := newFinalizerAPIServer(obj)
			client := &mockPatchClient{PatchIntoFunc: server.PatchInto, GetIntoFunc: func(_ context.Context, _ resource.Identifier, into resource.Object) error {
				// The concurrent write makes the finalizer patch refresh its version.
				copyFinalizerState(server.state(), into)
				return nil
			}}
			watcher, err := NewOpinionatedWatcher(schema, client, OpinionatedWatcherConfig{})
			require.NoError(t, err)
			var applied *corev1.ConfigMap
			var updates int
			watcher.AddFunc = func(_ context.Context, target resource.Object) error {
				applied = target.(*wrapped).Object.DeepCopy()
				server.change(func(obj resource.Object) { test.change(obj.(*wrapped).Object) })
				return nil
			}
			watcher.UpdateFunc = func(_ context.Context, _, target resource.Object) error {
				applied = target.(*wrapped).Object.DeepCopy()
				updates++
				return nil
			}

			require.NoError(t, watcher.Add(t.Context(), obj))
			changes := server.changesFrom(0)
			require.Len(t, changes, 3, "pending finalizer, concurrent change, completed finalizer")
			if test.relist {
				// The relist advances straight from the pending to the completed finalizer.
				require.NoError(t, watcher.Update(t.Context(), changes[0][1], changes[2][1]))
			} else {
				for _, change := range changes {
					require.NoError(t, watcher.Update(t.Context(), change[0], change[1]))
				}
			}
			current := server.state().(*wrapped).Object
			require.Equal(t, test.wantUpdates, updates)
			require.Equal(t, current.Data, applied.Data)
			require.Equal(t, current.BinaryData, applied.BinaryData)
		})
	}
}

func TestAddSpecChanged_WrappedPayload(t *testing.T) {
	// These adjacent int64 values become identical if decoded through float64.
	const value = int64(1) << 53
	src := resource.NewWrappedObject(&unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1",
		"kind":       "Counter",
		"value":      value,
	}})
	target := src.Copy().(*resource.WrappedObject[*unstructured.Unstructured])
	target.Object.Object["status"] = map[string]any{"observed": value}
	require.False(t, addSpecChanged(src, target), "a status update does not require Add catch-up")
	target.Object.Object["value"] = value + 1
	require.True(t, addSpecChanged(src, target), "a changed payload does require catch-up")
}

func TestOpinionatedWatcher_Update_SpecChangeWithCompletedFinalizer(t *testing.T) {
	kind := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
	watcher, err := NewOpinionatedWatcher(kind, &mockPatchClient{}, OpinionatedWatcherConfig{})
	require.NoError(t, err)
	// A relist can report both Add completion and a spec change even when
	// storage leaves generation unset.
	old := kind.ZeroValue()
	old.SetFinalizers([]string{watcher.addPendingFinalizer})
	old.SetResourceVersion("1")
	require.NoError(t, old.SetSpec("original"))
	current := old.Copy()
	current.SetFinalizers([]string{watcher.finalizer})
	current.SetResourceVersion("2")
	require.NoError(t, current.SetSpec("changed"))
	updateCalls := 0
	watcher.UpdateFunc = func(_ context.Context, src, tgt resource.Object) error {
		updateCalls++
		assert.Same(t, old, src)
		assert.Same(t, current, tgt)
		return nil
	}
	require.NoError(t, watcher.Update(t.Context(), old, current))
	assert.Equal(t, 1, updateCalls)
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

func TestOpinionatedWatcher_Update_ReplacementDuringCleanup(t *testing.T) {
	schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
	client := &mockPatchClient{}
	watcher, err := NewOpinionatedWatcher(schema, client, OpinionatedWatcherConfig{})
	require.NoError(t, err)
	original := schema.ZeroValue()
	original.SetName("test")
	original.SetUID("original")
	original.SetResourceVersion("1")
	original.SetFinalizers([]string{watcher.finalizer, watcher.addPendingFinalizer})
	now := metav1.Now()
	original.SetDeletionTimestamp(&now)
	server := newFinalizerAPIServer(original)
	client.GetIntoFunc = server.GetInto
	client.PatchIntoFunc = server.PatchInto
	watcher.DeleteFunc = func(context.Context, resource.Object) error {
		// The same name now belongs to a replacement that also started
		// deleting. Only the UID distinguishes it from the original cleanup.
		server.change(func(obj resource.Object) {
			obj.SetUID("replacement")
			replacementDeletion := metav1.Now()
			obj.SetDeletionTimestamp(&replacementDeletion)
		})
		return nil
	}
	require.NoError(t, watcher.Update(t.Context(), original.Copy(), original.Copy()))
	assert.Equal(t, original.GetFinalizers(), server.state().GetFinalizers())
}

func TestOpinionatedWatcher_Add_StaleObjects(t *testing.T) {
	// A queued watch notification or retry can still hold an old finalizer list.
	const otherFinalizer = "other.example/cleanup"
	type fixture struct {
		watcher *OpinionatedWatcher
		server  *finalizerAPIServer
		// created is the object the create event hands to Add.
		created resource.Object
	}
	newFixture := func(t *testing.T, finalizers func(finalizer, inProgress string) []string) *fixture {
		sch := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[string]{}, &resource.TypedList[*resource.TypedSpecObject[string]]{})
		created := sch.ZeroValue()
		created.SetName("test")
		created.SetGeneration(1)
		created.SetResourceVersion("1")
		if finalizers != nil {
			created.SetFinalizers(finalizers(DefaultFinalizerSupplier(sch), InProgressFinalizerSupplier(sch)))
		}
		watcher, server := newTestWatcherWithServer(t, sch, created)
		// Each case replaces the handlers it expects to be called.
		watcher.AddFunc = func(context.Context, resource.Object) error {
			t.Error("AddFunc should not be called")
			return nil
		}
		watcher.UpdateFunc = func(context.Context, resource.Object, resource.Object) error {
			t.Error("UpdateFunc should not be called")
			return nil
		}
		watcher.DeleteFunc = func(context.Context, resource.Object) error {
			t.Error("DeleteFunc should not be called")
			return nil
		}
		watcher.SyncFunc = func(context.Context, resource.Object) error {
			t.Error("SyncFunc should not be called")
			return nil
		}
		return &fixture{watcher: watcher, server: server, created: created}
	}
	// replay delivers every recorded change through Update, including changes made while delivering. The informer
	// cache hands handlers its own objects, so created is the first update's old and each update's new is the next
	// one's old: anything a handler writes into an object it was given shows up in the next update, as in production.
	replay := func(t *testing.T, f *fixture) {
		cached := f.created
		for delivered := 0; ; {
			changes := f.server.changesFrom(delivered)
			if len(changes) == 0 {
				return
			}
			for _, change := range changes {
				next := change[1]
				require.NoError(t, f.watcher.Update(t.Context(), cached, next))
				cached = next
				delivered++
			}
			require.Less(t, delivered, 20, "delivering changes keeps making new ones")
		}
	}

	t.Run("watch notifications after a successful Add", func(t *testing.T) {
		f := newFixture(t, nil)
		addCalls := 0
		f.watcher.AddFunc = func(context.Context, resource.Object) error {
			addCalls++
			return nil
		}
		require.NoError(t, f.watcher.Add(t.Context(), f.created))
		// The first change only adds the in-progress finalizer, so delivering it is the case under test.
		require.Equal(t, []string{f.watcher.addPendingFinalizer}, f.server.changesFrom(0)[0][1].GetFinalizers())

		replay(t, f)
		assert.Equal(t, 1, addCalls)
		assert.Equal(t, []string{f.watcher.finalizer}, f.server.state().GetFinalizers())
	})

	t.Run("leftover in-progress finalizer removed on startup", func(t *testing.T) {
		// Add handles an object carrying both of our finalizers by calling Sync and removing the in-progress one.
		// Delivering that removal must not look like a resync, which would call Sync again.
		f := newFixture(t, func(finalizer, inProgress string) []string { return []string{finalizer, inProgress} })
		syncCalls := 0
		f.watcher.SyncFunc = func(context.Context, resource.Object) error {
			syncCalls++
			return nil
		}
		original := f.created.Copy()
		require.NoError(t, f.watcher.Add(t.Context(), f.created))
		assert.Equal(t, original, f.created, "the patch must leave the informer snapshot intact")

		replay(t, f)
		assert.Equal(t, 1, syncCalls)
		assert.Equal(t, []string{f.watcher.finalizer}, f.server.state().GetFinalizers())
	})

	t.Run("conflicting Add preserves another owner's finalizer", func(t *testing.T) {
		// Another owner adds its finalizer before Add handles the create
		// event. The retry must preserve that finalizer when it patches.
		f := newFixture(t, nil)
		f.server.change(func(obj resource.Object) { obj.SetFinalizers(append(obj.GetFinalizers(), otherFinalizer)) })
		addCalls := 0
		f.watcher.AddFunc = func(context.Context, resource.Object) error {
			addCalls++
			return nil
		}
		require.NoError(t, f.watcher.Add(t.Context(), f.created))
		assert.Equal(t, 1, addCalls)
		assert.Equal(t, []string{otherFinalizer, f.watcher.finalizer}, f.server.state().GetFinalizers())
	})
}

func TestOpinionatedWatcher_Add_RefreshesRemovedSpecFields(t *testing.T) {
	for _, test := range []struct {
		name             string
		pendingFinalizer bool
		admissionChanges bool
		sharedFinalizer  bool
	}{
		{name: "pending-finalizer recovery", pendingFinalizer: true},
		{name: "pending-finalizer patch conflicts"},
		{name: "successful pending-finalizer patch changes spec", admissionChanges: true},
		{name: "shared finalizer still allows the initial Add", admissionChanges: true, sharedFinalizer: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			type spec = map[string]string
			schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[spec]{}, &resource.TypedList[*resource.TypedSpecObject[spec]]{})
			event := schema.ZeroValue()
			event.SetName("test")
			event.SetUID("test-uid")
			event.SetResourceVersion("1")
			event.SetGeneration(1)
			if test.pendingFinalizer {
				event.SetFinalizers([]string{InProgressFinalizerSupplier(schema)})
			}
			originalSpec := spec{"removed": "old", "kept": "old"}
			require.NoError(t, event.SetSpec(originalSpec))
			original := event.Copy()
			server := newFinalizerAPIServer(event)
			currentSpec := spec{"kept": "new"}
			changeSpec := func(obj resource.Object) {
				require.NoError(t, obj.SetSpec(currentSpec))
				obj.SetGeneration(2)
			}
			if test.admissionChanges {
				// Admission changes the spec in the same write that installs
				// the pending finalizer. The patch returns only metadata.
				server.admitPatch = func(obj resource.Object) {
					if slices.Contains(obj.GetFinalizers(), InProgressFinalizerSupplier(schema)) {
						changeSpec(obj)
					}
				}
			} else {
				server.change(changeSpec)
			}

			// A fresh GET first decodes JSON. Decoding it over the event's
			// spec would keep the map key the server removed.
			codec := resource.NewJSONCodec()
			client := &mockPatchClient{
				PatchIntoFunc: server.PatchInto,
				GetIntoFunc: func(_ context.Context, id resource.Identifier, into resource.Object) error {
					assert.Equal(t, event.GetStaticMetadata().Identifier(), id)
					var response bytes.Buffer
					if err := codec.Write(&response, server.state()); err != nil {
						return err
					}
					return codec.Read(&response, into)
				},
			}
			config := OpinionatedWatcherConfig{}
			if test.sharedFinalizer {
				config.Finalizer = InProgressFinalizerSupplier
			}
			watcher, err := NewOpinionatedWatcher(schema, &cleanupReadClient{client}, config)
			require.NoError(t, err)
			addCalls := 0
			watcher.AddFunc = func(_ context.Context, obj resource.Object) error {
				addCalls++
				assert.Equal(t, currentSpec, obj.GetSpec())
				assert.Equal(t, server.state().GetResourceVersion(), obj.GetResourceVersion())
				assert.Equal(t, []string{watcher.addPendingFinalizer}, obj.GetFinalizers())
				return nil
			}
			require.NoError(t, watcher.Add(t.Context(), event))
			assert.Equal(t, 1, addCalls)
			if !test.sharedFinalizer {
				assert.Equal(t, []string{watcher.finalizer}, server.state().GetFinalizers())
			}
			assert.Equal(t, original, event, "the event may still belong to the informer cache")
		})
	}
}

func TestOpinionatedWatcher_Update_RevalidatesAfterPendingFinalizerPatch(t *testing.T) {
	type spec = map[string]string
	schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[spec]{}, &resource.TypedList[*resource.TypedSpecObject[spec]]{})
	for _, test := range []struct {
		name        string
		change      func(resource.Object)
		readError   error
		wantCatchUp bool
		wantError   string
	}{
		{name: "different UID skips Add", change: func(obj resource.Object) { obj.SetUID("replacement") }},
		{name: "deletion skips Add", change: func(obj resource.Object) {
			now := metav1.Now()
			obj.SetDeletionTimestamp(&now)
		}},
		{name: "object is gone", readError: &apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}}},
		{name: "completed Add uses Update catch-up", change: func(obj resource.Object) {
			obj.SetFinalizers([]string{DefaultFinalizerSupplier(schema)})
		}, wantCatchUp: true},
		{name: "missing pending finalizer returns an error", change: func(obj resource.Object) {
			obj.SetFinalizers(nil)
		}, wantError: "missing in-progress finalizer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			old := schema.ZeroValue()
			old.SetName("test")
			old.SetUID("original")
			old.SetResourceVersion("1")
			old.SetGeneration(1)
			event := old.Copy()
			event.SetResourceVersion("2")
			event.SetGeneration(2)
			server := newFinalizerAPIServer(event)
			patchCalls := 0
			client := &mockPatchClient{
				PatchIntoFunc: func(ctx context.Context, id resource.Identifier, patch resource.PatchRequest, opts resource.PatchOptions, into resource.Object) error {
					patchCalls++
					return server.PatchInto(ctx, id, patch, opts, into)
				},
				GetIntoFunc: func(ctx context.Context, id resource.Identifier, into resource.Object) error {
					// The object changes between the successful patch and the read.
					require.Equal(t, []string{InProgressFinalizerSupplier(schema)}, server.state().GetFinalizers())
					if test.readError != nil {
						return test.readError
					}
					server.change(test.change)
					return server.GetInto(ctx, id, into)
				},
			}
			watcher, err := NewOpinionatedWatcher(schema, &cleanupReadClient{client}, OpinionatedWatcherConfig{})
			require.NoError(t, err)
			watcher.AddFunc = func(context.Context, resource.Object) error {
				t.Error("Add ran after the object ceased to need a protected Add")
				return nil
			}
			var caughtUp []resource.Object
			watcher.UpdateFunc = func(_ context.Context, _, current resource.Object) error {
				caughtUp = append(caughtUp, current)
				return nil
			}

			err = watcher.Update(t.Context(), old, event)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
			} else {
				require.NoError(t, err)
			}
			if test.wantCatchUp {
				require.Len(t, caughtUp, 1)
				assert.Equal(t, server.state().GetResourceVersion(), caughtUp[0].GetResourceVersion())
			} else {
				assert.Empty(t, caughtUp)
			}
			assert.Equal(t, 1, patchCalls, "the watcher must not replace a finalizer after skipping Add")
		})
	}
}

func TestOpinionatedWatcher_Add_RetriesAfterPostPatchReadError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		type spec = map[string]string
		schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecObject[spec]{}, &resource.TypedList[*resource.TypedSpecObject[spec]]{})
		event := schema.ZeroValue()
		event.SetName("test")
		event.SetUID("original")
		event.SetResourceVersion("1")
		server := newFinalizerAPIServer(event)
		readError := fmt.Errorf("apiserver unavailable")
		client := &mockPatchClient{
			PatchIntoFunc: server.PatchInto,
			GetIntoFunc: func(ctx context.Context, id resource.Identifier, into resource.Object) error {
				return readError
			},
		}
		watcher, err := NewOpinionatedWatcher(schema, &cleanupReadClient{client}, OpinionatedWatcherConfig{})
		require.NoError(t, err)
		addCalls := 0
		watcher.AddFunc = func(_ context.Context, obj resource.Object) error {
			addCalls++
			assert.Equal(t, []string{watcher.addPendingFinalizer}, obj.GetFinalizers())
			return nil
		}

		require.ErrorIs(t, watcher.Add(t.Context(), event), readError)
		require.Zero(t, addCalls)
		require.Equal(t, []string{watcher.addPendingFinalizer}, server.state().GetFinalizers())

		// The queued retry holds the original event. Its patch conflicts,
		// then it reads and reuses the already-installed pending finalizer.
		client.GetIntoFunc = server.GetInto
		require.NoError(t, watcher.Add(t.Context(), event))
		assert.Equal(t, 1, addCalls)
		assert.Equal(t, []string{watcher.finalizer}, server.state().GetFinalizers())
		assert.Len(t, server.changesFrom(0), 2, "only install the pending finalizer and replace it")
	})
}

func TestOpinionatedWatcher_Add_MetadataOnlyRefresh(t *testing.T) {
	schema := resource.NewSimpleSchema("group", "version", &resource.TypedSpecStatusObject[string, string]{}, &resource.TypedList[*resource.TypedSpecStatusObject[string, string]]{})
	event := schema.ZeroValue()
	event.SetName("item")
	event.SetNamespace("test")
	event.SetUID("original")
	event.SetResourceVersion("1")
	event.SetGeneration(1)
	event.SetFinalizers([]string{InProgressFinalizerSupplier(schema)})
	require.NoError(t, event.SetSpec("original spec"))
	require.NoError(t, event.SetSubresource("status", "original status"))
	server := newFinalizerAPIServer(event)
	server.change(func(obj resource.Object) {
		obj.SetGeneration(2)
		require.NoError(t, obj.SetSpec("new spec"))
	})
	// This client refreshes only lifecycle metadata. The later version can
	// guard a finalizer patch, but cannot describe the event's spec or status.
	client := &mockPatchClient{
		GetIntoFunc: func(_ context.Context, _ resource.Identifier, into resource.Object) error {
			into.SetCommonMetadata(server.state().GetCommonMetadata())
			return nil
		},
		PatchIntoFunc: func(ctx context.Context, id resource.Identifier, req resource.PatchRequest, opts resource.PatchOptions, into resource.Object) error {
			require.Equal(t, event.GetStaticMetadata().Identifier(), id)
			return server.PatchInto(ctx, id, req, opts, into)
		},
	}
	watcher, err := NewOpinionatedWatcher(schema, client, OpinionatedWatcherConfig{})
	require.NoError(t, err)
	var added resource.Object
	watcher.AddFunc = func(_ context.Context, obj resource.Object) error {
		added = obj.Copy()
		return nil
	}
	require.NoError(t, watcher.Add(t.Context(), event))
	require.Equal(t, event, added)
	require.Equal(t, []string{watcher.finalizer}, server.state().GetFinalizers())
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

// finalizerAPIServer stands in for the API server behind a PatchClient, so a test can follow one object through a
// sequence of changes. It applies the finalizer patches finalizerUpdater sends, answers a patch made against a stale
// resourceVersion with a conflict, and records each change as the (old, new) pair a watch delivers. It supports only
// the patch operations finalizerUpdater sends. Reads include the spec for UpdateFunc catch-up.
type finalizerAPIServer struct {
	mu         sync.Mutex
	current    resource.Object
	changes    [][2]resource.Object
	admitPatch func(resource.Object)
}

func newTestWatcherWithServer(t *testing.T, schema resource.Schema, obj resource.Object) (*OpinionatedWatcher, *finalizerAPIServer) {
	t.Helper()
	server := newFinalizerAPIServer(obj)
	watcher, err := NewOpinionatedWatcher(schema, server, OpinionatedWatcherConfig{})
	require.NoError(t, err)
	return watcher, server
}

func newFinalizerAPIServer(obj resource.Object) *finalizerAPIServer {
	return &finalizerAPIServer{current: obj.Copy()}
}

func (s *finalizerAPIServer) PatchInto(_ context.Context, _ resource.Identifier, req resource.PatchRequest, _ resource.PatchOptions, into resource.Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// finalizerUpdater sends the finalizer operation, then the resourceVersion it last read.
	if req.Operations[1].Value != s.current.GetResourceVersion() {
		return &apierrors.StatusError{ErrStatus: metav1.Status{Code: http.StatusConflict, Reason: metav1.StatusReasonConflict}}
	}
	finalizers := slices.Clone(s.current.GetFinalizers())
	op := req.Operations[0]
	switch op.Path {
	case "/metadata/finalizers":
		finalizers = slices.Clone(op.Value.([]string))
	case "/metadata/finalizers/-":
		finalizers = append(finalizers, op.Value.(string))
	default: // "/metadata/finalizers/<index>", replacing one entry
		i, err := strconv.Atoi(strings.TrimPrefix(op.Path, "/metadata/finalizers/"))
		if err != nil {
			return err
		}
		finalizers[i] = op.Value.(string)
	}
	s.commit(func(obj resource.Object) {
		obj.SetFinalizers(finalizers)
		if s.admitPatch != nil {
			s.admitPatch(obj)
		}
	})
	copyFinalizerState(s.current, into)
	return nil
}

func (s *finalizerAPIServer) Get(context.Context, resource.Identifier) (resource.Object, error) {
	return s.state(), nil
}

func (s *finalizerAPIServer) GetInto(_ context.Context, _ resource.Identifier, into resource.Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyFinalizerState(s.current, into)
	into.SetName(s.current.GetName())
	into.SetNamespace(s.current.GetNamespace())
	return into.SetSpec(s.current.GetSpec())
}

// change applies a change made by someone else, such as another owner adding its finalizer, and returns it as a
// watch delivers it.
func (s *finalizerAPIServer) change(apply func(resource.Object)) [2]resource.Object {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commit(apply)
	last := s.changes[len(s.changes)-1]
	return [2]resource.Object{last[0].Copy(), last[1].Copy()}
}

// markDeleting starts deleting the object, as a delete request does while finalizers remain. It leaves the generation
// unchanged, as some storage does, so only the deletion timestamp marks the update as a deletion.
func (s *finalizerAPIServer) markDeleting() [2]resource.Object {
	return s.change(func(obj resource.Object) {
		now := metav1.Now()
		obj.SetDeletionTimestamp(&now)
	})
}

// state returns the object as the server now holds it.
func (s *finalizerAPIServer) state() resource.Object {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current.Copy()
}

// changesFrom returns the changes recorded from index i on, as a watch delivers them.
func (s *finalizerAPIServer) changesFrom(i int) [][2]resource.Object {
	s.mu.Lock()
	defer s.mu.Unlock()
	changes := make([][2]resource.Object, 0, len(s.changes)-i)
	for _, change := range s.changes[i:] {
		changes = append(changes, [2]resource.Object{change[0].Copy(), change[1].Copy()})
	}
	return changes
}

// commit applies a change to a copy of the current object and records it. The caller holds mu.
func (s *finalizerAPIServer) commit(apply func(resource.Object)) {
	next := s.current.Copy()
	apply(next)
	version, _ := strconv.Atoi(s.current.GetResourceVersion())
	next.SetResourceVersion(strconv.Itoa(version + 1))
	s.changes = append(s.changes, [2]resource.Object{s.current, next})
	s.current = next
}

// copyFinalizerState writes the metadata the watcher reads from src into dst, as a client decodes a response into it.
func copyFinalizerState(src, dst resource.Object) {
	dst.SetUID(src.GetUID())
	dst.SetFinalizers(slices.Clone(src.GetFinalizers()))
	dst.SetResourceVersion(src.GetResourceVersion())
	dst.SetGeneration(src.GetGeneration())
	dst.SetDeletionTimestamp(src.GetDeletionTimestamp())
}
