package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/resource"
)

// PatchClient is a Client capable of making PatchInto requests. This is used by OpinionatedWatch to update finalizers.
type PatchClient interface {
	PatchInto(context.Context, resource.Identifier, resource.PatchRequest, resource.PatchOptions, resource.Object) error
	// GetInto refreshes finalizer metadata. Implement resource.Client's Get
	// to supply fresh callback payloads, for example by decoding into a fresh
	// schema.ZeroValue(). Otherwise watcher callbacks use the event snapshot.
	GetInto(context.Context, resource.Identifier, resource.Object) error
}

// Full-object readers can return a fresh decode of the resource. Other patch
// clients need only refresh finalizer metadata.
type fullObjectReader interface {
	Get(context.Context, resource.Identifier) (resource.Object, error)
}

// FinalizerUpdater is an interface which describes a type which can manipulate finalizers for an object,
// Updating them in the API server and updating the provided object with the current state of the entire object after the update.
type FinalizerUpdater interface {
	// AddFinalizer adds a finalizer to the provided object, and serializes the updated object back into the `obj` parameter
	AddFinalizer(ctx context.Context, obj resource.Object, finalizer string) error
	// ReplaceFinalizer replaces a finalizer in the provided object, and serializes the updated object back into the `obj` parameter
	ReplaceFinalizer(ctx context.Context, obj resource.Object, toReplace, replaceWith string) error
	// RemoveFinalizer removes a finalizer from the provided object, and serializes the updated object back into the `obj` parameter
	RemoveFinalizer(ctx context.Context, obj resource.Object, finalizer string) error
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
	AddFunc             func(ctx context.Context, object resource.Object) error
	UpdateFunc          func(ctx context.Context, src resource.Object, tgt resource.Object) error
	DeleteFunc          func(ctx context.Context, object resource.Object) error
	SyncFunc            func(ctx context.Context, object resource.Object) error
	finalizer           string
	addPendingFinalizer string
	schema              resource.Schema
	collectors          []prometheus.Collector
	finalizerUpdater    finalizerUpdater
}

// FinalizerSupplier represents a function that creates string finalizer from provider schema.
type FinalizerSupplier func(sch resource.Schema) string

// DefaultFinalizerSupplier crates finalizer following to pattern `operator.{version}.{kind}.{group}`.
func DefaultFinalizerSupplier(sch resource.Schema) string {
	return fmt.Sprintf("operator.%s.%s.%s", sch.Version(), sch.Kind(), sch.Group())
}

func InProgressFinalizerSupplier(sch resource.Schema) string {
	return fmt.Sprintf("wip.%s.%s.%s", sch.Version(), sch.Kind(), sch.Group())
}

// OpinionatedWatcherConfig contains configuration options for creating a new OpinionatedWatcher
type OpinionatedWatcherConfig struct {
	// Finalizer supplies the finalizer used for the OpinionatedWatcher.
	// If nil, DefaultFinalizerSupplier will be used.
	Finalizer FinalizerSupplier
	// InProgressFinalizer supplies the finalizer used to indicate an incomplete action and prevent deletes until completion.
	// If nil, InProgressFinalizerSupplier will be used.
	InProgressFinalizer FinalizerSupplier
}

// NewOpinionatedWatcher sets up a new OpinionatedWatcher and returns a pointer to it.
func NewOpinionatedWatcher(sch resource.Schema, client PatchClient, config OpinionatedWatcherConfig) (*OpinionatedWatcher, error) {
	if sch == nil {
		return nil, errors.New("schema cannot be nil")
	}
	if client == nil {
		return nil, errors.New("client cannot be nil")
	}
	supplier := config.Finalizer
	if supplier == nil {
		supplier = DefaultFinalizerSupplier
	}
	finalizer := supplier(sch)
	if len(finalizer) > 63 {
		return nil, fmt.Errorf("finalizer length cannot exceed 63 chars: %s", finalizer)
	}
	pendingAddSupplier := config.InProgressFinalizer
	if pendingAddSupplier == nil {
		pendingAddSupplier = InProgressFinalizerSupplier
	}
	pendingAddFinalizer := pendingAddSupplier(sch)
	if len(pendingAddFinalizer) > 63 {
		return nil, fmt.Errorf("in-progress finalizer length cannot exceed 63 chars: %s", finalizer)
	}
	return &OpinionatedWatcher{
		finalizerUpdater:    *newFinalizerUpdater(client),
		schema:              sch,
		finalizer:           finalizer,
		addPendingFinalizer: pendingAddFinalizer,
		collectors:          make([]prometheus.Collector, 0),
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

	if cast, ok := watcher.(metrics.Provider); ok {
		o.collectors = append(o.collectors, cast.PrometheusCollectors()...)
	}
}

// Add is part of implementing ResourceWatcher,
// and calls the underlying AddFunc, SyncFunc, or DeleteFunc based upon internal logic.
// When the object is first added, AddFunc is called and a finalizer is attached to it.
// Subsequent calls to Add will check the finalizer list and call SyncFunc if the finalizer is already attached,
// or if ObjectMetadata.DeletionTimestamp is non-nil, they will call DeleteFunc and remove the finalizer
// (the finalizer prevents the resource from being hard deleted until it is removed).
// An object Add has to add may be stale, so AddFunc is only called once the current state shows the add still has to
// happen; Add returns without calling it when an earlier attempt finished or the object is being deleted.
//
//nolint:funlen
func (o *OpinionatedWatcher) Add(ctx context.Context, object resource.Object) error {
	ctx, span := GetTracer().Start(ctx, "OpinionatedWatcher-add")
	defer span.End()
	if object == nil {
		span.SetStatus(codes.Error, "object cannot be nil")
		return errors.New("object cannot be nil")
	}

	logger := logging.FromContext(ctx).With("action", "add", "component", "OpinionatedWatcher", "kind", object.GroupVersionKind().Kind, "namespace", object.GetNamespace(), "name", object.GetName())
	logger.Debug("Handling add")

	finalizers := o.getFinalizers(object)

	// If we're pending deletion, check on the finalizers to see if it's waiting on us.
	// An "add" event would trigger if the informer was restart or resyncing,
	// so we may have missed the delete/update event.
	if object.GetDeletionTimestamp() != nil {
		return o.cleanup(ctx, object)
	}

	// Next, we need to check if our finalizer is already in the finalizer list.
	// If it is, we've already done the add logic on a previous run of the operator,
	// and this event is due to the list call on startup. In that case, we call our sync handler
	if slices.Contains(finalizers, o.finalizer) {
		span.AddEvent("object has watcher finalizer")
		logger.Debug("Object has our finalizer, calling Sync", "finalizers", finalizers)
		err := o.syncFunc(ctx, object)
		if err != nil {
			span.SetStatus(codes.Error, fmt.Sprintf("watcher sync error: %s", err.Error()))
			return err
		}
		// If we somehow still have the add-pending finalizer, remove it
		if slices.Contains(finalizers, o.addPendingFinalizer) {
			logger.Debug("Add-pending finalizer still on object, removing", "finalizer", o.addPendingFinalizer, "currentFinalizers", finalizers)
			// The removal writes the server's state back into the object it's given. object may be the informer cache's
			// own, and changing it would make the next event's old match its new, which Update takes for a resync.
			err = o.finalizerUpdater.RemoveFinalizer(ctx, object.Copy(), o.addPendingFinalizer)
			if err != nil {
				span.SetStatus(codes.Error, fmt.Sprintf("error removing finalizer: %s", err.Error()))
				if chk, ok := errors.AsType[FinalizerError](err); ok {
					logger = logger.With("status", chk.Status().Code, "message", chk.Status().Message, "request", chk.PatchRequest())
				}
				logger.Error("error removing in-progress finalizer", "error", err.Error(), "kind", object.GroupVersionKind().Kind, "namespace", object.GetNamespace(), "name", object.GetName())
				return err
			}
		}
		return nil
	}

	return o.addNew(ctx, span, logger, nil, object)
}

// addNew runs AddFunc with the in-progress finalizer in place if initialization
// is still needed. If Add already completed, it passes any spec change from an
// Update event to UpdateFunc.
func (o *OpinionatedWatcher) addNew(ctx context.Context, span trace.Span, logger logging.Logger, previous, event resource.Object) error {
	finalizers := o.getFinalizers(event)
	// If this isn't a delete or an add we've seen before, then it's a new resource we need to handle appropriately.
	// Call the add handler, and if it returns successfully (no error), add the finalizer.
	// Before we call the downstream add, add an "add pending" finalizer to prevent us missing a delete during the add process (and/or its retries)
	//
	// The finalizer patches write the server's state back into the object they're given. Work on a copy: object may be
	// the informer cache's own, and changing it would make the next event's old match its new, which Update takes for
	// a resync.
	object := event.Copy()
	logger.Debug("Adding in-progress finalizer before add call", "finalizer", o.addPendingFinalizer, "currentFinalizers", finalizers)
	object, state, err := o.startAdd(ctx, object)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("finalizer add error: %s", err.Error()))
		if chk, ok := errors.AsType[FinalizerError](err); ok {
			logger = logger.With("status", chk.Status().Code, "message", chk.Status().Message, "request", chk.PatchRequest())
		}
		logger.Error("error adding in-progress finalizer", "error", err.Error(), "kind", object.GroupVersionKind().Kind, "namespace", object.GetNamespace(), "name", object.GetName())
		return fmt.Errorf("error adding in-progress finalizer: %w", err)
	}
	if state != addNeeded {
		logger.Debug("Add no longer needed: an earlier add finished, or the object is deleting, gone, or replaced", "currentFinalizers", object.GetFinalizers())
		if state == addComplete && previous != nil && addSpecChanged(previous, event) {
			return o.dispatchUpdate(ctx, span, previous, o.callbackObject(event, object))
		}
		return nil
	}

	err = o.addFunc(ctx, o.callbackObject(event, object))
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("watcher add error: %s", err.Error()))
		return err
	}

	// Add the finalizer
	logger.Debug("Successful Add call, adding finalizer and removing in-progress one", "finalizer", o.finalizer, "currentFinalizers", finalizers)
	err = o.finalizerUpdater.ReplaceFinalizer(ctx, object, o.addPendingFinalizer, o.finalizer)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("finalizer add error: %s", err.Error()))
		if chk, ok := errors.AsType[FinalizerError](err); ok {
			logger = logger.With("status", chk.Status().Code, "message", chk.Status().Message, "request", chk.PatchRequest())
		}
		logger.Error("error adding finalizer", "error", err.Error(), "kind", object.GroupVersionKind().Kind, "namespace", object.GetNamespace(), "name", object.GetName())
		return fmt.Errorf("error adding finalizer: %w", err)
	}
	return nil
}

type addState int

const (
	addSkipped addState = iota
	addNeeded
	addComplete
)

// startAdd checks whether Add is still needed. A result of addNeeded means the
// in-progress finalizer is in place and the caller can run AddFunc.
func (o *OpinionatedWatcher) startAdd(ctx context.Context, object resource.Object) (resource.Object, addState, error) {
	id := object.GetStaticMetadata().Identifier()
	uid := object.GetUID()
	check := func(obj resource.Object) addState {
		if obj.GetUID() != uid || obj.GetDeletionTimestamp() != nil {
			return addSkipped
		}
		// A shared name can be the marker we just installed for this Add.
		if o.finalizer != o.addPendingFinalizer && slices.Contains(obj.GetFinalizers(), o.finalizer) {
			return addComplete
		}
		return addNeeded
	}
	if slices.Contains(object.GetFinalizers(), o.addPendingFinalizer) {
		// An earlier attempt started. Adding the in-progress finalizer again would be a no-op which reads nothing, so
		// read the current state here.
		current, err := o.readCurrent(ctx, id)
		if apierrors.IsNotFound(err) {
			return object, addSkipped, nil
		}
		if err != nil {
			return object, addSkipped, fmt.Errorf("unable to get object with in-progress finalizer: %w", err)
		}
		object = current
		if state := check(object); state != addNeeded {
			return object, state, nil
		}
	}
	object, patched, err := o.finalizerUpdater.addFinalizerUnless(ctx, object, o.addPendingFinalizer, o.readCurrent, func(obj resource.Object) bool {
		return check(obj) != addNeeded
	})
	if apierrors.IsNotFound(err) {
		return object, addSkipped, nil
	}
	if err != nil {
		return object, addSkipped, err
	}
	if !patched {
		return object, check(object), nil
	}
	if _, ok := o.finalizerUpdater.client.(fullObjectReader); !ok {
		return object, addNeeded, nil
	}

	// PatchInto can advance the version without copying admission changes to
	// the payload. Read both together before handing them to AddFunc.
	current, err := o.readCurrent(ctx, id)
	if apierrors.IsNotFound(err) {
		return object, addSkipped, nil
	}
	if err != nil {
		return object, addSkipped, fmt.Errorf("unable to get object after adding in-progress finalizer: %w", err)
	}
	state := check(current)
	if state == addNeeded && !slices.Contains(current.GetFinalizers(), o.addPendingFinalizer) {
		return current, addSkipped, fmt.Errorf("object is missing in-progress finalizer %q after patch", o.addPendingFinalizer)
	}
	return current, state, nil
}

// readCurrent returns private state for finalizer decisions and patches.
func (o *OpinionatedWatcher) readCurrent(ctx context.Context, id resource.Identifier) (resource.Object, error) {
	client := o.finalizerUpdater.client
	if reader, ok := client.(fullObjectReader); ok {
		current, err := reader.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return current.Copy(), nil
	}
	current := o.schema.ZeroValue()
	current.SetName(id.Name)
	current.SetNamespace(id.Namespace)
	err := client.GetInto(ctx, id, current)
	return current, err
}

// callbackObject copies the full current snapshot when available. For
// metadata-only clients it copies the event so the payload and resourceVersion
// come from the same snapshot.
func (o *OpinionatedWatcher) callbackObject(event, current resource.Object) resource.Object {
	if _, ok := o.finalizerUpdater.client.(fullObjectReader); ok {
		return current.Copy()
	}
	return event.Copy()
}

// Update filters unchanged positive generations, handles deletion and recovers missing finalizers.
// An admitted spec change is delivered even if Add completed after the notification was queued.
//
//nolint:funlen
func (o *OpinionatedWatcher) Update(ctx context.Context, src resource.Object, tgt resource.Object) error {
	ctx, span := GetTracer().Start(ctx, "OpinionatedWatcher-update")
	defer span.End()
	// TODO: If old is nil, it _might_ be ok?
	if src == nil {
		return errors.New("old cannot be nil")
	}
	if tgt == nil {
		return errors.New("new cannot be nil")
	}

	logger := logging.FromContext(ctx).With("action", "update", "component", "OpinionatedWatcher", "kind", tgt.GroupVersionKind().Kind, "namespace", tgt.GetNamespace(), "name", tgt.GetName())
	logger.Debug("Handling update")

	// Only fire off Update if the generation has changed (so skip subresource updates)
	if tgt.GetGeneration() > 0 && src.GetGeneration() == tgt.GetGeneration() {
		return nil
	}

	oldFinalizers := o.getFinalizers(src)
	newFinalizers := o.getFinalizers(tgt)

	// Check if the deletion timestamp is non-nil.
	// This denotes that the resource was deletes, but has one or more finalizers blocking it from actually deleting.
	if tgt.GetDeletionTimestamp() != nil {
		return o.cleanup(ctx, tgt)
	}

	if !slices.Contains(newFinalizers, o.finalizer) {
		// Either the add event was missed, or the original AddFunc call failed. Use the
		// complete Add path so the in-progress finalizer protects the side effect.
		logger.Debug("Missing finalizer, calling Add")
		return o.addNew(ctx, span, logger, src, tgt)
	} else if !slices.Contains(oldFinalizers, o.finalizer) && !addSpecChanged(src, tgt) {
		logger.Debug("Finalizer-only add update, ignoring")
		return nil
	}

	return o.dispatchUpdate(ctx, span, src, tgt)
}

func (o *OpinionatedWatcher) dispatchUpdate(ctx context.Context, span trace.Span, src, tgt resource.Object) error {
	err := o.updateFunc(ctx, src, tgt)
	if err != nil {
		span.SetStatus(codes.Error, fmt.Sprintf("watcher update error: %s", err.Error()))
		return err
	}
	return nil
}

// addSpecChanged distinguishes spec changes from our own finalizer notifications.
// A full-object event can carry a spec change even if storage omits generation.
func addSpecChanged(src, tgt resource.Object) bool {
	if src.GetGeneration() != tgt.GetGeneration() {
		return true
	}
	if tgt.GetGeneration() != 0 {
		return false
	}
	oldSpec, newSpec := src.GetSpec(), tgt.GetSpec()
	if oldSpec != nil || newSpec != nil {
		return !reflect.DeepEqual(oldSpec, newSpec)
	}

	// Wrapped native resources such as ConfigMaps carry their desired state
	// outside spec. Compare their wire payload only at Add completion.
	oldPayload, ok := addPayload(src)
	if !ok {
		return true
	}
	newPayload, ok := addPayload(tgt)
	return !ok || !reflect.DeepEqual(oldPayload, newPayload)
}

func addPayload(obj resource.Object) (map[string]any, bool) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return nil, false
	}
	for _, field := range []string{"apiVersion", "kind", "metadata", "status"} {
		delete(payload, field)
	}
	for field := range obj.GetSubresources() {
		delete(payload, field)
	}
	return payload, true
}

func (o *OpinionatedWatcher) needsCleanup(obj resource.Object) bool {
	return obj.GetDeletionTimestamp() != nil && o.ownsFinalizer(o.getFinalizers(obj))
}

func (o *OpinionatedWatcher) ownsFinalizer(finalizers []string) bool {
	return slices.Contains(finalizers, o.finalizer) || slices.Contains(finalizers, o.addPendingFinalizer)
}

// cleanup checks that the same UID is still deleting with an owned finalizer
// before calling DeleteFunc. It removes the watcher's finalizers after success.
func (o *OpinionatedWatcher) cleanup(ctx context.Context, obj resource.Object) error {
	if !o.needsCleanup(obj) {
		return nil
	}
	id := obj.GetStaticMetadata().Identifier()
	// A replay can carry our finalizer even after a previous cleanup removed it.
	current, err := o.readCurrent(ctx, id)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	uid := obj.GetUID()
	if current.GetUID() != uid || !o.needsCleanup(current) {
		return nil
	}
	if err := o.deleteFunc(ctx, o.callbackObject(obj, current)); err != nil {
		trace.SpanFromContext(ctx).SetStatus(codes.Error, fmt.Sprintf("watcher delete error: %s", err))
		return err
	}
	obj = current
	for _, f := range []string{o.finalizer, o.addPendingFinalizer} {
		if obj.GetUID() != uid || obj.GetDeletionTimestamp() == nil {
			break
		}
		if slices.Contains(obj.GetFinalizers(), f) {
			if err := o.finalizerUpdater.RemoveFinalizer(ctx, obj, f); err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				trace.SpanFromContext(ctx).SetStatus(codes.Error, fmt.Sprintf("error removing finalizer: %s", err))
				logging.FromContext(ctx).Error("error removing finalizer", "error", err, "finalizer", f, "namespace", id.Namespace, "name", id.Name)
				return err
			}
		}
	}
	return nil
}

// Delete exists to implement ResourceWatcher,
// but, due to deletes only happening after the finalizer is removed, this function does nothing.
func (*OpinionatedWatcher) Delete(context.Context, resource.Object) error {
	// Do nothing here, because we add finalizers, so we actually call delete code on updates/add-sync
	return nil
}

// RegisterMetricsCollectors registers additional prometheus collectors for the OpinionatedWatcher,
// in addition to those provided by any wrapped ResourceWatcher via Wrap().
// These additional prometheus collectors are exposed as a part of the list returned by PrometheusCollectors().
func (o *OpinionatedWatcher) RegisterMetricsCollectors(collectors ...prometheus.Collector) {
	o.collectors = append(o.collectors, collectors...)
}

func (o *OpinionatedWatcher) PrometheusCollectors() []prometheus.Collector {
	return o.collectors
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
func (o *OpinionatedWatcher) updateFunc(ctx context.Context, src, tgt resource.Object) error {
	if o.UpdateFunc != nil {
		return o.UpdateFunc(ctx, src, tgt)
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

func (*OpinionatedWatcher) getFinalizers(object resource.Object) []string {
	if object.GetFinalizers() != nil {
		return object.GetFinalizers()
	}
	return make([]string, 0)
}
