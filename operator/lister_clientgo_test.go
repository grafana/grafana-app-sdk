package operator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestPagedListerPreservesCreationTimestampThroughPeriodicDelivery(t *testing.T) {
	created := metav1.NewTime(time.Now().Add(-time.Minute).Truncate(time.Second))
	list := &corev1.PodList{
		ListMeta: metav1.ListMeta{ResourceVersion: "100"},
		Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{
			Name: "recent", Namespace: "default", ResourceVersion: "98", CreationTimestamp: created,
		}, Spec: corev1.PodSpec{NodeName: "node-1"}}},
	}
	kind := resource.Kind{Schema: resource.NewSimpleSchema("", "v1", resource.NewWrappedObject(&corev1.Pod{}),
		&resource.UntypedList{}, resource.WithKind("Pod"), resource.WithPlural("pods"))}
	read := func(context.Context, metav1.ListOptions) (runtime.Object, error) { return list, nil }

	t.Run("periodic identity delivery keeps the age signal", func(t *testing.T) {
		lister, err := NewPagedLister(kind, read, PagedListerOptions{Detail: WatchDetailIdentity})
		require.NoError(t, err)
		informer, err := NewIdentityPeriodicInformer(kind, lister, PeriodicInformerOptions{})
		require.NoError(t, err)
		received := 0
		_, err = NewDeltaSource(informer).AddEventHandler(cache.ResourceEventHandlerFuncs{AddFunc: func(obj any) {
			pod, ok := obj.(*corev1.Pod)
			require.True(t, ok)
			require.Equal(t, created, pod.CreationTimestamp)
			require.Empty(t, pod.Spec.NodeName)
			received++
		}})
		require.NoError(t, err)
		require.NoError(t, informer.sweep(t.Context()))
		require.Equal(t, 1, received)
	})

	t.Run("full mode wraps a native object with its spec", func(t *testing.T) {
		lister, err := NewPagedLister(kind, read, PagedListerOptions{})
		require.NoError(t, err)
		result, rv, err := lister.List(t.Context())
		require.NoError(t, err)
		require.Equal(t, "100", rv)
		require.Len(t, result, 1)
		pod, ok := clientGoObject(result[0]).(*corev1.Pod)
		require.True(t, ok)
		require.Equal(t, "node-1", pod.Spec.NodeName)
		require.Equal(t, created, pod.CreationTimestamp)
	})

	t.Run("full mode decodes a native object into an SDK type", func(t *testing.T) {
		decodedKind := resource.Kind{
			Schema: resource.NewSimpleSchema("", "v1", &resource.UntypedObject{},
				&resource.UntypedList{}, resource.WithKind("Pod"), resource.WithPlural("pods")),
			Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
		}
		lister, err := NewPagedLister(decodedKind, read, PagedListerOptions{})
		require.NoError(t, err)
		result, _, err := lister.List(t.Context())
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, decodedKind.GroupVersionKind(), result[0].GroupVersionKind())
		require.Equal(t, created, result[0].GetCreationTimestamp())
		spec, ok := result[0].GetSpec().(map[string]any)
		require.True(t, ok)
		require.Equal(t, "node-1", spec["nodeName"])
	})
}

func TestClientGoAdapterUnwrapsDynamicObjects(t *testing.T) {
	native := &unstructured.Unstructured{}
	native.SetName("dynamic")
	kind := resource.Kind{Schema: resource.NewSimpleSchema("test.example", "v1",
		resource.NewUnstructuredWrapper(&unstructured.Unstructured{}), &resource.UntypedList{},
		resource.WithKind("Dynamic"), resource.WithPlural("dynamics"))}
	native.SetGroupVersionKind(kind.GroupVersionKind())
	lister, err := NewPagedLister(kind, func(context.Context, metav1.ListOptions) (runtime.Object, error) {
		return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*native}}, nil
	}, PagedListerOptions{})
	require.NoError(t, err)
	listed, _, err := lister.List(t.Context())
	require.NoError(t, err)
	require.Len(t, listed, 1)
	var added, deleted any
	adapter := clientGoWatcher{handler: cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { added = obj },
		DeleteFunc: func(obj any) { deleted = obj },
	}}
	require.NoError(t, adapter.Add(ContextWithDelivery(t.Context(), DeliveryLive), listed[0]))
	require.Equal(t, native, added)
	require.NoError(t, adapter.Delete(ContextWithDelivery(t.Context(), DeliveryLive), listed[0]))
	require.Equal(t, native, deleted)
	require.NoError(t, adapter.Delete(ContextWithDelivery(t.Context(), DeliveryRelist), listed[0]))
	deletion, ok := deleted.(cache.DeletedFinalStateUnknown)
	require.True(t, ok)
	require.Equal(t, native, deletion.Obj)
}

func TestPagedListerNormalizesSDKObjects(t *testing.T) {
	kind := resource.Kind{Schema: resource.NewSimpleSchema("example.com", "v1", &resource.UntypedObject{},
		&resource.UntypedList{}, resource.WithKind("Widget")),
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
	matching := &resource.UntypedObject{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Widget"},
		ObjectMeta: metav1.ObjectMeta{Name: "matching"},
	}
	different := &resource.TypedSpecObject[map[string]any]{
		ObjectMeta: metav1.ObjectMeta{Name: "different"},
		Spec:       map[string]any{"value": "preserved"},
	}
	lister, err := NewPagedLister(kind, func(context.Context, metav1.ListOptions) (runtime.Object, error) {
		return &resource.UntypedList{Items: []resource.Object{matching, different}}, nil
	}, PagedListerOptions{})
	require.NoError(t, err)
	items, _, err := lister.List(t.Context())
	require.NoError(t, err)
	require.Len(t, items, 2)
	for _, item := range items {
		require.IsType(t, &resource.UntypedObject{}, item)
		require.Equal(t, kind.GroupVersionKind(), item.GroupVersionKind())
	}
	require.Equal(t, different.Spec, items[1].GetSpec())
	require.Equal(t, "v1", matching.APIVersion)
	require.Empty(t, different.GroupVersionKind())
}
