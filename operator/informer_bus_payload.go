package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/resourcebus"
)

// BusPayload configures what the bus delivers to handlers.
type BusPayload string

const (
	// PayloadIdentity guarantees only the resource's identity.
	PayloadIdentity BusPayload = "identity"
	// PayloadHydrated fetches full objects for live writes.
	PayloadHydrated BusPayload = "hydrated"
)

// BusObjectGetter reads the object named by a bus notification.
type BusObjectGetter interface {
	Get(context.Context, resource.Identifier) (resource.Object, error)
}

func validateBusPayload(opts BusRecoveryOptions) error {
	switch opts.Payload {
	case PayloadIdentity:
		return nil
	case PayloadHydrated:
	default:
		return fmt.Errorf("unknown bus payload %q", opts.Payload)
	}
	if opts.Getter == nil {
		return errors.New("bus payload requires an object getter")
	}
	return nil
}

func (b *BusInformerWithRecovery) liveObject(ctx context.Context, event resourcebus.Event, id resource.Identifier) (resource.Object, error) {
	opts := b.opts
	if opts.Payload == PayloadIdentity {
		return newIdentityObject(b.kind, id.Namespace, id.Name, event.ResourceVersion), nil
	}
	if b.timeout > 0 {
		readCtx, cancel := context.WithTimeout(ctx, b.timeout)
		defer cancel()
		ctx = readCtx
	}
	obj, err := opts.Getter.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if obj == nil || obj.GetName() != id.Name || obj.GetNamespace() != id.Namespace {
		return nil, errors.New("object response does not match the notification identity")
	}
	observed, announced := numericVersion(obj.GetResourceVersion()), numericVersion(event.ResourceVersion)
	if observed == 0 {
		return nil, fmt.Errorf("object response has an invalid resource version %q", obj.GetResourceVersion())
	}
	if observed < announced {
		return nil, fmt.Errorf("object version %d predates announced version %d", observed, announced)
	}
	return b.listObject(obj), nil
}

func (b *BusInformerWithRecovery) listObject(obj resource.Object) resource.Object {
	payload := b.opts.Payload
	if payload == PayloadHydrated {
		return obj.Copy()
	}
	return newIdentityObject(b.kind, obj.GetNamespace(), obj.GetName(), obj.GetResourceVersion())
}
