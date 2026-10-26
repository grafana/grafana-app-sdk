package operator

import (
	"maps"
	"strconv"
	"sync"

	"github.com/grafana/grafana-app-sdk/resource"
)

// busStore is a BusInformer's snapshot of one kind, keyed by namespace/name. It is refreshed wholesale on each
// re-list (Replace) and written through between re-lists (Update/DeleteAt) when a live event has just reported a
// fresh object. Diffing each re-list against it is how the informer catches changes the live stream never delivered.
type busStore struct {
	mu    sync.Mutex
	items map[string]busEntry
	// tombstones records keys evicted by a live delete, keyed to the delete's resource version, so that a re-list
	// whose snapshot predates the delete does not resurrect the object. Entries are dropped once a snapshot at or
	// past their version confirms the removal.
	tombstones map[string]int64
}

// busEntry is a stored object tagged with the resource version it was last written at, so that Replace can tell a
// live write which outran the list snapshot from an object the snapshot authoritatively dropped.
type busEntry struct {
	obj resource.Object
	rv  int64
}

func newBusStore() *busStore {
	return &busStore{items: map[string]busEntry{}, tombstones: map[string]int64{}}
}

// List returns a snapshot of the objects in the store, empty but non-nil before the first Replace.
func (s *busStore) List() []resource.Object {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]resource.Object, 0, len(s.items))
	for _, e := range s.items {
		out = append(out, e.obj)
	}
	return out
}

// Identifiers returns the namespace and name of every object in the store, empty but non-nil before the first
// Replace. It is the read a snapshot accessor serves: identities only, so no caller can mistake the store for a view
// of the objects themselves.
func (s *busStore) Identifiers() []resource.Identifier {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]resource.Identifier, 0, len(s.items))
	for _, e := range s.items {
		out = append(out, resource.Identifier{Namespace: e.obj.GetNamespace(), Name: e.obj.GetName()})
	}
	return out
}

// Get returns the stored object for a namespace and name, if there is one.
func (s *busStore) Get(namespace, name string) (resource.Object, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.items[busStoreKey(namespace, name)]
	return e.obj, ok
}

// Update writes obj into the store, tagged with its resource version, and reports whether the store took the write:
// an object with no name, or a write a newer delete supersedes, is refused. A write at or past a tombstoned delete
// supersedes it, because the object was re-created.
func (s *busStore) Update(obj resource.Object) bool {
	if obj == nil || obj.GetName() == "" {
		return false
	}
	key := busStoreKey(obj.GetNamespace(), obj.GetName())
	rv := parseResourceVersion(obj.GetResourceVersion())
	s.mu.Lock()
	defer s.mu.Unlock()
	if trv, ok := s.tombstones[key]; ok {
		if rv < trv {
			return false
		}
		delete(s.tombstones, key)
	}
	s.items[key] = busEntry{obj: obj, rv: rv}
	return true
}

// DeleteAt removes an object and tombstones it at the delete's resource version, so that a subsequent re-list whose
// snapshot predates that version does not resurrect the just-deleted object as a spurious add. An unparseable or
// absent version records no tombstone: there is nothing to arbitrate against.
func (s *busStore) DeleteAt(namespace, name, resourceVersion string) {
	key := busStoreKey(namespace, name)
	rv := parseResourceVersion(resourceVersion)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	if rv > 0 {
		s.tombstones[key] = rv
	}
}

// Replace swaps the store's contents for objs — the snapshot a list read at listResourceVersion — and reports the
// diff against the previous snapshot as three sets: added (keys not present before), updated (keys present before
// and now), and removed (keys present before but absent now, carrying their last-known state). The informer
// dispatches added as adds, so that an add handler still wakes for an object a re-list saw first, updated as
// updates, and removed as deletes.
//
// listResourceVersion arbitrates the snapshot against live write-throughs which raced it: the subscription is open
// while the list runs, so a write can land after the list read its snapshot. A live-added object newer than the
// snapshot is carried forward rather than reported removed, and a re-listed object a newer live delete already
// evicted is suppressed rather than resurrected. A version which is absent, unparseable (an opaque resource version)
// or zero disables the arbitration, leaving a plain wholesale swap.
func (s *busStore) Replace(objs []resource.Object, listResourceVersion string) (added, updated, removed []resource.Object) {
	listRV := parseResourceVersion(listResourceVersion)

	next := make(map[string]busEntry, len(objs))
	for _, obj := range objs {
		if obj == nil || obj.GetName() == "" {
			continue
		}
		next[busStoreKey(obj.GetNamespace(), obj.GetName())] = busEntry{obj: obj, rv: parseResourceVersion(obj.GetResourceVersion())}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.items
	prevTomb := s.tombstones

	// Without a dated snapshot every positive object version would read as newer than a zero one, so an
	// authoritative list could never remove a deleted object and tombstones would never expire.
	arbitrate := listRV > 0

	// result is the snapshot we install: objs, minus resurrections we suppress, plus live writes newer than the
	// snapshot which we carry forward.
	result := make(map[string]busEntry, len(next))
	maps.Copy(result, next)

	// Classify in objs order rather than map order so that added/updated are deterministic.
	seen := make(map[string]struct{}, len(objs))
	for _, obj := range objs {
		if obj == nil || obj.GetName() == "" {
			continue
		}
		key := busStoreKey(obj.GetNamespace(), obj.GetName())
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		e := next[key]
		if _, ok := prev[key]; ok {
			updated = append(updated, e.obj)
			continue
		}
		if arbitrate {
			if trv, ok := prevTomb[key]; ok && trv > listRV && e.rv < trv {
				delete(result, key)
				continue
			}
		}
		added = append(added, e.obj)
	}

	for key, e := range prev {
		if _, ok := next[key]; ok {
			continue
		}
		// A retained copy newer than the snapshot is a live write which landed after the list read its snapshot,
		// so the snapshot's silence about it is stale rather than authoritative.
		if arbitrate && e.rv > listRV {
			result[key] = e
			continue
		}
		removed = append(removed, e.obj)
	}

	// Keep only the tombstones the snapshot does not yet reflect and that no present object supersedes, so that a
	// tombstone cannot suppress an object forever. A wholesale swap makes no dated claim, so it drops them all.
	newTomb := make(map[string]int64, len(prevTomb))
	if arbitrate {
		for key, trv := range prevTomb {
			if trv <= listRV {
				continue
			}
			if _, present := result[key]; present {
				continue
			}
			newTomb[key] = trv
		}
	}

	s.items = result
	s.tombstones = newTomb
	return added, updated, removed
}

// busStoreKey builds the namespace/name store key, matching the cluster-scoped (empty namespace) and namespaced cases.
func busStoreKey(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

// parseResourceVersion is the only place the SDK assumes a resource version is numeric. The kubernetes API contract
// makes it opaque, so an unparseable version reads as 0, which disables the store's arbitration and the latency
// histograms rather than producing a wrong answer.
func parseResourceVersion(rv string) int64 {
	parsed, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
