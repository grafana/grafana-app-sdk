package operator

import (
	"fmt"
	"time"
)

// WatchOptions is what a kind's handlers need of the changes they are watched by, and what a watch mechanism
// provides. Both sides use one type so the two can be compared.
//
// Experimental: this API may change in a minor release.
type WatchOptions struct {
	// Live is what the handlers need as changes happen, or nil for none. A WatchOptions setting neither channel
	// needs the live channel with its defaults.
	Live *WatchLiveOptions
	// Resync is what they need of the periodic re-delivery of every object, or nil for none.
	Resync *WatchResyncOptions
}

// WatchLiveOptions is one mechanism's live channel, or what a kind's handlers need of it.
//
// Experimental: this API may change in a minor release.
type WatchLiveOptions struct {
	// Detail is the least a live delivery must carry. Zero needs WatchDetailFullObject.
	Detail WatchDetail
	// Distribution is how live deliveries may be divided across instances. Zero needs WatchDistributionAll.
	Distribution WatchDistribution
}

// WatchResyncOptions is one mechanism's periodic re-delivery of every object, or what a kind's handlers need of it.
// It is separate from WatchLiveOptions because a mechanism's two channels differ: a bus may carry identities live
// and whole objects on its re-list, and a round-robin group divides only the live one.
//
// Experimental: this API may change in a minor release.
type WatchResyncOptions struct {
	// Detail is the least a re-delivered object must carry. Zero needs WatchDetailFullObject.
	Detail WatchDetail
	// Distribution is how the resync may be divided across instances. Zero needs WatchDistributionAll.
	Distribution WatchDistribution
	// Interval is how often the handlers need the resync, resolved against the mechanism's own by
	// InformerOptions.ResyncOptions. A request rather than a requirement: it takes no part in SatisfiedBy.
	Interval time.Duration
	// ListPageSize is how large a page any listing should read at a time, resolved the same way. It is named for
	// listing because that is what it governs: a mechanism which resyncs by listing uses it there, a watch for the
	// initial list it does at startup. The least likely field to be honored, since a mechanism which lists nothing
	// ignores it.
	ListPageSize int64
}

// SatisfiedBy reports whether a mechanism providing provided can serve handlers needing o. A channel the need leaves
// nil is not compared.
func (o WatchOptions) SatisfiedBy(provided WatchOptions) bool {
	needLive, needResync := o.channels()
	haveLive, haveResync := provided.channels()
	if needLive != nil {
		if haveLive == nil ||
			!needLive.detail().SatisfiedBy(haveLive.detail()) ||
			!needLive.distribution().SatisfiedBy(haveLive.distribution()) {
			return false
		}
	}
	if needResync != nil {
		if haveResync == nil ||
			!needResync.detail().SatisfiedBy(haveResync.detail()) ||
			!needResync.distribution().SatisfiedBy(haveResync.distribution()) {
			return false
		}
	}
	return true
}

// WithDefaults returns o with every unset field filled in, which is what SatisfiedBy compares and what a mechanism
// is handed: the one place the zero values are defined. The result is a copy, so a declaration is never defaulted in
// place.
func (o WatchOptions) WithDefaults() WatchOptions {
	live, resync := o.channels()
	defaulted := WatchOptions{}
	if live != nil {
		defaulted.Live = &WatchLiveOptions{Detail: live.detail(), Distribution: live.distribution()}
	}
	if resync != nil {
		defaulted.Resync = &WatchResyncOptions{
			Detail:       resync.detail(),
			Distribution: resync.distribution(),
			Interval:     resync.Interval,
			ListPageSize: resync.ListPageSize,
		}
	}
	return defaulted
}

// String names each channel as detail/distribution, and only what is compared.
func (o WatchOptions) String() string {
	live, resync := o.channels()
	described := "live=none"
	if live != nil {
		described = fmt.Sprintf("live=%s/%s", live.detail(), live.distribution())
	}
	if resync == nil {
		return described + " resync=none"
	}
	return fmt.Sprintf("%s resync=%s/%s", described, resync.detail(), resync.distribution())
}

// channels defaults a WatchOptions describing neither to the live channel: a kind which says nothing needs changes
// as they happen, and an informer which says nothing delivers them.
func (o WatchOptions) channels() (*WatchLiveOptions, *WatchResyncOptions) {
	if o.Live == nil && o.Resync == nil {
		return &WatchLiveOptions{}, nil
	}
	return o.Live, o.Resync
}

func (l WatchLiveOptions) detail() WatchDetail {
	if l.Detail == "" {
		return WatchDetailFullObject
	}
	return l.Detail
}

func (l WatchLiveOptions) distribution() WatchDistribution {
	if l.Distribution == "" {
		return WatchDistributionAll
	}
	return l.Distribution
}

func (r WatchResyncOptions) detail() WatchDetail {
	if r.Detail == "" {
		return WatchDetailFullObject
	}
	return r.Detail
}

func (r WatchResyncOptions) distribution() WatchDistribution {
	if r.Distribution == "" {
		return WatchDistributionAll
	}
	return r.Distribution
}

// WatchDetail is how much of an object a delivery carries: what a mechanism's deliveries carry, or the least a
// kind's handlers need. It is about the delivery, not about whether the informer behind it holds a cache.
//
// Experimental: this API may change in a minor release.
type WatchDetail string

const (
	// WatchDetailFullObject is a delivery carrying the whole object, which is what every informer the SDK ships
	// carries and what a kind needs by needing nothing.
	WatchDetailFullObject WatchDetail = "full-object"
	// WatchDetailMetadata is a delivery carrying the object's metadata but not its spec or status — the projection
	// the API serves as PartialObjectMetadata. It is what the Opinionated variants need, since the deletion
	// timestamp, the finalizer list and the generation are all metadata.
	WatchDetailMetadata WatchDetail = "metadata"
	// WatchDetailIdentity is a delivery carrying only which object it is about — namespace, name and version — for
	// handlers which re-read whatever they act on. It cannot serve the Opinionated variants.
	WatchDetailIdentity WatchDetail = "identity"
)

// SatisfiedBy reports whether deliveries carrying provided can serve handlers needing d. A value this build does not
// recognize satisfies only a need for exactly it: the type is an open string a third-party mechanism answers with, so
// an unknown detail level must not pass for whole objects, while two sides which name the same one agree.
func (d WatchDetail) SatisfiedBy(provided WatchDetail) bool {
	switch provided {
	case WatchDetailFullObject, "":
		return true
	case WatchDetailMetadata:
		return d == WatchDetailMetadata || d == WatchDetailIdentity
	case WatchDetailIdentity:
		return d == WatchDetailIdentity
	default:
		return d == provided
	}
}

// WatchDistribution is how one channel's work is divided across an app's instances — which live deliveries each
// receives, or which slice of a resync each lists. Which instance is *active* is a separate matter: leader election
// decides that, and composes with any distribution.
//
// Experimental: this API may change in a minor release.
type WatchDistribution string

const (
	// WatchDistributionAll gives every instance the whole channel, which a watch does and a kind needs by needing
	// nothing. Handlers need it when they keep state of their own derived from the kind, where an instance which
	// does not see part of it holds something wrong rather than something unfinished.
	WatchDistributionAll WatchDistribution = "all"
	// WatchDistributionShared divides the channel over the instances with no affinity. As a kind's declaration it
	// permits division rather than requiring it, so every distribution satisfies it: a kind declaring it where
	// nothing divides anything is simply watched.
	WatchDistributionShared WatchDistribution = "shared"
	// WatchDistributionSharded divides it by object, so each object keeps going to the same instance. Unlike
	// WatchDistributionShared it is a requirement, for handlers which keep per-object state between deliveries.
	WatchDistributionSharded WatchDistribution = "sharded"
)

// SatisfiedBy reports whether a channel divided as provided can serve handlers needing d. The values are not a
// scale, and two of them read in opposite directions: WatchDistributionShared permits division rather than requiring
// it, so every instance getting everything serves it too, while WatchDistributionSharded requires the affinity only
// sharding gives and WatchDistributionAll requires that nothing is divided at all.
func (d WatchDistribution) SatisfiedBy(provided WatchDistribution) bool {
	// Unset means WatchDistributionAll on either side, so this comparator agrees with WatchOptions.SatisfiedBy,
	// which defaults both before comparing.
	needed := d
	if needed == "" {
		needed = WatchDistributionAll
	}
	if provided == "" {
		provided = WatchDistributionAll
	}
	switch needed {
	case WatchDistributionShared:
		return true
	case WatchDistributionSharded:
		return provided == WatchDistributionSharded
	default: // WatchDistributionAll and anything unrecognized
		return provided == needed
	}
}

// WatchOptionsInformer is an Informer which says what it provides. Implement it on any informer which carries less
// than the whole object, does not deliver each change as it happens, or divides a channel, so that pairing it with a
// kind it cannot serve fails rather than silently losing deliveries.
//
// Experimental: this API may change in a minor release.
type WatchOptionsInformer interface {
	Informer
	// WatchOptions is what this informer gives a kind's handlers.
	WatchOptions() WatchOptions
}

// InformerWatchOptions reports what inf says it provides, and whether it says anything at all. An informer wrapping
// another is unwrapped, since wrapping changes nothing about what a delivery carries.
func InformerWatchOptions(inf Informer) (WatchOptions, bool) {
	for inf != nil {
		if described, ok := inf.(WatchOptionsInformer); ok {
			return described.WatchOptions(), true
		}
		wrapper, ok := inf.(interface{ Unwrap() Informer })
		if !ok {
			break
		}
		inf = wrapper.Unwrap()
	}
	return WatchOptions{}, false
}
