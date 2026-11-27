package operator

import (
	"maps"

	"github.com/grafana/grafana-app-sdk/resource"
)

// WatchIgnoreFunc reports whether a live change or a resync can leave out an object, based on the current object
// only. It returns true to leave the object out.
//
// Experimental: this API may change in a minor release.
type WatchIgnoreFunc func(current resource.Object) bool

// Ignores calls f. It never leaves out an object that is being deleted. A nil f leaves out nothing.
func (f WatchIgnoreFunc) Ignores(current resource.Object) bool {
	return f != nil && current.GetDeletionTimestamp() == nil && f(current)
}

// WatchUnchangedFunc reports whether an object is unchanged since its previous version, for the fields your handlers
// use. It returns true to leave the object out.
//
// Experimental: this API may change in a minor release.
type WatchUnchangedFunc func(previous, current resource.Object) bool

// Unchanged calls f. It reports a change when f is nil, when previous is nil, when the object is being deleted, or
// when the UID is missing or different (the object was deleted and created again).
func (f WatchUnchangedFunc) Unchanged(previous, current resource.Object) bool {
	if f == nil || previous == nil || current.GetDeletionTimestamp() != nil {
		return false
	}
	if uid := current.GetUID(); uid == "" || uid != previous.GetUID() {
		return false
	}
	return f(previous, current)
}

// SameResourceVersion reports an object unchanged when its resourceVersion is the same, which means nobody wrote to
// it.
//
// Experimental: this API may change in a minor release.
func SameResourceVersion(previous, current resource.Object) bool {
	rv := current.GetResourceVersion()
	return rv != "" && rv == previous.GetResourceVersion()
}

// SameGeneration reports an object unchanged when its generation is the same, so writes to status and metadata are
// left out. If either object has no generation, it compares resourceVersion instead.
//
// Experimental: this API may change in a minor release.
func SameGeneration(previous, current resource.Object) bool {
	if previous.GetGeneration() == 0 || current.GetGeneration() == 0 {
		return SameResourceVersion(previous, current)
	}
	return previous.GetGeneration() == current.GetGeneration()
}

// SameLabels reports an object unchanged when its labels are.
//
// Experimental: this API may change in a minor release.
func SameLabels(previous, current resource.Object) bool {
	return maps.Equal(previous.GetLabels(), current.GetLabels())
}

// SameAnnotations reports an object unchanged when its annotations are.
//
// Experimental: this API may change in a minor release.
func SameAnnotations(previous, current resource.Object) bool {
	return maps.Equal(previous.GetAnnotations(), current.GetAnnotations())
}

// AllUnchanged reports an object unchanged only when every check reports it unchanged. With no checks, or with a
// nil check, it reports a change.
//
// Experimental: this API may change in a minor release.
func AllUnchanged(checks ...WatchUnchangedFunc) WatchUnchangedFunc {
	return func(previous, current resource.Object) bool {
		if len(checks) == 0 {
			return false
		}
		for _, check := range checks {
			if check == nil || !check(previous, current) {
				return false
			}
		}
		return true
	}
}

// AlreadyObserved returns a WatchIgnoreFunc that leaves out an object when its observed generation equals its
// generation. observedGeneration reads the observed generation from the object, and returns false if there is none.
// The object is kept when either value is missing.
//
// Experimental: this API may change in a minor release.
func AlreadyObserved(observedGeneration func(obj resource.Object) (int64, bool)) WatchIgnoreFunc {
	return func(current resource.Object) bool {
		generation := current.GetGeneration()
		if generation == 0 || observedGeneration == nil {
			return false
		}
		observed, ok := observedGeneration(current)
		return ok && observed == generation
	}
}
