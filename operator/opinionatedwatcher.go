package operator

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/codes"
	"k8s.io/utils/strings/slices"

	"github.com/grafana/grafana-app-sdk/resource"
)

// PatchClient is a Client capable of making PatchInto requests. This is used by OpinionatedWatch to update finalizers.
type PatchClient interface {
	PatchInto(context.Context, resource.Identifier, resource.PatchRequest, resource.PatchOptions, resource.Object) error
}

// OpinionatedWatcher is a ResourceWatcher implementation that handles extra state logic,
// ensuring that downtime and restarts will not result in missed events.
// It does this via a few mechanisms is transparently handles for the user:
//
// It adds a finalizer for all newly-created resources.
// This ensures that deletes cannot complete until the finalizer is removed,
// so the event will not be missed if the operator is down.
//
// It only removes the finalizer after a successful call to `DeleteFunc`,
// which ensures that the resource is only deleted once the handler has succeeded.
//
// On startup, it is able to differentiate between `Add` events,
// which are newly-created resources the operator has not yet handled,
// and `Add` events which are previously-created resources that have already been handled by the operator.
// Fully new resources call the `AddFunc` handler,
// and previously-created call the `SyncFunc` handler.
//
// `Update` events which do not update anything in the spec or significant parts of the metadata are ignored.
//
// OpinionatedWatcher contains unexported fields, and must be created with NewOpinionatedWatcher
type OpinionatedWatcher struct {
	AddFunc    func(ctx context.Context, object resource.Object) error
	UpdateFunc func(ctx context.Context, old resource.Object, new resource.Object) error
	DeleteFunc func(ctx context.Context, object resource.Object) error
	SyncFunc   func(ctx context.Context, object resource.Object) error
	finalizer  string
	schema     resource.Schema
	client     PatchClient
}

// FinalizerSupplier represents a function that creates string finalizer from provider schema.
type FinalizerSupplier func(sch resource.Schema) string

// DefaultFinalizerSupplier crates finalizer following to pattern `operator.{version}.{kind}.{group}`.
func DefaultFinalizerSupplier(sch resource.Schema) string {
	return fmt.Sprintf("operator.%s.%s.%s", sch.Version(), sch.Kind(), sch.Group())
}

// NewOpinionatedWatcher sets up a new OpinionatedWatcher and returns a pointer to it.
func NewOpinionatedWatcher(sch resource.Schema, client PatchClient) (*OpinionatedWatcher, error) {
	return NewOpinionatedWatcherWithFinalizer(sch, client, DefaultFinalizerSupplier)
}

// NewOpinionatedWatcherWithFinalizer sets up a new OpinionatedWatcher with finalizer from provided supplier and returns a pointer to it.
func NewOpinionatedWatcherWithFinalizer(sch resource.Schema, client PatchClient, supplier FinalizerSupplier) (*OpinionatedWatcher, error) {
	if sch == nil {
		return nil, fmt.Errorf("schema cannot be nil")
	}
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}
	finalizer := supplier(sch)
	if len(finalizer) > 63 {
		return nil, fmt.Errorf("finalizer length cannot exceed 63 chars: %s", finalizer)
	}
	return &OpinionatedWatcher{
		client:    client,
		schema:    sch,
		finalizer: finalizer,
	}, nil
}

// Wrap wraps the Add, Update, and Delete calls in another ResourceWatcher by having the AddFunc call watcher.
// Add, UpdateFunc call watcher.Update, and DeleteFunc call watcher.Delete.
// If syncToAdd is true, SyncFunc will also call resource.Add. If it is false, SyncFunc will not be assigned.
func (o *OpinionatedWatcher) Wrap(watcher ResourceWatcher, syncToAdd bool) { // nolint: revive
	if watcher == nil {
		return
	}

	o.AddFunc = watcher.Add
	o.UpdateFunc = watcher.Update
	o.DeleteFunc = watcher.Delete
	if syncToAdd {
		o.SyncFunc = watcher.Add
	}
}

// Add is part of implementing ResourceWatcher,
// and calls the underlying AddFunc, SyncFunc, or DeleteFunc based upon internal logic.
// When the object is first added, AddFunc is called and a finalizer is attached to it.
// Subsequent calls to Add will check the finalizer list and call SyncFunc if the finalizer is already attached,
// or if ObjectMetadata.DeletionTimestamp is non-nil, they will call DeleteFunc and remove the finalizer
// (the finalizer prevents the resource from being hard deleted until it is removed).
func (o *OpinionatedWatcher) Add(ctx context.Context, object resource.Object) error {
	ctx, span := GetTracer().Start(ctx, "OpinionatedWatcher-add")
	defer span.End()
	if object == nil {
		span.SetStatus(codes.Error, "object cannot be nil")
		return fmt.Errorf("object cannot be nil")
	}

	finalizers := o.getFinalizers(object)

	// If we're pending deletion, check on the finalizers to see if it's waiting on us.
	// An "add" event would trigger if the informer was restart or resyncing,
	// so we may have missed the delete/update event.
	if object.GetDeletionTimestamp() != nil {
		span.AddEvent("object is deleted and pending finalizer removal")

		// Check if we're the finalizer it's waiting for. If we're not, we can drop this whole event.
		if !slices.Contains(finalizers, o.finalizer) {
			return nil
		}

		// Otherwise, we need to run our delete handler, then remove the finalizer
		err := o.deleteFunc(ctx, object)
		if err != nil {
			return err
		}

		// The remove finalizer code is shared by both our add and update handlers, as this logic can be hit from either
		err = o.removeFinalizer(ctx, object, finalizers)
		if err != nil {
			span.SetStatus(codes.Error, fmt.Sprintf("error removing finalizer: %s", err.Error()))
			return err
		}
		return nil
	}

	// Next, we need to check if our finalizer is already in the finalizer list.
	// If it is, we've already done the add logic on a previous run of the operator,
	// and this event is due to the list call on startup. In that case, we call our sync handler
	if slices.Contains(finalizers, o.finalizer) {
		return o.syncFunc(ctx, object)
	}

	// If this isn't a delete or an add we've seen before, then it's a new resource we need to handle appropriately.
	// Call the add handler, and if it returns successfully (no error), add the finalizer
	err := o.addFunc(ctx, object)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("watcher add error: %s", err.Error()))
		return err
	}

	// Add the finalizer
	err = o.addFinalizer(ctx, object, finalizers)
	if err != nil {
		return fmt.Errorf("error adding finalizer: %w", err)
	}
	return nil
}

// Update is part of implementing ResourceWatcher
// and calls the underlying UpdateFunc or DeleteFunc based on internal logic.
// If the new object has a non-nil ObjectMetadata.DeletionTimestamp in its metadata, DeleteFunc will be called,
// and the object's finalizer will be removed to allow kubernetes to hard delete it.
// Otherwise, UpdateFunc is called, provided the update is non-trivial (that is, the metadata.Generation has changed).
func (o *OpinionatedWatcher) Update(ctx context.Context, old resource.Object, new resource.Object) error {
	ctx, span := GetTracer().Start(ctx, "OpinionatedWatcher-update")
	defer span.End()
	// TODO: If old is nil, it _might_ be ok?
	if old == nil {
		return fmt.Errorf("old cannot be nil")
	}
	if new == nil {
		return fmt.Errorf("new cannot be nil")
	}

	// Only fire off Update if the generation has changed (so skip subresource updates)
	if new.GetGeneration() > 0 && old.GetGeneration() == new.GetGeneration() {
		return nil
	}

	// TODO: finalizers part of object metadata?
	oldFinalizers := o.getFinalizers(old)
	newFinalizers := o.getFinalizers(new)
	if !slices.Contains(newFinalizers, o.finalizer) && new.GetDeletionTimestamp() == nil {
		// Either the add somehow snuck past us (unlikely), or the original AddFunc call failed, and should be retried.
		// Either way, we need to try calling AddFunc
		err := o.addFunc(ctx, new)
		if err != nil {
			return err
		}
		// Add the finalizer (which also updates `new` inline)
		err = o.addFinalizer(ctx, new, newFinalizers)
		if err != nil {
			return fmt.Errorf("error adding finalizer: %w", err)
		}
	}

	// Check if the deletion timestamp is non-nil.
	// This denotes that the resource was deletes, but has one or more finalizers blocking it from actually deleting.
	if new.GetDeletionTimestamp() != nil {
		// If our finalizer is in the list, treat this as a delete.
		// Otherwise, drop the event and don't handle it as an update.
		if !slices.Contains(newFinalizers, o.finalizer) {
			return nil
		}

		// Call the delete handler, then remove the finalizer on success
		err := o.deleteFunc(ctx, new)
		if err != nil {
			span.SetStatus(codes.Error, fmt.Sprintf("watcher delete error: %s", err.Error()))
			return err
		}

		return o.removeFinalizer(ctx, new, newFinalizers)
	}

	// Check if this was us adding our finalizer. If it was, we can ignore it.
	if !slices.Contains(oldFinalizers, o.finalizer) && slices.Contains(newFinalizers, o.finalizer) {
		return nil
	}

	err := o.updateFunc(ctx, old, new)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("watcher update error: %s", err.Error()))
		return err
	}
	return nil
}

// Delete exists to implement ResourceWatcher,
// but, due to deletes only happening after the finalizer is removed, this function does nothing.
func (*OpinionatedWatcher) Delete(context.Context, resource.Object) error {
	// Do nothing here, because we add finalizers, so we actually call delete code on updates/add-sync
	return nil
}

// addFunc is a wrapper for AddFunc which makes a nil check to avoid panics
func (o *OpinionatedWatcher) addFunc(ctx context.Context, object resource.Object) error {
	if o.AddFunc != nil {
		return o.AddFunc(ctx, object)
	}
	// TODO: log?
	return nil
}

// updateFunc is a wrapper for UpdateFunc which makes a nil check to avoid panics
func (o *OpinionatedWatcher) updateFunc(ctx context.Context, old, new resource.Object) error {
	if o.UpdateFunc != nil {
		return o.UpdateFunc(ctx, old, new)
	}
	// TODO: log?
	return nil
}

// deleteFunc is a wrapper for DeleteFunc which makes a nil check to avoid panics
func (o *OpinionatedWatcher) deleteFunc(ctx context.Context, object resource.Object) error {
	if o.DeleteFunc != nil {
		return o.DeleteFunc(ctx, object)
	}
	// TODO: log?
	return nil
}

// syncFunc is a wrapper for SyncFunc which makes a nil check to avoid panics
func (o *OpinionatedWatcher) syncFunc(ctx context.Context, object resource.Object) error {
	if o.SyncFunc != nil {
		return o.SyncFunc(ctx, object)
	}
	// TODO: log?
	return nil
}

func (o *OpinionatedWatcher) addFinalizer(ctx context.Context, object resource.Object, finalizers []string) error {
	if slices.Contains(finalizers, o.finalizer) {
		// Finalizer already added
		return nil
	}

	return o.client.PatchInto(ctx, object.GetStaticMetadata().Identifier(), resource.PatchRequest{
		Operations: []resource.PatchOperation{{
			Operation: resource.PatchOpAdd,
			Path:      "/metadata/finalizers",
			Value:     []string{o.finalizer},
		}},
	}, resource.PatchOptions{}, object)
}

func (o *OpinionatedWatcher) removeFinalizer(ctx context.Context, object resource.Object, finalizers []string) error {
	if !slices.Contains(finalizers, o.finalizer) {
		// Finalizer already removed
		return nil
	}

	return o.client.PatchInto(ctx, object.GetStaticMetadata().Identifier(), resource.PatchRequest{
		Operations: []resource.PatchOperation{{
			Operation: resource.PatchOpRemove,
			Path:      fmt.Sprintf("/metadata/finalizers/%d", slices.Index(finalizers, o.finalizer)),
		}},
	}, resource.PatchOptions{}, object)
}

func (*OpinionatedWatcher) getFinalizers(object resource.Object) []string {
	if object.GetFinalizers() != nil {
		return object.GetFinalizers()
	}
	return make([]string, 0)
}
