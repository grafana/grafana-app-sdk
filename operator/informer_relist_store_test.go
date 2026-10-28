package operator

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

const relistTestNamespace = "default"

func relistObj(name string) *resource.UntypedObject {
	return &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: relistTestNamespace},
	}
}

// relistObjRV is an object tagged with a resource version, for exercising the arbitration Replace does against live
// write-throughs.
func relistObjRV(name string, rv int64) *resource.UntypedObject {
	o := relistObj(name)
	o.ResourceVersion = strconv.FormatInt(rv, 10)
	return o
}

func relistNames(objs []resource.Object) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		out[i] = o.GetName()
	}
	return out
}

func TestRelistStore_ReplaceReportsDiff(t *testing.T) {
	s := newRelistStore()

	added, updated, removed := s.Replace([]resource.Object{relistObj("a"), relistObj("b")}, "")
	assert.ElementsMatch(t, []string{"a", "b"}, relistNames(added), "first replace adds everything")
	assert.Empty(t, updated, "first replace updates nothing")
	assert.Empty(t, removed, "first replace removes nothing")
	assert.ElementsMatch(t, []string{"a", "b"}, relistNames(s.List()))

	added, updated, removed = s.Replace([]resource.Object{relistObj("a"), relistObj("c")}, "")
	assert.Equal(t, []string{"c"}, relistNames(added), "c newly appeared")
	assert.Equal(t, []string{"a"}, relistNames(updated), "a was present before and now")
	assert.Equal(t, []string{"b"}, relistNames(removed), "b vanished")
	assert.ElementsMatch(t, []string{"a", "c"}, relistNames(s.List()))
}

func TestRelistStore_WriteThrough(t *testing.T) {
	s := newRelistStore()
	s.Replace([]resource.Object{relistObj("a")}, "")

	assert.True(t, s.Update(relistObj("b")))
	assert.ElementsMatch(t, []string{"a", "b"}, relistNames(s.List()))

	s.DeleteAt(relistTestNamespace, "a", "1")
	assert.Equal(t, []string{"b"}, relistNames(s.List()))

	got, ok := s.Get(relistTestNamespace, "b")
	assert.True(t, ok)
	assert.Equal(t, "b", got.GetName())
	_, ok = s.Get(relistTestNamespace, "a")
	assert.False(t, ok)
}

// A live write newer than the list snapshot is carried forward rather than reported removed: the watch is
// open while the list runs, so a write can land after the snapshot was read.
func TestRelistStore_ReplaceCarriesForwardLiveWriteNewerThanSnapshot(t *testing.T) {
	s := newRelistStore()
	s.Replace([]resource.Object{relistObjRV("a", 100)}, "100")

	// "b" is live-added at 200, after a snapshot the next re-list reads at 150.
	s.Update(relistObjRV("b", 200))

	added, updated, removed := s.Replace([]resource.Object{relistObjRV("a", 100)}, "150")
	assert.Empty(t, added)
	assert.Equal(t, []string{"a"}, relistNames(updated))
	assert.Empty(t, removed, "a live write newer than the snapshot must not be reported removed")
	assert.ElementsMatch(t, []string{"a", "b"}, relistNames(s.List()), "b is carried forward")

	added, updated, _ = s.Replace([]resource.Object{relistObjRV("a", 100), relistObjRV("b", 200)}, "250")
	assert.Empty(t, added, "b is already in the store, not a new add")
	assert.ElementsMatch(t, []string{"a", "b"}, relistNames(updated))
}

// An object a newer live delete already evicted is suppressed rather than resurrected as a spurious add. Once a
// snapshot postdates the delete the tombstone is spent, so a genuine later re-create adds normally.
func TestRelistStore_ReplaceSuppressesResurrectionOfLiveDeleted(t *testing.T) {
	s := newRelistStore()
	s.Replace([]resource.Object{relistObjRV("a", 100), relistObjRV("x", 120)}, "120")

	s.DeleteAt(relistTestNamespace, "x", "200")
	assert.Equal(t, []string{"a"}, relistNames(s.List()))

	added, _, removed := s.Replace([]resource.Object{relistObjRV("a", 100), relistObjRV("x", 120)}, "150")
	assert.Empty(t, added, "the stale snapshot must not resurrect the live-deleted object")
	assert.Empty(t, removed)
	assert.Equal(t, []string{"a"}, relistNames(s.List()), "x stays evicted")

	s.Replace([]resource.Object{relistObjRV("a", 100)}, "250")
	added, _, _ = s.Replace([]resource.Object{relistObjRV("a", 100), relistObjRV("x", 300)}, "300")
	assert.Equal(t, []string{"x"}, relistNames(added), "a re-create after the tombstone is spent is a real add")
}

// An absent list version disables the arbitration and does a plain wholesale swap: an object the snapshot omits is
// removed regardless of its version, and no tombstone lingers.
func TestRelistStore_UndatedSnapshotDoesWholesaleSwap(t *testing.T) {
	s := newRelistStore()
	s.Replace([]resource.Object{relistObjRV("a", 100), relistObjRV("b", 200)}, "")

	added, updated, removed := s.Replace([]resource.Object{relistObjRV("a", 100)}, "")
	assert.Empty(t, added)
	assert.Equal(t, []string{"a"}, relistNames(updated))
	assert.Equal(t, []string{"b"}, relistNames(removed), "an undated snapshot must remove a vanished object")

	s.DeleteAt(relistTestNamespace, "a", "300")
	added, _, _ = s.Replace([]resource.Object{relistObjRV("a", 100)}, "")
	assert.Equal(t, []string{"a"}, relistNames(added), "an undated snapshot must not suppress via a tombstone")
}

// An opaque (non-numeric) resource version reads as 0 everywhere, which disables the arbitration: the delete leaves
// no tombstone, so a snapshot which predates it resurrects the object as a spurious add and the correction is the
// next re-list.
func TestRelistStore_OpaqueResourceVersionDisablesArbitration(t *testing.T) {
	s := newRelistStore()
	opaque := func(name string) *resource.UntypedObject {
		o := relistObj(name)
		o.ResourceVersion = "opaque-" + name
		return o
	}
	s.Replace([]resource.Object{opaque("x")}, "opaque-list")

	s.DeleteAt(relistTestNamespace, "x", "opaque-delete")
	assert.Empty(t, s.List(), "the delete still evicts")

	added, _, _ := s.Replace([]resource.Object{opaque("x")}, "opaque-list")
	assert.Equal(t, []string{"x"}, relistNames(added), "with no arbitration the stale snapshot re-adds the object")

	_, _, removed := s.Replace(nil, "opaque-list")
	assert.Equal(t, []string{"x"}, relistNames(removed), "the next re-list corrects it")
}

func TestRelistStore_ListEmpty(t *testing.T) {
	assert.Empty(t, newRelistStore().List())
}

func TestRelistStore_UpdateReportsWhetherItTookTheWrite(t *testing.T) {
	t.Run("plain write", func(t *testing.T) {
		s := newRelistStore()
		assert.True(t, s.Update(relistObjRV("a", 10)))
		assert.Equal(t, []string{"a"}, relistNames(s.List()))
	})

	t.Run("superseded by a newer delete", func(t *testing.T) {
		s := newRelistStore()
		s.Update(relistObjRV("a", 10))
		s.DeleteAt(relistTestNamespace, "a", "20")

		assert.False(t, s.Update(relistObjRV("a", 10)), "the delete is newer, so the write is dropped")
		assert.Empty(t, s.List(), "and the object stays evicted")
	})

	t.Run("at or past the delete is a re-creation", func(t *testing.T) {
		s := newRelistStore()
		s.DeleteAt(relistTestNamespace, "a", "20")

		assert.True(t, s.Update(relistObjRV("a", 20)))
		assert.Equal(t, []string{"a"}, relistNames(s.List()))
	})

	t.Run("no name to key it by", func(t *testing.T) {
		s := newRelistStore()
		assert.False(t, s.Update(&resource.UntypedObject{}))
		assert.False(t, s.Update(nil))
		assert.Empty(t, s.List())
	})
}

func TestBusStoreKey(t *testing.T) {
	assert.Equal(t, "name", relistStoreKey("", "name"), "a cluster-scoped object is keyed by name alone")
	assert.Equal(t, "ns/name", relistStoreKey("ns", "name"))
}

func TestParseResourceVersion(t *testing.T) {
	assert.Equal(t, int64(42), parseResourceVersion("42"))
	assert.Equal(t, int64(0), parseResourceVersion(""))
	assert.Equal(t, int64(0), parseResourceVersion("opaque"))
}
