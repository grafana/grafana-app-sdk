package operator

import (
	"fmt"
	"slices"
	"strings"

	"github.com/grafana/grafana-app-sdk/resource"
)

// InformerRequest is what building one kind's informer takes. It holds no policy of its own: Supplier is what
// decides, and the caller resolves it.
//
// Experimental: this API may change in a minor release.
type InformerRequest struct {
	Kind     resource.Kind
	Clients  resource.ClientGenerator
	Supplier InformerSupplier
	// Options is the informer's configuration, with Options.WatchNeeds the kind's declaration as given. It is
	// defaulted in a copy, so a declaration is never defaulted in place.
	Options InformerOptions
	// Opinionated states that the kind's handler is wrapped in its Opinionated variant, which reads the deletion
	// timestamp, the finalizer list and the generation off the object on the event.
	Opinionated bool
	// DiscardsResyncs states that the kind's handler drops a resync delivery, as an OpinionatedWatcher does: it skips
	// an update whose generation is unchanged, which is how a resync arrives. Declaring a resync for such a handler
	// is refused rather than accepted and never delivered.
	DiscardsResyncs bool
	// Finalizer is the finalizer an Opinionated handler adds once an object is handled. Resync checks never leave out
	// an object without it. Required when an Opinionated kind sets Ignore or IgnoreUnchanged.
	Finalizer string
}

// BuildInformer asks the request's supplier for the kind's informer, handing it the kind's needs as
// InformerOptions.WatchNeeds, and reports an error when an informer which says what it provides provides less than
// those needs.
//
// Experimental: this API may change in a minor release.
func BuildInformer(req InformerRequest) (Informer, error) {
	req.Options.WatchNeeds = req.Options.WatchNeeds.WithDefaults()
	if err := req.validate(); err != nil {
		return nil, err
	}
	req.keepUnfinishedObjects()

	inf, err := req.Supplier(req.Kind, req.Clients, req.Options)
	if err != nil {
		return nil, err
	}
	if err := req.checkProvision(inf); err != nil {
		return nil, err
	}
	return inf, nil
}

// validate rejects a declaration no mechanism which satisfies it could serve: the Opinionated variants read the
// deletion timestamp, the finalizer list and the generation off the object, so a kind declaring less than metadata
// alongside one is a contradiction rather than something to quietly upgrade. Since every channel is held to what the
// kind declared (see WatchOptions.RefusedDeliveries), this also keeps every delivery to such a kind at metadata or
// more, so selection needs no knowledge of the handler.
func (r InformerRequest) validate() error {
	if r.DiscardsResyncs && r.Options.WatchNeeds.Resync != nil {
		return fmt.Errorf("kind '%s' declares a resync, which its opinionated watcher would never act on: it skips"+
			" an update whose generation is unchanged, which is how a resync arrives. Use a reconciler, which the"+
			" opinionated variant passes resyncs to, or a plain watcher", r.Kind.GroupVersionKind().String())
	}
	if !r.Opinionated {
		return nil
	}
	if resync := r.Options.WatchNeeds.Resync; r.Finalizer == "" && resync != nil &&
		(resync.Ignore != nil || resync.IgnoreUnchanged != nil) {
		return fmt.Errorf("kind '%s' sets WatchNeeds.Resync.Ignore or IgnoreUnchanged for its opinionated"+
			" reconciler or watcher, but its InformerRequest names no Finalizer to tell an object the handler has"+
			" finished from one it has not, which either check could leave out of every resync",
			r.Kind.GroupVersionKind().String())
	}
	if channels := channelsBelowMetadata(r.Options.WatchNeeds); len(channels) > 0 {
		return fmt.Errorf("kind '%s' declares %s deliveries carrying less than the object's metadata, which its"+
			" opinionated reconciler or watcher cannot be served by: it reads the deletion timestamp, the finalizer"+
			" list and the generation off the object on the event. Declare %q or more, or use a plain reconciler or"+
			" watcher for this kind and handle deletion yourself",
			r.Kind.GroupVersionKind().String(), strings.Join(channels, " and "), WatchDetailMetadata)
	}
	return nil
}

// keepUnfinishedObjects wraps an Opinionated kind's resync checks so they keep objects without its Finalizer.
func (r *InformerRequest) keepUnfinishedObjects() {
	resync := r.Options.WatchNeeds.Resync
	if !r.Opinionated || resync == nil {
		return
	}
	finished := func(obj resource.Object) bool { return slices.Contains(obj.GetFinalizers(), r.Finalizer) }
	if ignore := resync.Ignore; ignore != nil {
		resync.Ignore = func(current resource.Object) bool { return finished(current) && ignore(current) }
	}
	if unchanged := resync.IgnoreUnchanged; unchanged != nil {
		resync.IgnoreUnchanged = func(previous, current resource.Object) bool {
			return finished(current) && unchanged(previous, current)
		}
	}
}

// checkProvision reports an error when an informer which says what it provides provides less than its kind's
// handlers need. One which says nothing is not checked: nothing can be read off it.
func (r InformerRequest) checkProvision(inf Informer) error {
	needs := r.Options.WatchNeeds
	provided, stated := InformerWatchOptions(inf)
	if stated && dividesComparedResync(needs, inf, provided) {
		return fmt.Errorf("informer for kind '%s' compares its resync against previous deliveries, for the kind's"+
			" IgnoreUnchanged, on a resync divided as %q, which has no affinity: an object listed to another instance"+
			" would be taken for one which is gone. Divide it by object (%q), or not at all",
			r.Kind.GroupVersionKind().String(), provided.Resync.distribution(), WatchDistributionSharded)
	}
	if !stated {
		return nil
	}
	// Every channel the mechanism delivers on reaches an opinionated wrapper, including one this kind did not ask
	// for and so did not have compared, and identities on any of them break its deletion handling.
	if channels := channelsBelowMetadata(provided); r.Opinionated && len(channels) > 0 {
		return fmt.Errorf("informer for kind '%s' carries less than the object's metadata on its %s channel, which"+
			" reaches its opinionated reconciler or watcher whether the kind asked for that channel or not and"+
			" leaves it unable to tell a delete in progress from a fresh object; use a plain reconciler or watcher"+
			" for this kind, or give it a mechanism carrying at least metadata on every channel",
			r.Kind.GroupVersionKind().String(), strings.Join(channels, " and "))
	}
	if needs.SatisfiedBy(provided) {
		return nil
	}
	handler := "its reconciler or watcher"
	if r.Opinionated {
		handler = "its opinionated reconciler or watcher"
	}
	if refused := needs.RefusedDeliveries(provided); len(refused) > 0 {
		return fmt.Errorf("informer for kind '%s' makes %s, which reach %s whether the kind asked for that channel"+
			" or not, and it needs %s", r.Kind.GroupVersionKind().String(), strings.Join(refused, " and "), handler,
			needs)
	}
	remedy := ""
	// The one need nothing anywhere was configured for: a resync with no cadence to run it at.
	resolved := r.Options.ResyncOptions()
	if needs.Resync != nil && provided.Resync == nil && (resolved == nil || resolved.Interval == 0) {
		remedy = "; set WatchNeeds.Resync.Interval for the kind, or InformerOptions.CacheResyncInterval for the" +
			" app, so that a mechanism has a cadence to resync at"
	}
	return fmt.Errorf("informer for kind '%s' provides %s, but %s needs %s%s",
		r.Kind.GroupVersionKind().String(), provided, handler, needs, remedy)
}

// channelsBelowMetadata names the channels of o whose deliveries carry less than the object's metadata.
func channelsBelowMetadata(o WatchOptions) []string {
	var channels []string
	if o.Live != nil && !WatchDetailMetadata.SatisfiedBy(o.Live.Detail) {
		channels = append(channels, "Live")
	}
	if o.Resync != nil && !WatchDetailMetadata.SatisfiedBy(o.Resync.Detail) {
		channels = append(channels, "Resync")
	}
	return channels
}
