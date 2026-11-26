package operator

import (
	"maps"

	"github.com/grafana/grafana-app-sdk/resource"
)

// WatchIgnoreFunc reports whether a resync can leave out an object, judged on the current object alone.
//
// Experimental: this API may change in a minor release.
type WatchIgnoreFunc func(current resource.Object) bool

// Ignores applies f. It never leaves out an object being deleted, and a nil f leaves out nothing.
func (f WatchIgnoreFunc) Ignores(current resource.Object) bool {
	return f != nil && current.GetDeletionTimestamp() == nil && f(current)
}

// WatchUnchangedFunc reports whether an object is unchanged since its last delivery, as far as the handlers care.
//
// Experimental: this API may change in a minor release.
type WatchUnchangedFunc func(previous, current resource.Object) bool

// Unchanged applies f. An object being deleted, with no previous delivery, without a UID, or with a different UID
// (a replacement) is never unchanged, and a nil f reports nothing unchanged.
func (f WatchUnchangedFunc) Unchanged(previous, current resource.Object) bool {
	if f == nil || previous == nil || current.GetDeletionTimestamp() != nil {
		return false
	}
	if uid := current.GetUID(); uid == "" || uid != previous.GetUID() {
		return false
	}
	return f(previous, current)
}

// SameResourceVersion reports an object unchanged when its resourceVersion is the same, meaning nobody wrote to it.
//
// Experimental: this API may change in a minor release.
func SameResourceVersion(previous, current resource.Object) bool {
	rv := current.GetResourceVersion()
	return rv != "" && rv == previous.GetResourceVersion()
}

// SameGeneration reports an object unchanged when its generation is the same, so status and metadata writes are
// left out. Without a generation, it compares resourceVersions.
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

// AllUnchanged reports an object unchanged only when every check does. With no checks, or a nil one, it reports a
// change.
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

// AlreadyObserved leaves out an object whose observedGeneration, read by the given accessor, equals its generation.
// It keeps the object when either is missing.
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
