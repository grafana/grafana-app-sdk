package operator

import (
	"context"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
)

// ReconcileAction describes the action that triggered reconciliation.
type ReconcileAction int

const (
	ReconcileActionUnknown ReconcileAction = iota

	// Object was created
	ReconcileActionCreated

	// Object was updated
	ReconcileActionUpdated

	// Object was deleted
	ReconcileActionDeleted

	// Object was re-synced (i.e. periodic reconciliation)
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
