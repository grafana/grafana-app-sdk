package operator

import (
	"context"
	"fmt"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/utils/strings/slices"
)

// ReconcileAction describes the action that triggered reconciliation.
type ReconcileAction int

const (
	ReconcileActionUnknown ReconcileAction = iota

	// ReconcileActionCreated indicates that the resource to reconcile has been created.
	// Note that this action may also be used on initial start-up of some informer-based implementations,
	// such as the KubernetesBasedInformer. To instead receive Resync actions for these events,
	// use the OpinionatedReconciler.
	ReconcileActionCreated

	// ReconcileActionUpdated indicates that the resource to reconcile has been updated.
	ReconcileActionUpdated

	// ReconcileActionDeleted indicates that the resource to reconcile has been deleted.
	// Note that if the resource has Finalizers attached to it, a ReconcileActionUpdated will be used to indicate
	// "tombstoning" of the resource where DeletionTimestamp is non-nil and Finalizers may only be removed.
	// On completion of the actual delete from the API server once the Finalizers list is empty,
	// a Delete reconcile action will be triggered.
	ReconcileActionDeleted

	// ReconcileActionResynced indicates a periodic or initial re-sync of existing resources in the API server.
	// Note that not all implementations support this action (KubernetesBasedInformer will only trigger Created,
	// Updated, and Deleted actions. You can use OpinionatedReconciler to introduce Resync events on start instead
	// of Add events).
	ReconcileActionResynced
)

// ReconcileRequest contains the action which took place, and a snapshot of the object at that point in time.
// The Object in the ReconcileRequest is not guaranteed to be the current state of the object in-storage,
// as other actions may have taken place subsequently.
//
// Controllers such as InformerController contain logic to dequeue ReconcileRequests if subsequent actions
// are received for the same object.
type ReconcileRequest struct {
	Action ReconcileAction
	Object resource.Object
}

// ReconcileResult is the status of a successful Reconcile action.
// "Success" in this case simply indicates that unexpected errors did not occur,
// as the ReconcileResult can specify that the Reconcile action should be re-queued to run again
// after a period of time has elapsed.
type ReconcileResult struct {
	// RequeueAfter is a duration after which the Reconcile action which returned this result should be retried.
	// If nil, the Reconcile action will not be requeued.
	RequeueAfter *time.Duration
}

// Reconciler is an interface which describes an object which implements simple Reconciliation behavior.
type Reconciler interface {
	// Reconcile should be called whenever any action is received for a relevant object.
	// The action and object at the time the action was received are contained within the ReconcileRequest.
	// If the returned ReconcileResult has a non-nil RequeueAfter, the managing controller should requeue
	// the Reconcile action, with the same ReconcileRequest and context, after that duration has elapsed.
	// If the call returns an error, the Reconcile action should be requeued according to the retry policy
	// of the controller.
	Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileResult, error)
}

func ReconcileActionFromResourceAction(action ResourceAction) ReconcileAction {
	switch action {
	case ResourceActionCreate:
		return ReconcileActionCreated
	case ResourceActionUpdate:
		return ReconcileActionUpdated
	case ResourceActionDelete:
		return ReconcileActionDeleted
	default:
		return ReconcileActionUnknown
	}
}

func ResourceActionFromReconcileAction(action ReconcileAction) ResourceAction {
	switch action {
	case ReconcileActionCreated:
		return ResourceActionCreate
	case ReconcileActionUpdated:
		return ResourceActionUpdate
	case ReconcileActionDeleted:
		return ResourceActionDelete
	default:
		return ResourceAction("")
	}
}

type OpinionatedReconciler struct {
	ReconcileFunc func(context.Context, ReconcileRequest) (ReconcileResult, error)
	finalizer     string
	client        PatchClient
}

func (o *OpinionatedReconciler) Reconcile(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
	// Check if this action is a create, and the resource already has a finalizer. If so, make it a sync.
	if request.Action == ReconcileActionCreated && slices.Contains(request.Object.CommonMetadata().Finalizers, o.finalizer) {
		request.Action = ReconcileActionResynced
		return o.ReconcileFunc(ctx, request)
	}
	if request.Action == ReconcileActionCreated {
		// Delegate
		resp, err := o.wrappedReconcile(ctx, request)
		if err != nil || resp.RequeueAfter != nil {
			return resp, err
		}

		// Attach the finalizer on success
		patchErr := o.client.PatchInto(ctx, request.Object.StaticMetadata().Identifier(), resource.PatchRequest{
			Operations: []resource.PatchOperation{{
				Operation: resource.PatchOpAdd,
				Path:      "/metadata/finalizers",
				Value:     []string{o.finalizer},
			}},
		}, resource.PatchOptions{}, request.Object)
		// What to do with patch error???
		// TODO
		fmt.Println(patchErr)
		return resp, err
	}
	if request.Action == ReconcileActionUpdated && request.Object.CommonMetadata().DeletionTimestamp != nil && slices.Contains(request.Object.CommonMetadata().Finalizers, o.finalizer) {
		patchErr := o.client.PatchInto(ctx, request.Object.StaticMetadata().Identifier(), resource.PatchRequest{
			Operations: []resource.PatchOperation{{
				Operation: resource.PatchOpRemove,
				Path:      fmt.Sprintf("/metadata/finalizers/%d", slices.Index(request.Object.CommonMetadata().Finalizers, o.finalizer)),
			}},
		}, resource.PatchOptions{}, request.Object)
		return ReconcileResult{}, patchErr
	}
	if request.Action == ReconcileActionUpdated && !slices.Contains(request.Object.CommonMetadata().Finalizers, o.finalizer) {
		// Add the finalizer, don't delegate, let the reconcile action for adding the finalizer propagate down to avoid confusing extra reconciliations
		patchErr := o.client.PatchInto(ctx, request.Object.StaticMetadata().Identifier(), resource.PatchRequest{
			Operations: []resource.PatchOperation{{
				Operation: resource.PatchOpAdd,
				Path:      "/metadata/finalizers",
				Value:     []string{o.finalizer},
			}},
		}, resource.PatchOptions{}, request.Object)
		return ReconcileResult{}, patchErr
	}
	return o.wrappedReconcile(ctx, request)
}

func (o *OpinionatedReconciler) wrappedReconcile(ctx context.Context, request ReconcileRequest) (ReconcileResult, error) {
	if o.ReconcileFunc != nil {
		return o.ReconcileFunc(ctx, request)
	}
	return ReconcileResult{}, nil
}

func (o *OpinionatedReconciler) Wrap(reconciler Reconciler) {
	o.ReconcileFunc = reconciler.Reconcile
}

// Compile-time interface compliance check
var _ Reconciler = &OpinionatedReconciler{}
