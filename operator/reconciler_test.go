package operator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestNewOpinionatedReconciler(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		finalizer := "finalizer"
		op, err := NewOpinionatedReconciler(nil, finalizer)
		assert.Nil(t, op)
		assert.Equal(t, fmt.Errorf("client cannot be nil"), err)
	})

	t.Run("empty finalizer", func(t *testing.T) {
		client := &mockPatchClient{}
		op, err := NewOpinionatedReconciler(client, "")
		assert.Nil(t, op)
		assert.Equal(t, fmt.Errorf("finalizer cannot be empty"), err)
	})

	t.Run("success", func(t *testing.T) {
		finalizer := "finalizer"
		client := &mockPatchClient{}
		op, err := NewOpinionatedReconciler(client, finalizer)
		assert.Nil(t, err)
		assert.Equal(t, finalizer, op.finalizer)
		cast, ok := op.finalizerUpdater.(*finalizerUpdater)
		require.True(t, ok)
		assert.Equal(t, client, cast.client)
	})

	t.Run("finzlizer too long", func(t *testing.T) {
		finalizer := "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789"
		client := &mockPatchClient{}
		op, err := NewOpinionatedReconciler(client, finalizer)
		assert.Equal(t, fmt.Errorf("finalizer length cannot exceed 63 chars: %s", finalizer), err)
		assert.Nil(t, op)
	})
}

func TestOpinionatedReconciler_Reconcile(t *testing.T) {
	finalizer := "finalizer"
	t.Run("normal add", func(t *testing.T) {
		patchCalled := false
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: &resource.TypedSpecObject[int]{},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				assert.Equal(t, req.Object.GetStaticMetadata().Identifier(), identifier)
				assert.Equal(t, resource.PatchRequest{
					Operations: []resource.PatchOperation{{
						Path:      "/metadata/finalizers",
						Operation: resource.PatchOpAdd,
						Value:     []string{finalizer},
					}, {
						Path:      "/metadata/resourceVersion",
						Operation: resource.PatchOpReplace,
						Value:     object.GetResourceVersion(),
					}},
				}, request)
				patchCalled = true
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		result := ReconcileResult{
			// If we return a RequeueAfter then the finalizer doesn't get added, so we add a State here to uniquely
			// identify this ReconcileResult and ensure that the correct one is returned
			State: map[string]any{
				"foo": "bar",
			},
		}
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				assert.Equal(t, req, request)
				return result, nil
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, result, res)
		assert.Nil(t, err)
		assert.True(t, patchCalled)
	})

	t.Run("normal add with error", func(t *testing.T) {
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: &resource.TypedSpecObject[int]{},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				assert.Fail(t, "patch should not be called")
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		after := time.Second
		result := ReconcileResult{
			RequeueAfter: &after,
		}
		resErr := errors.New("I AM ERROR")
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				assert.Equal(t, req, request)
				return result, resErr
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, result, res)
		assert.Equal(t, resErr, err)
	})

	t.Run("normal add with client error", func(t *testing.T) {
		patchCalled := false
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: &resource.TypedSpecObject[int]{},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		patchErr := errors.New("I AM ERROR")
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				assert.Equal(t, req.Object.GetStaticMetadata().Identifier(), identifier)
				assert.Equal(t, resource.PatchRequest{
					Operations: []resource.PatchOperation{{
						Path:      "/metadata/finalizers",
						Operation: resource.PatchOpAdd,
						Value:     []string{finalizer},
					}, {
						Path:      "/metadata/resourceVersion",
						Operation: resource.PatchOpReplace,
						Value:     object.GetResourceVersion(),
					}},
				}, request)
				patchCalled = true
				return patchErr
			},
		}, finalizer)
		require.Nil(t, err)
		result := ReconcileResult{
			// If we return a RequeueAfter then the finalizer doesn't get added, so we add a State here to uniquely
			// identify this ReconcileResult and ensure that the correct one is returned
			State: map[string]any{
				"foo": "bar",
			},
		}
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				assert.Equal(t, req, request)
				return result, nil
			},
		}
		res, err := op.Reconcile(ctx, req)
		expectedPatchErr := NewFinalizerOperationError(patchErr, resource.PatchRequest{Operations: []resource.PatchOperation{{Path: "/metadata/finalizers", Operation: resource.PatchOpAdd, Value: []string{finalizer}}, {Path: "/metadata/resourceVersion", Operation: resource.PatchOpReplace, Value: req.Object.GetResourceVersion()}}})
		assert.Equal(t, expectedPatchErr, err)
		assert.True(t, patchCalled)
		assert.Equal(t, "bar", res.State["foo"])
		assert.Equal(t, expectedPatchErr, res.State[opinionatedReconcilerPatchAddStateKey])
	})

	t.Run("retried add from client error", func(t *testing.T) {
		patchCalled := false
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: &resource.TypedSpecObject[int]{},
			State: map[string]any{
				opinionatedReconcilerPatchAddStateKey: errors.New("I AM ERROR"),
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				assert.Equal(t, req.Object.GetStaticMetadata().Identifier(), identifier)
				assert.Equal(t, resource.PatchRequest{
					Operations: []resource.PatchOperation{{
						Path:      "/metadata/finalizers",
						Operation: resource.PatchOpAdd,
						Value:     []string{finalizer},
					}, {
						Path:      "/metadata/resourceVersion",
						Operation: resource.PatchOpReplace,
						Value:     object.GetResourceVersion(),
					}},
				}, request)
				patchCalled = true
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				assert.Fail(t, "Reconcile shouldn't be called")
				return ReconcileResult{}, nil
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, ReconcileResult{}, res)
		assert.Nil(t, err)
		assert.True(t, patchCalled)
	})

	t.Run("add to resync", func(t *testing.T) {
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: &resource.TypedSpecObject[int]{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{finalizer},
				},
			},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				assert.Fail(t, "patch should not be called")
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		after := time.Second
		result := ReconcileResult{
			RequeueAfter: &after,
		}
		resErr := errors.New("I AM ERROR")
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				assert.Equal(t, ReconcileActionResynced, request.Action)
				assert.Equal(t, req.Object, request.Object)
				assert.Equal(t, req.State, request.State)
				return result, resErr
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, result, res)
		assert.Equal(t, resErr, err)
	})

	t.Run("normal update", func(t *testing.T) {
		req := ReconcileRequest{
			Action: ReconcileActionUpdated,
			Object: &resource.TypedSpecObject[int]{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{finalizer},
				},
			},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				assert.Fail(t, "patch should not be called")
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		after := time.Second
		result := ReconcileResult{
			RequeueAfter: &after,
		}
		resErr := errors.New("I AM ERROR")
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				assert.Equal(t, req, request)
				return result, resErr
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, result, res)
		assert.Equal(t, resErr, err)
	})

	t.Run("update without finalizer", func(t *testing.T) {
		patchCalled := false
		req := ReconcileRequest{
			Action: ReconcileActionUpdated,
			Object: &resource.TypedSpecObject[int]{},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				assert.Equal(t, req.Object.GetStaticMetadata().Identifier(), identifier)
				assert.Equal(t, resource.PatchRequest{
					Operations: []resource.PatchOperation{{
						Path:      "/metadata/finalizers",
						Operation: resource.PatchOpAdd,
						Value:     []string{finalizer},
					}, {
						Path:      "/metadata/resourceVersion",
						Operation: resource.PatchOpReplace,
						Value:     object.GetResourceVersion(),
					}},
				}, request)
				patchCalled = true
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				assert.Fail(t, "Reconcile shouldn't be called")
				return ReconcileResult{}, nil
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, ReconcileResult{}, res)
		assert.Nil(t, err)
		assert.True(t, patchCalled)
	})

	t.Run("update with non-nil deletionTimestamp", func(t *testing.T) {
		patchCalled := false
		deleted := metav1.NewTime(time.Now())
		req := ReconcileRequest{
			Action: ReconcileActionUpdated,
			Object: &resource.TypedSpecObject[int]{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers:        []string{finalizer},
					DeletionTimestamp: &deleted,
				},
			},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				assert.Equal(t, req.Object.GetStaticMetadata().Identifier(), identifier)
				assert.Equal(t, resource.PatchRequest{
					Operations: []resource.PatchOperation{{
						Path:      "/metadata/finalizers",
						Operation: resource.PatchOpReplace,
						Value:     []string{},
					}, {
						Path:      "/metadata/resourceVersion",
						Operation: resource.PatchOpReplace,
						Value:     object.GetResourceVersion(),
					}},
				}, request)
				patchCalled = true
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		expRes := ReconcileResult{
			State: map[string]any{
				"bar": "foo",
			},
		}
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				// Request should have the Deleted action instead of Updated, but otherwise be the same
				assert.Equal(t, ReconcileActionDeleted, request.Action)
				assert.Equal(t, req.Object, request.Object)
				assert.Equal(t, req.State, request.State)
				return expRes, nil
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, expRes, res)
		assert.Nil(t, err)
		assert.True(t, patchCalled)
	})

	t.Run("update with non-nil deletionTimestamp, reconcile error", func(t *testing.T) {
		deleted := metav1.NewTime(time.Now())
		req := ReconcileRequest{
			Action: ReconcileActionUpdated,
			Object: &resource.TypedSpecObject[int]{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers:        []string{finalizer},
					DeletionTimestamp: &deleted,
				},
			},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				// No patch to remove finalizer if reconcile fails
				require.Fail(t, "patch should not be called")
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		reconcileErr := errors.New("I AM ERROR")
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				return ReconcileResult{}, reconcileErr
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, ReconcileResult{}, res)
		assert.Equal(t, reconcileErr, err)
	})

	t.Run("update with non-nil deletionTimestamp, patch error", func(t *testing.T) {
		deleted := metav1.NewTime(time.Now())
		req := ReconcileRequest{
			Action: ReconcileActionUpdated,
			Object: &resource.TypedSpecObject[int]{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers:        []string{finalizer},
					DeletionTimestamp: &deleted,
				},
			},
			State: map[string]any{
				"foo": "bar",
			},
		}
		patchErr := errors.New("I AM ERROR")
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				assert.Equal(t, req.Object.GetStaticMetadata().Identifier(), identifier)
				assert.Equal(t, resource.PatchRequest{
					Operations: []resource.PatchOperation{{
						Path:      "/metadata/finalizers",
						Operation: resource.PatchOpReplace,
						Value:     []string{},
					}, {
						Path:      "/metadata/resourceVersion",
						Operation: resource.PatchOpReplace,
						Value:     object.GetResourceVersion(),
					}},
				}, request)
				return patchErr
			},
		}, finalizer)
		require.Nil(t, err)
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				return ReconcileResult{}, nil
			},
		}
		res, err := op.Reconcile(ctx, req)
		expectedPatchErr := NewFinalizerOperationError(patchErr, resource.PatchRequest{Operations: []resource.PatchOperation{{Path: "/metadata/finalizers", Operation: resource.PatchOpReplace, Value: []string{}}, {Path: "/metadata/resourceVersion", Operation: resource.PatchOpReplace, Value: req.Object.GetResourceVersion()}}})
		assert.Equal(t, ReconcileResult{
			State: map[string]any{
				opinionatedReconcilerPatchRemoveStateKey: expectedPatchErr,
			},
		}, res)
		assert.Equal(t, expectedPatchErr, err)
	})

	t.Run("update with non-nil deletionTimestamp, missing finalizer", func(t *testing.T) {
		deleted := metav1.NewTime(time.Now())
		req := ReconcileRequest{
			Action: ReconcileActionUpdated,
			Object: &resource.TypedSpecObject[int]{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers:        []string{finalizer + "no"},
					DeletionTimestamp: &deleted,
				},
			},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				require.Fail(t, "patch should not be called")
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				require.Fail(t, "reconcile should not be called")
				return ReconcileResult{}, nil
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, ReconcileResult{}, res)
		assert.Nil(t, err)
	})

	t.Run("normal delete", func(t *testing.T) {
		req := ReconcileRequest{
			Action: ReconcileActionDeleted,
			Object: &resource.TypedSpecObject[int]{},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		op, err := NewOpinionatedReconciler(&mockPatchClient{
			PatchIntoFunc: func(c context.Context, identifier resource.Identifier, request resource.PatchRequest, options resource.PatchOptions, object resource.Object) error {
				require.Fail(t, "patch should not be called")
				return nil
			},
		}, finalizer)
		require.Nil(t, err)
		op.Reconciler = &SimpleReconciler{
			ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
				require.Fail(t, "reconcile should not be called")
				return ReconcileResult{}, nil
			},
		}
		res, err := op.Reconcile(ctx, req)
		assert.Equal(t, ReconcileResult{}, res)
		assert.Nil(t, err)
	})
}

func TestOpinionatedReconciler_Wrap(t *testing.T) {
	rr := ReconcileResult{
		State: map[string]any{
			"foo": "bar",
		},
	}
	rreq := ReconcileRequest{
		Action: ReconcileActionResynced,
		Object: &resource.TypedSpecObject[bool]{},
	}
	ctx := context.Background()
	myRec := &SimpleReconciler{
		ReconcileFunc: func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
			assert.Equal(t, rreq, request)
			return rr, nil
		},
	}
	op, err := NewOpinionatedReconciler(&mockPatchClient{}, "foo")
	assert.Nil(t, err)
	op.Wrap(myRec)
	res, err := op.Reconciler.Reconcile(ctx, rreq)
	assert.Nil(t, err)
	assert.Equal(t, rr, res)
}

func TestTypedReconciler_Reconcile(t *testing.T) {
	t.Run("nil ReconcileFunc", func(t *testing.T) {
		r := TypedReconciler[*resource.TypedSpecObject[string]]{}
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: &resource.TypedSpecObject[string]{},
			State: map[string]any{
				"foo": "bar",
			},
		}
		res, err := r.Reconcile(context.Background(), req)
		assert.Nil(t, err)
		assert.Equal(t, ReconcileResult{}, res)
	})

	t.Run("non-nil ReconcileFunc", func(t *testing.T) {
		r := TypedReconciler[*resource.TypedSpecObject[string]]{}
		obj := &resource.TypedSpecObject[string]{}
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: obj,
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		after := time.Second
		result := ReconcileResult{
			RequeueAfter: &after,
			State: map[string]any{
				"bar": "foo",
			},
		}
		recErr := errors.New("I AM ERROR")
		r.ReconcileFunc = func(c context.Context, request TypedReconcileRequest[*resource.TypedSpecObject[string]]) (ReconcileResult, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, req.Action, request.Action)
			assert.Equal(t, req.State, request.State)
			assert.Equal(t, obj, request.Object)
			return result, recErr
		}
		res, err := r.Reconcile(context.Background(), req)
		assert.Equal(t, err, recErr)
		assert.Equal(t, result, res)
	})

	t.Run("wrong type", func(t *testing.T) {
		r := TypedReconciler[*resource.TypedSpecObject[string]]{}
		obj := &resource.TypedSpecObject[int]{
			TypeMeta: metav1.TypeMeta{
				Kind:       "obj",
				APIVersion: "test/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
		}
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: obj,
			State: map[string]any{
				"foo": "bar",
			},
		}
		r.ReconcileFunc = func(c context.Context, request TypedReconcileRequest[*resource.TypedSpecObject[string]]) (ReconcileResult, error) {
			assert.Fail(t, "ReconcileFunc should not be called")
			return ReconcileResult{}, nil
		}
		res, err := r.Reconcile(context.Background(), req)
		assert.Equal(t, err, NewCannotCastError(obj.GetStaticMetadata()))
		assert.Equal(t, ReconcileResult{}, res)
	})
}

func TestSimpleReconciler_Reconcile(t *testing.T) {
	t.Run("nil ReconcileFunc", func(t *testing.T) {
		r := SimpleReconciler{}
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: &resource.TypedSpecObject[string]{},
			State: map[string]any{
				"foo": "bar",
			},
		}
		res, err := r.Reconcile(context.Background(), req)
		assert.Nil(t, err)
		assert.Equal(t, ReconcileResult{}, res)
	})

	t.Run("non-nil ReconcileFunc", func(t *testing.T) {
		r := SimpleReconciler{}
		req := ReconcileRequest{
			Action: ReconcileActionCreated,
			Object: &resource.TypedSpecObject[string]{},
			State: map[string]any{
				"foo": "bar",
			},
		}
		ctx := context.Background()
		after := time.Second
		result := ReconcileResult{
			RequeueAfter: &after,
			State: map[string]any{
				"bar": "foo",
			},
		}
		recErr := errors.New("I AM ERROR")
		r.ReconcileFunc = func(c context.Context, request ReconcileRequest) (ReconcileResult, error) {
			assert.Equal(t, ctx, c)
			assert.Equal(t, req, request)
			return result, recErr
		}
		res, err := r.Reconcile(context.Background(), req)
		assert.Equal(t, err, recErr)
		assert.Equal(t, result, res)
	})
}
