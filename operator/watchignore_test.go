package operator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

func watchIgnoreObj(resourceVersion string, generation int64) *resource.UntypedObject {
	return &resource.UntypedObject{ObjectMeta: metav1.ObjectMeta{
		Name: "obj", UID: "uid-1", ResourceVersion: resourceVersion, Generation: generation,
	}}
}

func deleting(obj *resource.UntypedObject) *resource.UntypedObject {
	dt := metav1.NewTime(time.Time{})
	obj.SetDeletionTimestamp(&dt)
	return obj
}

func TestSameResourceVersion(t *testing.T) {
	assert.True(t, SameResourceVersion(watchIgnoreObj("5", 1), watchIgnoreObj("5", 1)))
	assert.False(t, SameResourceVersion(watchIgnoreObj("5", 1), watchIgnoreObj("6", 1)), "a status write changes it")
	assert.False(t, SameResourceVersion(watchIgnoreObj("", 1), watchIgnoreObj("", 1)), "an empty one identifies nothing")
}

func TestSameGeneration(t *testing.T) {
	assert.True(t, SameGeneration(watchIgnoreObj("5", 1), watchIgnoreObj("6", 1)), "a status write is passed over")
	assert.False(t, SameGeneration(watchIgnoreObj("5", 1), watchIgnoreObj("6", 2)), "a spec write is not")
	assert.True(t, SameGeneration(watchIgnoreObj("5", 0), watchIgnoreObj("5", 0)), "without one, the version decides")
	assert.False(t, SameGeneration(watchIgnoreObj("5", 0), watchIgnoreObj("6", 0)))
}

func TestSameLabelsAndAnnotations(t *testing.T) {
	prev, cur := watchIgnoreObj("5", 1), watchIgnoreObj("6", 1)
	prev.SetLabels(map[string]string{"paused": "true"})
	cur.SetLabels(map[string]string{"paused": "true"})
	assert.True(t, SameLabels(prev, cur))
	cur.SetLabels(map[string]string{"paused": "false"})
	assert.False(t, SameLabels(prev, cur))

	assert.True(t, SameAnnotations(prev, cur))
	cur.SetAnnotations(map[string]string{"note": "x"})
	assert.False(t, SameAnnotations(prev, cur))
}

func TestAllUnchanged(t *testing.T) {
	prev, cur := watchIgnoreObj("5", 1), watchIgnoreObj("6", 1)
	cur.SetLabels(map[string]string{"paused": "true"})

	assert.True(t, AllUnchanged(SameGeneration)(prev, cur))
	assert.False(t, AllUnchanged(SameGeneration, SameLabels)(prev, cur), "a change either check sees is delivered")
	assert.False(t, AllUnchanged()(prev, cur), "no checks must not leave everything out")
	assert.False(t, AllUnchanged(SameGeneration, nil)(prev, cur), "a nil check reports a change rather than panicking")
	assert.False(t, AllUnchanged(SameGeneration, nil).Unchanged(prev, cur))
}

func TestAlreadyObserved(t *testing.T) {
	observed := func(seen int64, ok bool) func(resource.Object) (int64, bool) {
		return func(resource.Object) (int64, bool) { return seen, ok }
	}
	assert.True(t, AlreadyObserved(observed(3, true))(watchIgnoreObj("5", 3)))
	assert.False(t, AlreadyObserved(observed(2, true))(watchIgnoreObj("5", 3)), "a newer generation is not handled yet")
	assert.False(t, AlreadyObserved(observed(3, false))(watchIgnoreObj("5", 3)), "nothing recorded")
	assert.False(t, AlreadyObserved(observed(0, true))(watchIgnoreObj("5", 0)), "no generation to compare")
	assert.False(t, AlreadyObserved(nil)(watchIgnoreObj("5", 3)), "a nil accessor observes nothing rather than panicking")
}

func TestWatchIgnoreFunc_Ignores(t *testing.T) {
	always := WatchIgnoreFunc(func(resource.Object) bool { return true })
	assert.True(t, always.Ignores(watchIgnoreObj("5", 1)))
	assert.False(t, always.Ignores(deleting(watchIgnoreObj("5", 1))), "a deletion must reach finalizer handlers")
	assert.False(t, WatchIgnoreFunc(nil).Ignores(watchIgnoreObj("5", 1)))
}

func TestWatchUnchangedFunc_Unchanged(t *testing.T) {
	always := WatchUnchangedFunc(func(resource.Object, resource.Object) bool { return true })
	assert.True(t, always.Unchanged(watchIgnoreObj("5", 1), watchIgnoreObj("5", 1)))
	assert.False(t, always.Unchanged(watchIgnoreObj("5", 1), deleting(watchIgnoreObj("6", 1))),
		"a deletion must reach finalizer handlers")
	assert.False(t, always.Unchanged(nil, watchIgnoreObj("5", 1)), "with no previous delivery it is new")
	assert.False(t, WatchUnchangedFunc(nil).Unchanged(watchIgnoreObj("5", 1), watchIgnoreObj("5", 1)))

	replacement := watchIgnoreObj("5", 1)
	replacement.SetUID("uid-2")
	assert.False(t, always.Unchanged(watchIgnoreObj("5", 1), replacement),
		"a replacement, deleted and recreated under the same name, must be delivered")
	assert.True(t, SameGeneration(watchIgnoreObj("5", 1), replacement),
		"the check alone cannot tell, which is why Unchanged guards it")

	identity := watchIgnoreObj("5", 1)
	identity.SetUID("")
	assert.False(t, always.Unchanged(identity, identity.Copy()), "without a UID a replacement cannot be ruled out")
}
