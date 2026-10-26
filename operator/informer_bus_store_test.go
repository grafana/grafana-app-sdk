package operator

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

const busTestNamespace = "default"

func busObj(name string) *resource.UntypedObject {
	return &resource.UntypedObject{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: busTestNamespace},
	}
}

// busObjRV is an object tagged with a resource version, for exercising the arbitration Replace does against live
// write-throughs.
func busObjRV(name string, rv int64) *resource.UntypedObject {
	o := busObj(name)
	o.ResourceVersion = strconv.FormatInt(rv, 10)
	return o
}

// busUpdateNames is the current side of each update pair.
func busUpdateNames(updates []busUpdate) []string {
	out := make([]string, len(updates))
	for i, u := range updates {
		out[i] = u.current.GetName()
	}
	return out
}

func busNames(objs []resource.Object) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		out[i] = o.GetName()
	}
	return out
}

func TestBusStore_ReplaceReportsDiff(t *testing.T) {
	s := newBusStore()

	added, updated, removed := s.Replace([]resource.Object{busObj("a"), busObj("b")}, "")
	assert.ElementsMatch(t, []string{"a", "b"}, busNames(added), "first replace adds everything")
	assert.Empty(t, updated, "first replace updates nothing")
	assert.Empty(t, removed, "first replace removes nothing")
	assert.ElementsMatch(t, []string{"a", "b"}, busNames(s.List()))

	added, updated, removed = s.Replace([]resource.Object{busObj("a"), busObj("c")}, "")
	assert.Equal(t, []string{"c"}, busNames(added), "c newly appeared")
	assert.Equal(t, []string{"a"}, busUpdateNames(updated), "a was present before and now")
	assert.Equal(t, []string{"b"}, busNames(removed), "b vanished")
	assert.ElementsMatch(t, []string{"a", "c"}, busNames(s.List()))
}

func TestBusStore_WriteThrough(t *testing.T) {
	s := newBusStore()
	s.Replace([]resource.Object{busObj("a")}, "")

	_, took := s.Update(busObj("b"))
	assert.True(t, took)
	assert.ElementsMatch(t, []string{"a", "b"}, busNames(s.List()))

	s.DeleteAt(busTestNamespace, "a", "1")
	assert.Equal(t, []string{"b"}, busNames(s.List()))

	got, ok := s.Get(busTestNamespace, "b")
	assert.True(t, ok)
	assert.Equal(t, "b", got.GetName())
	_, ok = s.Get(busTestNamespace, "a")
	assert.False(t, ok)
}

// A live write newer than the list snapshot is carried forward rather than reported removed: the subscription is
// open while the list runs, so a write can land after the snapshot was read.
func TestBusStore_ReplaceCarriesForwardLiveWriteNewerThanSnapshot(t *testing.T) {
	s := newBusStore()
	s.Replace([]resource.Object{busObjRV("a", 100)}, "100")

	// "b" is live-added at 200, after a snapshot the next re-list reads at 150.
	s.Update(busObjRV("b", 200))

	added, updated, removed := s.Replace([]resource.Object{busObjRV("a", 100)}, "150")
	assert.Empty(t, added)
	assert.Equal(t, []string{"a"}, busUpdateNames(updated))
	assert.Empty(t, removed, "a live write newer than the snapshot must not be reported removed")
	assert.ElementsMatch(t, []string{"a", "b"}, busNames(s.List()), "b is carried forward")

	added, updated, _ = s.Replace([]resource.Object{busObjRV("a", 100), busObjRV("b", 200)}, "250")
	assert.Empty(t, added, "b is already in the store, not a new add")
	assert.ElementsMatch(t, []string{"a", "b"}, busUpdateNames(updated))
}

// An object a newer live delete already evicted is suppressed rather than resurrected as a spurious add. Once a
// snapshot postdates the delete the tombstone is spent, so a genuine later re-create adds normally.
func TestBusStore_ReplaceSuppressesResurrectionOfLiveDeleted(t *testing.T) {
	s := newBusStore()
	s.Replace([]resource.Object{busObjRV("a", 100), busObjRV("x", 120)}, "120")

	s.DeleteAt(busTestNamespace, "x", "200")
	assert.Equal(t, []string{"a"}, busNames(s.List()))

	added, _, removed := s.Replace([]resource.Object{busObjRV("a", 100), busObjRV("x", 120)}, "150")
	assert.Empty(t, added, "the stale snapshot must not resurrect the live-deleted object")
	assert.Empty(t, removed)
	assert.Equal(t, []string{"a"}, busNames(s.List()), "x stays evicted")

	s.Replace([]resource.Object{busObjRV("a", 100)}, "250")
	added, _, _ = s.Replace([]resource.Object{busObjRV("a", 100), busObjRV("x", 300)}, "300")
	assert.Equal(t, []string{"x"}, busNames(added), "a re-create after the tombstone is spent is a real add")
}

// An absent list version disables the arbitration and does a plain wholesale swap: an object the snapshot omits is
// removed regardless of its version, and no tombstone lingers.
func TestBusStore_UndatedSnapshotDoesWholesaleSwap(t *testing.T) {
	s := newBusStore()
	s.Replace([]resource.Object{busObjRV("a", 100), busObjRV("b", 200)}, "")

	added, updated, removed := s.Replace([]resource.Object{busObjRV("a", 100)}, "")
	assert.Empty(t, added)
	assert.Equal(t, []string{"a"}, busUpdateNames(updated))
	assert.Equal(t, []string{"b"}, busNames(removed), "an undated snapshot must remove a vanished object")

	s.DeleteAt(busTestNamespace, "a", "300")
	added, _, _ = s.Replace([]resource.Object{busObjRV("a", 100)}, "")
	assert.Equal(t, []string{"a"}, busNames(added), "an undated snapshot must not suppress via a tombstone")
}

// An opaque (non-numeric) resource version reads as 0 everywhere, which disables the arbitration: the delete leaves
// no tombstone, so a snapshot which predates it resurrects the object as a spurious add and the correction is the
// next re-list.
func TestBusStore_OpaqueResourceVersionDisablesArbitration(t *testing.T) {
	s := newBusStore()
	opaque := func(name string) *resource.UntypedObject {
		o := busObj(name)
		o.ResourceVersion = "opaque-" + name
		return o
	}
	s.Replace([]resource.Object{opaque("x")}, "opaque-list")

	s.DeleteAt(busTestNamespace, "x", "opaque-delete")
	assert.Empty(t, s.List(), "the delete still evicts")

	added, _, _ := s.Replace([]resource.Object{opaque("x")}, "opaque-list")
	assert.Equal(t, []string{"x"}, busNames(added), "with no arbitration the stale snapshot re-adds the object")

	_, _, removed := s.Replace(nil, "opaque-list")
	assert.Equal(t, []string{"x"}, busNames(removed), "the next re-list corrects it")
}

func TestBusStore_ListEmpty(t *testing.T) {
	assert.Empty(t, newBusStore().List())
}

func TestBusStore_UpdateReportsWhetherItTookTheWrite(t *testing.T) {
	t.Run("plain write", func(t *testing.T) {
		s := newBusStore()
		_, took := s.Update(busObjRV("a", 10))
		assert.True(t, took)
		assert.Equal(t, []string{"a"}, busNames(s.List()))
	})

	t.Run("superseded by a newer delete", func(t *testing.T) {
		s := newBusStore()
		s.Update(busObjRV("a", 10))
		s.DeleteAt(busTestNamespace, "a", "20")

		_, took := s.Update(busObjRV("a", 10))
		assert.False(t, took, "the delete is newer, so the write is dropped")
		assert.Empty(t, s.List(), "and the object stays evicted")
	})

	t.Run("at or past the delete is a re-creation", func(t *testing.T) {
		s := newBusStore()
		s.DeleteAt(busTestNamespace, "a", "20")

		_, took := s.Update(busObjRV("a", 20))
		assert.True(t, took)
		assert.Equal(t, []string{"a"}, busNames(s.List()))
	})

	t.Run("no name to key it by", func(t *testing.T) {
		s := newBusStore()
		_, took := s.Update(&resource.UntypedObject{})
		assert.False(t, took)
		_, tookNil := s.Update(nil)
		assert.False(t, tookNil)
		assert.Empty(t, s.List())
	})
}

func TestBusStoreKey(t *testing.T) {
	assert.Equal(t, "name", busStoreKey("", "name"), "a cluster-scoped object is keyed by name alone")
	assert.Equal(t, "ns/name", busStoreKey("ns", "name"))
}

func TestParseResourceVersion(t *testing.T) {
	assert.Equal(t, int64(42), parseResourceVersion("42"))
	assert.Equal(t, int64(0), parseResourceVersion(""))
	assert.Equal(t, int64(0), parseResourceVersion("opaque"))
}

// A stored object newer than the list snapshot must survive a re-list that still lists an older copy of it. Taking
// the listed state would roll the snapshot back to a superseded object and re-deliver it as an update.
func TestBusStore_ReplaceKeepsNewerStoredEntryForRetainedKeys(t *testing.T) {
	s := newBusStore()
	s.Replace([]resource.Object{busObjRV("x", 120)}, "120")

	// A live write lands at 200, after the snapshot the next re-list reads at 150.
	s.Update(busObjRV("x", 200))

	added, updated, removed := s.Replace([]resource.Object{busObjRV("x", 120)}, "150")
	assert.Empty(t, added)
	assert.Empty(t, removed)
	assert.Empty(t, updated, "the live write already delivered the newer state; the stale one is not re-delivered")

	stored, ok := s.Get(busTestNamespace, "x")
	require.True(t, ok)
	assert.Equal(t, "200", stored.GetResourceVersion(), "the newer stored object is kept")

	// A snapshot that postdates the write lists it normally again.
	_, updated, _ = s.Replace([]resource.Object{busObjRV("x", 200)}, "250")
	assert.Equal(t, []string{"x"}, busUpdateNames(updated))
}

// Replace reports the previous state alongside the current one, so a watcher receives a real before/after pair.
func TestBusStore_ReplaceReportsPreviousStateForUpdates(t *testing.T) {
	s := newBusStore()
	s.Replace([]resource.Object{busObjRV("x", 100)}, "100")

	_, updated, _ := s.Replace([]resource.Object{busObjRV("x", 200)}, "200")

	require.Len(t, updated, 1)
	assert.Equal(t, "100", updated[0].previous.GetResourceVersion())
	assert.Equal(t, "200", updated[0].current.GetResourceVersion())
}
