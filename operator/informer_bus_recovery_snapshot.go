package operator

import (
	"context"
	"fmt"
	"strconv"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

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
		b.items[id] = obj
		delete(b.deleted, id)
		if event.Verb == resourcebus.VerbAdded {
			// Deliver an add, not an update: this member may have missed the delete of an earlier object.
			old = nil
		}
		b.deliverChange(ctx, old, obj)
	case resourcebus.VerbDeleted:
		delete(b.items, id)
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
	for id, obj := range next {
		old := b.items[id]
		// The first LIST page can include writes made after the snapshot version was read, so compare each object's
		// own version.
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
		b.deliverChange(ctx, old, obj)
	}
	for id, old := range b.items {
		if next[id] != nil {
			continue
		}
		if numericVersion(old.GetResourceVersion()) > version {
			next[id] = old
			continue
		}
		b.deliverChange(ctx, old, nil)
	}
	for id, deleted := range b.deleted {
		if deleted <= version {
			delete(b.deleted, id)
		}
	}
	b.items, b.listVersion = next, version
}

func numericVersion(version string) int64 {
	n, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		return 0
	}
	return max(0, n)
}
