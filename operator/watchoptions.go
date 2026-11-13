package operator

import "fmt"

// WatchOptions is what a kind's handlers need of the changes they are watched by, and what a watch mechanism
// provides. simple.App compares the two, which is why both sides use one type: an axis added here is added to the
// need, to what a mechanism provides, and to the comparison at once.
//
// Experimental: this API may change in a minor release.
type WatchOptions struct {
	// Detail is the least a delivery must carry: the whole object, or the identity of the object it is about. Its
	// zero value needs WatchDetailFullObject.
	Detail WatchDetail
	// Trigger is what must cause a delivery. Its zero value needs WatchTriggerChange.
	Trigger WatchTrigger
}

// SatisfiedBy reports whether a mechanism providing provided can serve handlers needing o, which it does when it
// satisfies every axis: a mechanism may carry the objects a kind reads and still not be told of a change in time
// to deliver one.
func (o WatchOptions) SatisfiedBy(provided WatchOptions) bool {
	return o.Detail.SatisfiedBy(provided.Detail) && o.Trigger.SatisfiedBy(provided.Trigger)
}

// String describes both axes, for logs and errors.
func (o WatchOptions) String() string {
	detail, trigger := o.Detail, o.Trigger
	if detail == "" {
		detail = WatchDetailFullObject
	}
	if trigger == "" {
		trigger = WatchTriggerChange
	}
	return fmt.Sprintf("detail=%s trigger=%s", detail, trigger)
}

// WatchDetail is how much a delivery carries. It is read two ways: as what a watch mechanism's deliveries carry,
// and as the least a kind's handlers need.
//
// It is about the delivery, not about the informer behind it: whether that informer holds a cache of the kind, and
// whether its deliveries are shared among an app's replicas, are properties of the informer which no handler reads
// through this.
//
// Experimental: this API may change in a minor release.
type WatchDetail string

const (
	// WatchDetailFullObject is a delivery carrying the whole object. Handlers served by it read that object off the
	// event, and read anything else through the API. It is what every informer the SDK ships carries, and what an
	// informer is taken to carry when it says nothing, so it is also what a kind needs by needing nothing.
	WatchDetailFullObject WatchDetail = "full-object"
	// WatchDetailIdentity is a delivery carrying only which object it is about — namespace, name and version.
	// Handlers served by it re-read the object they act on, so a mechanism can carry it without holding a cache
	// and without a read of its own per delivery.
	WatchDetailIdentity WatchDetail = "identity"
)

// SatisfiedBy reports whether deliveries carrying provided can serve handlers needing d.
func (d WatchDetail) SatisfiedBy(provided WatchDetail) bool {
	// Anything this build of the SDK does not recognize reads as the object, so that an unknown need is needed in
	// full and an unknown provision is not assumed to be short.
	return provided != WatchDetailIdentity || d == WatchDetailIdentity
}

// WatchTrigger is what causes a delivery. It is read two ways: as what a watch mechanism delivers on, and as what a
// kind's handlers need to be delivered on.
//
// Experimental: this API may change in a minor release.
type WatchTrigger string

const (
	// WatchTriggerChange delivers each change, as it happens. It is what a watch and an event bus do, and what a
	// kind needs by needing nothing: a reconciler acting on a change has to be told of it.
	WatchTriggerChange WatchTrigger = "change"
	// WatchTriggerSchedule delivers every object on a schedule, whether it changed or not, so an individual change
	// is seen on the next pass rather than when it happens. A re-list does this — its deliveries carry whole
	// objects, it is only the individual change it does not announce. Handlers served by it re-evaluate everything
	// on each pass, as age-based cleanup does.
	WatchTriggerSchedule WatchTrigger = "schedule"
)

// SatisfiedBy reports whether a mechanism delivering on provided can serve handlers needing d.
func (d WatchTrigger) SatisfiedBy(provided WatchTrigger) bool {
	// An unrecognized value reads as WatchTriggerChange, for the same reason as WatchDetail.
	return provided != WatchTriggerSchedule || d == WatchTriggerSchedule
}

// WatchOptionsInformer is an Informer which provides less than the default WatchOptions, and says so. An informer
// which does not implement it carries whole objects and delivers each change, which is what every informer the SDK
// shipped before this interface existed does.
//
// Experimental: this API may change in a minor release.
type WatchOptionsInformer interface {
	Informer
	// WatchOptions is what this informer gives a kind's handlers.
	WatchOptions() WatchOptions
}

// InformerWatchOptions reports what inf provides, which is a delivery per change carrying the whole object unless
// inf says otherwise.
func InformerWatchOptions(inf Informer) WatchOptions {
	if described, ok := inf.(WatchOptionsInformer); ok {
		return described.WatchOptions()
	}
	return WatchOptions{Detail: WatchDetailFullObject, Trigger: WatchTriggerChange}
}
