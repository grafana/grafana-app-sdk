package operator

import (
	"fmt"
	"strings"
)

// WatchOptions is what a kind's handlers need of the changes they are watched by, and what a watch mechanism
// provides. simple.App compares the two, which is why both sides use one type: an axis added here is added to the
// need, to what a mechanism provides, and to the comparison at once.
//
// Experimental: this API may change in a minor release.
type WatchOptions struct {
	// Detail is the least a delivery must carry: the whole object, or the identity of the object it is about.
	// Its zero value needs WatchDetailFullObject.
	Detail WatchDetail
	// Triggers are what must cause a delivery, as a set rather than a choice: a mechanism may both deliver each
	// change and re-deliver everything on a schedule. Its zero value needs WatchTriggerChange.
	Triggers WatchTriggers
}

// SatisfiedBy reports whether a mechanism providing provided can serve handlers needing o, which it does when it
// carries at least the detail they read and triggers at least the deliveries they need. The axes are independent: a
// mechanism may carry the objects a kind reads and still not deliver on what makes it act.
func (o WatchOptions) SatisfiedBy(provided WatchOptions) bool {
	return o.Detail.SatisfiedBy(provided.Detail) && o.triggers()&^provided.triggers() == 0
}

// String describes both axes, for logs and errors.
func (o WatchOptions) String() string {
	detail := o.Detail
	if detail == "" {
		detail = WatchDetailFullObject
	}
	return fmt.Sprintf("detail=%s triggers=%s", detail, o.triggers())
}

func (o WatchOptions) triggers() WatchTriggers {
	if o.Triggers == 0 {
		return WatchTriggerChange
	}
	return o.Triggers
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

// WatchTriggers is a set of what causes a delivery. It is read two ways: as what a watch mechanism delivers on, and
// as what a kind's handlers need to be delivered on. A mechanism satisfies a kind when its set contains the kind's.
//
// Experimental: this API may change in a minor release.
type WatchTriggers uint8

const (
	// WatchTriggerChange delivers each change, as it happens. A watch and an event bus do this, and it is what a
	// kind needs by needing nothing: a reconciler acting on a change has to be told of it.
	WatchTriggerChange WatchTriggers = 1 << iota
	// WatchTriggerSchedule re-delivers every object on a schedule, whether it changed or not, so handlers served by
	// it re-evaluate everything on each pass — age-based cleanup, for instance.
	//
	// More than one mechanism provides it, and they differ in what the pass reads. The API watch re-delivers from
	// its cache (InformerOptions.CacheResyncInterval, unset by default and disabled when unset), so a pass costs
	// nothing and discovers nothing the stream did not deliver. A re-list reads the API, so its pass is both the
	// re-evaluation and the recovery of anything missed, at the cost of a list. A kind which needs the sweep only
	// to re-evaluate state it has is served by either.
	WatchTriggerSchedule
)

// String names the triggers in the set, for logs and errors.
func (t WatchTriggers) String() string {
	names := make([]string, 0, 2)
	if t&WatchTriggerChange != 0 {
		names = append(names, "change")
	}
	if t&WatchTriggerSchedule != 0 {
		names = append(names, "schedule")
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, "+")
}

// WatchOptionsInformer is an Informer which says what it provides. An informer which does not implement it is not
// taken to provide less: nothing can be read off it, so simple.App cannot check it against a kind's needs and does
// not try. Implement it on any informer which carries less than the whole object, or which does not deliver each
// change, so that pairing it with a kind it cannot serve fails rather than silently losing deliveries.
//
// Experimental: this API may change in a minor release.
type WatchOptionsInformer interface {
	Informer
	// WatchOptions is what this informer gives a kind's handlers.
	WatchOptions() WatchOptions
}

// InformerWatchOptions reports what inf says it provides, and whether it says anything at all.
func InformerWatchOptions(inf Informer) (WatchOptions, bool) {
	if described, ok := inf.(WatchOptionsInformer); ok {
		return described.WatchOptions(), true
	}
	return WatchOptions{}, false
}
