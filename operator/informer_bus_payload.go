package operator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

const defaultHydrationTimeout = 30 * time.Second

func (b *BusInformerWithRecovery) liveObject(ctx context.Context, event resourcebus.Event, id resource.Identifier) (resource.Object, error) {
	if b.getObject == nil {
		return b.kind.NewIdentityObject(id, event.ResourceVersion), nil
	}
	timeout := b.timeout
	if timeout <= 0 {
		timeout = defaultHydrationTimeout
	}
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	obj, err := b.getObject(readCtx, id)
	if err != nil {
		return nil, err
	}
	if obj == nil || obj.GetStaticMetadata().Identifier() != id {
		return nil, errors.New("object response does not match the notification identity")
	}
	observed, announced := numericVersion(obj.GetResourceVersion()), numericVersion(event.ResourceVersion)
	if observed == 0 {
		return nil, fmt.Errorf("object response has an invalid resource version %q", obj.GetResourceVersion())
	}
	if observed < announced {
		return nil, fmt.Errorf("object version %d predates announced version %d", observed, announced)
	}
	return b.snapshotObject(obj), nil
}

func (b *BusInformerWithRecovery) snapshotObject(obj resource.Object) resource.Object {
	if b.getObject != nil {
		owned := obj.Copy()
		gvk := obj.GroupVersionKind()
		if filled := withFallbackGVK(gvk, b.kind.GroupVersionKind()); filled != gvk {
			owned.SetGroupVersionKind(filled)
		}
		return owned
	}
	return b.kind.NewIdentityObject(obj.GetStaticMetadata().Identifier(), obj.GetResourceVersion())
}

func withFallbackGVK(gvk, fallback schema.GroupVersionKind) schema.GroupVersionKind {
	if gvk.Group == "" {
		gvk.Group = fallback.Group
	}
	if gvk.Version == "" {
		gvk.Version = fallback.Version
	}
	if gvk.Kind == "" {
		gvk.Kind = fallback.Kind
	}
	return gvk
}
