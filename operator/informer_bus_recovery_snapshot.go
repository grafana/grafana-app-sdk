package operator

import (
	"context"
	"fmt"
	"strconv"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// Snapshot returns the informer's last known identities. Missed notifications
// can leave this view behind storage until a recovery LIST succeeds.
func (b *BusInformerWithRecovery) Snapshot() []resource.Identifier {
	b.snapshotMu.RLock()
	defer b.snapshotMu.RUnlock()
	keys := make([]resource.Identifier, 0, len(b.items))
	for key := range b.items {
		keys = append(keys, key)
	}
	return keys
}

func (b *BusInformerWithRecovery) applyLive(ctx context.Context, event resourcebus.Event) {
	if !b.acceptEvent(event) {
		return
	}
	id := resource.Identifier{Namespace: event.Namespace, Name: event.Name}
	old := b.items[id]
	version := numericVersion(event.ResourceVersion)
	if version == 0 {
		b.reportError(ctx, fmt.Errorf("bus recovery notification for %v has an invalid resource version %q", id, event.ResourceVersion))
		return
	}
	if version <= b.listVersion || version <= b.deleted[id] ||
		(old != nil && version <= numericVersion(old.GetResourceVersion())) {
		return
	}

	switch event.Verb {
	case resourcebus.VerbAdded, resourcebus.VerbModified:
		obj := newIdentityObject(b.kind, id.Namespace, id.Name, event.ResourceVersion)
		b.snapshotMu.Lock()
		b.items[id] = obj
		b.snapshotMu.Unlock()
		delete(b.deleted, id)
		if event.Verb == resourcebus.VerbAdded {
			// A recreate can reach this group member even if its delete did not.
			old = nil
		}
		b.deliverChange(ctx, old, obj)
	case resourcebus.VerbDeleted:
		b.snapshotMu.Lock()
		delete(b.items, id)
		b.snapshotMu.Unlock()
		b.deleted[id] = version
		if old == nil {
			old = newIdentityObject(b.kind, id.Namespace, id.Name, event.ResourceVersion)
		}
		b.deliverChange(ctx, old, nil)
	default:
		b.reportError(ctx, fmt.Errorf("unknown bus verb %q", event.Verb))
	}
}

func (b *BusInformerWithRecovery) relist(ctx context.Context, trigger busRelistTrigger) error {
	next, version, err := b.readSnapshot(ctx)
	if err != nil {
		return err
	}
	b.applySnapshot(ctx, trigger, next, version)
	return ctx.Err()
}

func (b *BusInformerWithRecovery) readSnapshot(ctx context.Context) (map[resource.Identifier]resource.Object, int64, error) {
	items, rv, err := b.lister.List(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("bus recovery LIST: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	version := numericVersion(rv)
	if version == 0 && (rv != "0" || len(items) != 0) {
		return nil, 0, fmt.Errorf("bus recovery LIST requires a numeric snapshot resource version, got %q", rv)
	}
	if version < b.listVersion {
		return nil, 0, fmt.Errorf("bus recovery LIST version %q predates installed snapshot %d", rv, b.listVersion)
	}
	next := make(map[resource.Identifier]resource.Object, len(items))
	for _, item := range items {
		if item == nil || item.GetName() == "" {
			return nil, 0, fmt.Errorf("bus recovery LIST at %q contains an item without identity", rv)
		}
		id := resource.Identifier{Namespace: item.GetNamespace(), Name: item.GetName()}
		if numericVersion(item.GetResourceVersion()) == 0 {
			return nil, 0, fmt.Errorf("bus recovery LIST at %q has an invalid item resource version %q for %v", rv, item.GetResourceVersion(), id)
		}
		if _, duplicate := next[id]; duplicate {
			return nil, 0, fmt.Errorf("bus recovery LIST at %q contains duplicate key %v", rv, id)
		}
		next[id] = newIdentityObject(b.kind, id.Namespace, id.Name, item.GetResourceVersion())
	}
	return next, version, nil
}

func (b *BusInformerWithRecovery) applySnapshot(ctx context.Context, trigger busRelistTrigger, next map[resource.Identifier]resource.Object, version int64) {
	changes := make([]busDelivery, 0, len(next))
	for id, obj := range next {
		old := b.items[id]
		// Grafana's first LIST page can see item writes committed after it
		// reads the collection version. Use the item's own version when present.
		itemVersion := numericVersion(obj.GetResourceVersion())
		if b.deleted[id] >= itemVersion {
			delete(next, id)
			continue
		}
		if old != nil {
			oldVersion := numericVersion(old.GetResourceVersion())
			if oldVersion > itemVersion {
				next[id] = old
				continue
			}
			if trigger != busRelistScheduled && oldVersion == itemVersion {
				continue
			}
		}
		changes = append(changes, busDelivery{old: old, current: obj})
	}
	for id, old := range b.items {
		if next[id] != nil {
			continue
		}
		if numericVersion(old.GetResourceVersion()) > version {
			next[id] = old
			continue
		}
		changes = append(changes, busDelivery{old: old})
	}
	for id, deleted := range b.deleted {
		if deleted <= version {
			delete(b.deleted, id)
		}
	}
	b.snapshotMu.Lock()
	b.items, b.listVersion = next, version
	b.snapshotMu.Unlock()
	for _, change := range changes {
		b.deliverChange(ctx, change.old, change.current)
	}
}

func numericVersion(version string) int64 {
	n, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		return 0
	}
	return max(0, n)
}
