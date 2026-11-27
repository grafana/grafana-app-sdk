package operator

import (
	"fmt"
	"strings"

	"github.com/grafana/grafana-app-sdk/resource"
)

// InformerRequest holds what BuildInformer needs to build the informer for one kind. Supplier decides which
// informer to build.
//
// Experimental: this API may change in a minor release.
type InformerRequest struct {
	Kind     resource.Kind
	Clients  resource.ClientGenerator
	Supplier InformerSupplier
	// Options configures the informer. Options.WatchNeeds is the kind's declaration. BuildInformer fills in its
	// defaults on a copy.
	Options InformerOptions
	// Opinionated is true when the kind's handler is an opinionated reconciler or watcher. These read the deletion
	// timestamp, the finalizers, and the generation from each event, so they need at least metadata.
	Opinionated bool
	// DiscardsResyncs is true when the kind's handler drops resync deliveries, as an OpinionatedWatcher does (it skips
	// updates with an unchanged generation). BuildInformer rejects a Resync declaration for such a handler.
	DiscardsResyncs bool
}

// BuildInformer builds the kind's informer with req.Supplier, passing the kind's needs in InformerOptions.WatchNeeds.
// It returns an error if the declaration is invalid for the handler, or if the informer reports that it provides
// less than the kind needs.
//
// Experimental: this API may change in a minor release.
func BuildInformer(req InformerRequest) (Informer, error) {
	req.Options.WatchNeeds = req.Options.WatchNeeds.WithDefaults()
	if err := req.validate(); err != nil {
		return nil, err
	}

	inf, err := req.Supplier(req.Kind, req.Clients, req.Options)
	if err != nil {
		return nil, err
	}
	if err := req.checkProvision(inf); err != nil {
		return nil, err
	}
	return inf, nil
}

// validate rejects declarations that the kind's handler cannot use: a resync for a handler that drops resyncs, and
// less than metadata for an opinionated handler. Because RefusedDeliveries holds every channel to the declared
// detail, this also keeps every delivery to an opinionated handler at metadata or more.
func (r InformerRequest) validate() error {
	if r.DiscardsResyncs && r.Options.WatchNeeds.Resync != nil {
		return fmt.Errorf("kind '%s' declares a resync, which its opinionated watcher would never act on: it skips"+
			" an update whose generation is unchanged, which is how a resync arrives. Use a reconciler, which the"+
			" opinionated variant passes resyncs to, or a plain watcher", r.Kind.GroupVersionKind().String())
	}
	if !r.Opinionated {
		return nil
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

// checkProvision returns an error when the informer reports that it provides less than the kind needs. An informer
// that does not report what it provides is not checked.
func (r InformerRequest) checkProvision(inf Informer) error {
	needs := r.Options.WatchNeeds
	provided, stated := InformerWatchOptions(inf)
	if !stated {
		return nil
	}
	// Every channel reaches an opinionated handler, including channels the kind did not ask for. Identities on any
	// channel break its deletion handling.
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
	// Suggest a fix when the kind needs a resync but no interval is set anywhere.
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
