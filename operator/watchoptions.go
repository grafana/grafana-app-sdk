package operator

import (
	"fmt"
	"time"
)

// WatchOptions describes the deliveries for one kind. A kind uses it to say what its handlers need. A watch mechanism
// uses it to say what it provides. Use SatisfiedBy to compare the two.
//
// Experimental: this API may change in a minor release.
type WatchOptions struct {
	// Live describes deliveries sent when an object changes. Nil means none. If both Live and Resync are nil, the
	// kind needs Live with default values.
	Live *WatchLiveOptions
	// Resync describes deliveries of every object again, on a schedule. Nil means none.
	Resync *WatchResyncOptions
}

// WatchLiveOptions describes the live channel: deliveries sent when an object changes.
//
// Experimental: this API may change in a minor release.
type WatchLiveOptions struct {
	// Detail is the least each delivery must contain. Empty means WatchDetailFullObject.
	Detail WatchDetail
	// Distribution is how deliveries can be split across instances. Empty means WatchDistributionAll.
	Distribution WatchDistribution
	// Ignore can leave out an add or update, based on the current object. Deletes are always delivered. A mechanism
	// can ignore this field and deliver the change anyway.
	Ignore WatchIgnoreFunc
	// IgnoreUnchanged can leave out an update when the current object is unchanged from the previous object in the
	// update. Adds and deletes are always delivered. A mechanism can ignore this field and deliver every update.
	IgnoreUnchanged WatchUnchangedFunc
}

// WatchResyncOptions describes the resync channel: deliveries of every object again, on a schedule. It is separate
// from WatchLiveOptions because a mechanism can provide different detail and distribution on each channel.
//
// Experimental: this API may change in a minor release.
type WatchResyncOptions struct {
	// Detail is the least each delivery must contain. Empty means WatchDetailFullObject.
	Detail WatchDetail
	// Distribution is how deliveries can be split across instances. Empty means WatchDistributionAll.
	Distribution WatchDistribution
	// Interval is how often the resync should run. Zero uses InformerOptions.CacheResyncInterval (see
	// InformerOptions.ResyncOptions). It is a request: it is not used to pick a mechanism, and a mechanism can ignore
	// it.
	Interval time.Duration
	// ListPageSize is how many objects to read per page when a mechanism lists objects from the API. Zero uses
	// InformerOptions.WatchListPageSize. It is a request: a mechanism that does not list ignores it.
	ListPageSize int64
	// Ignore can leave an object out of a resync, based on the current object. A mechanism can ignore this field and
	// deliver the object anyway.
	Ignore WatchIgnoreFunc
	// IgnoreUnchanged can leave an object out of a resync when it is unchanged since its last delivery. Leave it nil
	// if your handlers use the resync to repair drift. Only a PreviousComparer applies it. Other mechanisms deliver
	// every object.
	IgnoreUnchanged WatchUnchangedFunc
}

// SatisfiedBy reports whether a mechanism that provides provided can serve handlers that need o. A channel that o
// leaves nil is not required. But if provided delivers on that channel, those deliveries must pass
// RefusedDeliveries.
func (o WatchOptions) SatisfiedBy(provided WatchOptions) bool {
	if len(o.RefusedDeliveries(provided)) > 0 {
		return false
	}
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

// RefusedDeliveries describes the deliveries from provided that handlers needing o cannot accept. It checks every
// channel that provided delivers on, including channels that o leaves nil. Handlers cannot tell the two channels
// apart, so a nil channel in o must meet the rules of the channel o declared: at least the same Detail, and
// WatchDistributionSharded if o requires it.
//
// It does not check that provided has the channels o needs. Use SatisfiedBy for that.
func (o WatchOptions) RefusedDeliveries(provided WatchOptions) []string {
	floorLive, floorResync := o.deliveryFloors()
	haveLive, haveResync := provided.channels()
	var refused []string
	if haveLive != nil {
		refused = append(refused, floorLive.refuses("Live", haveLive.detail(), haveLive.distribution())...)
	}
	if haveResync != nil {
		refused = append(refused, floorResync.refuses("Resync", haveResync.detail(), haveResync.distribution())...)
	}
	return refused
}

// deliveryFloor is the least a delivery on one channel must meet to reach a kind's handlers.
type deliveryFloor struct {
	detail  WatchDetail
	sharded bool
}

// deliveryFloors returns the floor for each channel: what o declared for it, or, for a nil channel, what o declared
// for the other channel.
func (o WatchOptions) deliveryFloors() (live, resync deliveryFloor) {
	needLive, needResync := o.channels()
	if needLive != nil {
		live = deliveryFloor{needLive.detail(), needLive.distribution() == WatchDistributionSharded}
	}
	if needResync != nil {
		resync = deliveryFloor{needResync.detail(), needResync.distribution() == WatchDistributionSharded}
	}
	if needLive == nil {
		live = resync
	}
	if needResync == nil {
		resync = live
	}
	return live, resync
}

func (f deliveryFloor) refuses(channel string, detail WatchDetail, distribution WatchDistribution) []string {
	var refused []string
	if !f.detail.SatisfiedBy(detail) {
		refused = append(refused, fmt.Sprintf("%s deliveries carrying less than %q", channel, f.detail))
	}
	if f.sharded && distribution != WatchDistributionSharded {
		refused = append(refused, fmt.Sprintf("%s deliveries which are not %q", channel, WatchDistributionSharded))
	}
	return refused
}

// WithDefaults returns a copy of o with every empty field set to its default. This is what SatisfiedBy compares and
// what a mechanism receives. It does not change o.
func (o WatchOptions) WithDefaults() WatchOptions {
	live, resync := o.channels()
	defaulted := WatchOptions{}
	if live != nil {
		defaulted.Live = &WatchLiveOptions{
			Detail:          live.detail(),
			Distribution:    live.distribution(),
			Ignore:          live.Ignore,
			IgnoreUnchanged: live.IgnoreUnchanged,
		}
	}
	if resync != nil {
		defaulted.Resync = &WatchResyncOptions{
			Detail:          resync.detail(),
			Distribution:    resync.distribution(),
			Interval:        resync.Interval,
			ListPageSize:    resync.ListPageSize,
			Ignore:          resync.Ignore,
			IgnoreUnchanged: resync.IgnoreUnchanged,
		}
	}
	return defaulted
}

// String describes each channel as detail/distribution, and marks Ignore and IgnoreUnchanged when they are set.
func (o WatchOptions) String() string {
	live, resync := o.channels()
	described := "live=none"
	if live != nil {
		described = fmt.Sprintf("live=%s/%s%s", live.detail(), live.distribution(),
			ignoreMarks(live.Ignore, live.IgnoreUnchanged))
	}
	if resync == nil {
		return described + " resync=none"
	}
	return fmt.Sprintf("%s resync=%s/%s%s", described, resync.detail(), resync.distribution(),
		ignoreMarks(resync.Ignore, resync.IgnoreUnchanged))
}

func ignoreMarks(ignore WatchIgnoreFunc, ignoreUnchanged WatchUnchangedFunc) string {
	marks := ""
	if ignore != nil {
		marks += "/ignore"
	}
	if ignoreUnchanged != nil {
		marks += "/ignore-unchanged"
	}
	return marks
}

// channels returns o's channels. If both are nil, it returns a default live channel: a kind that declares nothing
// needs live changes, and an informer that describes nothing delivers them.
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

// WatchDetail is how much of an object a delivery contains. It describes the delivery only, not whether the
// informer keeps a cache.
//
// Experimental: this API may change in a minor release.
type WatchDetail string

const (
	// WatchDetailFullObject is a delivery with the whole object. It is the default.
	WatchDetailFullObject WatchDetail = "full-object"
	// WatchDetailMetadata is a delivery with the object's metadata, but not its spec or status (like
	// PartialObjectMetadata). Opinionated reconcilers and watchers need at least this, because they read the deletion
	// timestamp, the finalizers, and the generation.
	WatchDetailMetadata WatchDetail = "metadata"
	// WatchDetailIdentity is a delivery with only the namespace, name, and resourceVersion. Use it for handlers that
	// read the object again before they act. Opinionated reconcilers and watchers cannot use it.
	WatchDetailIdentity WatchDetail = "identity"
)

// SatisfiedBy reports whether deliveries with detail provided can serve handlers that need d. More detail serves a
// need for less. An unknown value serves only a need for the same value.
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

// WatchDistribution is how a channel's deliveries are split across the app's instances. It does not decide which
// instance is active. Leader election does that, and works with any distribution.
//
// Experimental: this API may change in a minor release.
type WatchDistribution string

const (
	// WatchDistributionAll sends every delivery to every instance. It is the default. Handlers need it when they
	// keep their own state about the kind in memory.
	WatchDistributionAll WatchDistribution = "all"
	// WatchDistributionShared sends each delivery to one instance, with no affinity. As a need, it allows splitting
	// but does not require it, so every distribution satisfies it.
	WatchDistributionShared WatchDistribution = "shared"
	// WatchDistributionSharded sends all deliveries for one object to the same instance. As a need, it is a
	// requirement: only a sharded mechanism satisfies it.
	WatchDistributionSharded WatchDistribution = "sharded"
)

// SatisfiedBy reports whether a channel with distribution provided can serve handlers that need d. An empty value
// means WatchDistributionAll. WatchDistributionShared is satisfied by any value. WatchDistributionSharded is
// satisfied only by WatchDistributionSharded. WatchDistributionAll is satisfied only by WatchDistributionAll.
func (d WatchDistribution) SatisfiedBy(provided WatchDistribution) bool {
	// Empty means WatchDistributionAll, as in WatchOptions.WithDefaults.
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

// WatchOptionsInformer is an Informer that describes what it provides. Implement it on an informer that sends less
// than the whole object, does not send live changes, or splits a channel. Then the app rejects a kind that the
// informer cannot serve, instead of losing deliveries.
//
// An update must include the previous object, with the same detail. The OpinionatedWatcher returns an error for an
// update without one. Nothing checks this for you.
//
// Experimental: this API may change in a minor release.
type WatchOptionsInformer interface {
	Informer
	// WatchOptions returns what this informer provides to a kind's handlers.
	WatchOptions() WatchOptions
}

// InformerWatchOptions returns what inf provides, and false if inf does not say. It looks through wrappers that
// implement Unwrap() Informer.
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
