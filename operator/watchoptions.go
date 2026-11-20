package operator

import (
	"fmt"
	"time"
)

// WatchOptions is what a kind's handlers need of the changes they are watched by, and what a watch mechanism
// provides. Both sides use one type so that simple.App can compare them.
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
	// PageSize is how large a page the resync should list at a time, resolved the same way. Also a request, and the
	// least likely to be honored: a mechanism which re-delivers a cache rather than listing ignores it.
	PageSize int64
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

// WatchDetail is how much a delivery carries: what a mechanism's deliveries carry, or the least a kind's handlers
// need. It is about the delivery, not about whether the informer behind it holds a cache.
//
// Experimental: this API may change in a minor release.
type WatchDetail string

const (
	// WatchDetailFullObject is a delivery carrying the whole object, which is what every informer the SDK ships
	// carries and what a kind needs by needing nothing.
	WatchDetailFullObject WatchDetail = "full-object"
	// WatchDetailIdentity is a delivery carrying only namespace, name and version, for handlers which re-read the
	// object they act on.
	WatchDetailIdentity WatchDetail = "identity"
)

// SatisfiedBy reports whether deliveries carrying provided can serve handlers needing d.
func (d WatchDetail) SatisfiedBy(provided WatchDetail) bool {
	// An unrecognized value reads as the full object, so an unknown need is needed in full and an unknown
	// provision is not assumed to be short.
	return provided != WatchDetailIdentity || d == WatchDetailIdentity
}

// WatchDistribution is how one channel's work is divided across an app's instances — which live deliveries each
// receives, or which slice of a resync each lists. Which instance is *active* is a separate matter: leader election
// decides that, and composes with any distribution.
//
// Experimental: this API may change in a minor release.
type WatchDistribution string

const (
	// WatchDistributionAll gives every instance the whole channel, which a watch does and a kind needs by needing
	// nothing. Handlers need it when they keep state of their own derived from the kind.
	WatchDistributionAll WatchDistribution = "all"
	// WatchDistributionShared divides the channel over the instances with no affinity, for handlers whose work is
	// idempotent and done once.
	WatchDistributionShared WatchDistribution = "shared"
	// WatchDistributionSharded divides it by object, so each object keeps going to the same instance, for handlers
	// which keep per-object state or must not run twice at once on one object.
	WatchDistributionSharded WatchDistribution = "sharded"
)

// SatisfiedBy reports whether a channel divided as provided can serve handlers needing d. The values are not a
// scale: affinity is the only strengthening, and giving every instance everything serves nothing but itself, since
// it makes every instance act.
func (d WatchDistribution) SatisfiedBy(provided WatchDistribution) bool {
	if d == provided {
		return true
	}
	return d == WatchDistributionShared && provided == WatchDistributionSharded
}

// WatchOptionsInformer is an Informer which says what it provides. Implement it on any informer which carries less
// than the whole object, does not deliver each change as it happens, or divides a channel, so that pairing it with a
// kind it cannot serve fails rather than silently losing deliveries. One which does not implement it is not taken to
// provide less — nothing can be read off it.
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
