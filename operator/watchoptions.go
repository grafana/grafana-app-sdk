package operator

import (
	"fmt"
	"time"
)

// WatchOptions is what a kind's handlers need of the changes they are watched by, and what a watch mechanism
// provides. simple.App compares the two, which is why both sides use one type: an axis added here is added to the
// need, to what a mechanism provides, and to the comparison at once.
//
// Experimental: this API may change in a minor release.
type WatchOptions struct {
	// Live is what the handlers need of deliveries made as changes happen, or nil when they need none. A
	// WatchOptions with neither channel set needs the live channel with its defaults, which is what every informer
	// the SDK ships delivers.
	Live *WatchLiveOptions
	// Sweep is what the handlers need of the periodic re-delivery of every object, or nil when they need none.
	Sweep *WatchSweepOptions
}

// WatchLiveOptions is what a kind's handlers need of deliveries made as changes happen, and what a mechanism's live
// channel provides.
//
// Experimental: this API may change in a minor release.
type WatchLiveOptions struct {
	// Detail is the least a live delivery must carry. Its zero value needs WatchDetailFullObject.
	Detail WatchDetail
	// Distribution is how live deliveries must be divided across the app's instances. Its zero value needs
	// WatchDistributionAll.
	Distribution WatchDistribution
}

// WatchSweepOptions is what a kind's handlers need of the periodic re-delivery of every object, and what a
// mechanism's sweep provides. It is separate from WatchLiveOptions because a mechanism's two channels differ: a bus
// may carry identities live and read whole objects on its re-list, and a round-robin group divides the live channel
// while that re-list still lists the whole kind.
//
// Experimental: this API may change in a minor release.
type WatchSweepOptions struct {
	// Detail is the least a swept delivery must carry. Its zero value needs WatchDetailFullObject.
	Detail WatchDetail
	// Distribution is how the sweep must be divided across the app's instances. Its zero value needs
	// WatchDistributionAll, which is what a re-list of the whole kind does.
	Distribution WatchDistribution
	// Every is how often the handlers need the sweep. Like PageSize, and unlike the fields above, it is not
	// compared: a mechanism takes its cadence rather than offering one, so simple.App passes this to the mechanism
	// it selects — as the cache resync interval for a watch, or the re-list interval for a re-list — which is how
	// two kinds served by the same mechanism sweep at different rates. Its zero value takes the mechanism's own.
	Every time.Duration
	// PageSize is how large a page the sweep should read at a time, for a kind whose objects are many or large
	// enough that one list of them is not a reasonable request. It is not compared either, for the same reason, and
	// because it changes how a sweep is fetched rather than what the handlers are given: a mechanism which cannot
	// paginate ignores it. Its zero value takes the mechanism's own.
	PageSize int64
}

// SatisfiedBy reports whether a mechanism providing provided can serve handlers needing o: it has every channel they
// need, and on each of those carries at least the detail they read and divides it no further than they allow. A
// channel a kind does not need is not compared, so a mechanism which carries identities live still serves a kind
// which only sweeps.
func (o WatchOptions) SatisfiedBy(provided WatchOptions) bool {
	needLive, needSweep := o.channels()
	haveLive, haveSweep := provided.channels()
	if needLive != nil {
		if haveLive == nil ||
			!needLive.detail().SatisfiedBy(haveLive.detail()) ||
			!needLive.distribution().SatisfiedBy(haveLive.distribution()) {
			return false
		}
	}
	if needSweep != nil {
		if haveSweep == nil ||
			!needSweep.detail().SatisfiedBy(haveSweep.detail()) ||
			!needSweep.distribution().SatisfiedBy(haveSweep.distribution()) {
			return false
		}
	}
	return true
}

// String describes each channel as detail/distribution, and "none" for a channel which is not delivered on. It
// names only what is compared, so the cadence and page size a mechanism is handed are left out.
func (o WatchOptions) String() string {
	live, sweep := o.channels()
	described := "live=none"
	if live != nil {
		described = fmt.Sprintf("live=%s/%s", live.detail(), live.distribution())
	}
	if sweep == nil {
		return described + " sweep=none"
	}
	return fmt.Sprintf("%s sweep=%s/%s", described, sweep.detail(), sweep.distribution())
}

// channels returns the channels o describes, defaulting a WatchOptions which describes neither to the live channel:
// a kind which says nothing needs changes as they happen, and an informer which says nothing delivers them.
func (o WatchOptions) channels() (*WatchLiveOptions, *WatchSweepOptions) {
	if o.Live == nil && o.Sweep == nil {
		return &WatchLiveOptions{}, nil
	}
	return o.Live, o.Sweep
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

func (s WatchSweepOptions) detail() WatchDetail {
	if s.Detail == "" {
		return WatchDetailFullObject
	}
	return s.Detail
}

func (s WatchSweepOptions) distribution() WatchDistribution {
	if s.Distribution == "" {
		return WatchDistributionAll
	}
	return s.Distribution
}

// WatchDetail is how much a delivery carries. It is read two ways: as what a watch mechanism's deliveries carry,
// and as the least a kind's handlers need.
//
// It is about the delivery, not about the informer behind it: whether that informer holds a cache of the kind is a
// property of the informer, which no handler reads through this.
//
// Experimental: this API may change in a minor release.
type WatchDetail string

const (
	// WatchDetailFullObject is a delivery carrying the whole object. Handlers served by it read that object off the
	// event, and read anything else through the API. It is what every informer the SDK ships carries, so it is also
	// what a kind needs by needing nothing.
	WatchDetailFullObject WatchDetail = "full-object"
	// WatchDetailIdentity is a delivery carrying only which object it is about — namespace, name and version.
	// Handlers served by it re-read the object they act on, so a mechanism can carry it without holding a cache
	// and without a read of its own per delivery.
	WatchDetailIdentity WatchDetail = "identity"
)

// SatisfiedBy reports whether deliveries carrying provided can serve handlers needing d.
func (d WatchDetail) SatisfiedBy(provided WatchDetail) bool {
	// Anything this build of the SDK does not recognize reads as the full object, so that an unknown need is needed
	// in full and an unknown provision is not assumed to be short.
	return provided != WatchDetailIdentity || d == WatchDetailIdentity
}

// WatchDistribution is how one channel's work is divided across an app's instances: which live deliveries each
// instance receives, or — once a mechanism supports it — which slice of a sweep each instance lists. It is read two
// ways: as what a mechanism's channel does, and as what a kind's handlers need of it.
//
// The values do not form a scale. Giving every instance everything is not "more" than dividing it: handlers which
// must act once per change are broken by every instance acting, and handlers which keep state of their own are
// broken by an instance not seeing part of the kind. Only the affinity between WatchDistributionShared and
// WatchDistributionSharded is a strengthening, and only there does one satisfy the other.
//
// Which instance is *active* is a separate matter: leader election and leases decide that, and compose with any
// distribution.
//
// Experimental: this API may change in a minor release.
type WatchDistribution string

const (
	// WatchDistributionAll gives every instance the whole channel, which is what a watch does and what a kind needs
	// by needing nothing. Handlers need it when they keep state of their own derived from the kind — an index, a
	// counter, a warmed cache — where an instance which does not see part of the kind holds something wrong rather
	// than something unfinished.
	WatchDistributionAll WatchDistribution = "all"
	// WatchDistributionShared divides the channel over the instances, each object's work going to one of them, with
	// no affinity between an object and the instance handling it. Handlers need no more than this when their work
	// is idempotent and done once, which is most reconciliation, and it is cheaper because each instance carries a
	// share rather than all of it.
	WatchDistributionShared WatchDistribution = "shared"
	// WatchDistributionSharded divides the channel over the instances by object, so each object's work goes to one
	// instance and keeps going to the same one. Handlers need it when they keep per-object state between
	// deliveries, or when two instances must not reconcile one object at once.
	WatchDistributionSharded WatchDistribution = "sharded"
)

// SatisfiedBy reports whether a channel divided as provided can serve handlers needing d. Affinity is the only
// strengthening: sharded work serves a kind which needs only one instance to act, while giving every instance
// everything serves nothing but itself, since it makes every instance act.
func (d WatchDistribution) SatisfiedBy(provided WatchDistribution) bool {
	if d == provided {
		return true
	}
	return d == WatchDistributionShared && provided == WatchDistributionSharded
}

// WatchOptionsInformer is an Informer which says what it provides. An informer which does not implement it is not
// taken to provide less: nothing can be read off it, so simple.App cannot check it against a kind's needs and does
// not try. Implement it on any informer which carries less than the whole object, which does not deliver each change
// as it happens, or which divides a channel across instances, so that pairing it with a kind it cannot serve fails
// rather than silently losing deliveries.
//
// Experimental: this API may change in a minor release.
type WatchOptionsInformer interface {
	Informer
	// WatchOptions is what this informer gives a kind's handlers.
	WatchOptions() WatchOptions
}

// InformerWatchOptions reports what inf says it provides, and whether it says anything at all. An informer which
// wraps another — ConcurrentInformer does — is unwrapped, since wrapping changes nothing about what a delivery
// carries; an informer which neither states options nor wraps one says nothing, and is not taken to provide less.
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
