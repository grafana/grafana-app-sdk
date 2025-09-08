package resource

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type UpdateClient[T Object] interface {
	Get(context.Context, Identifier) (T, error)
	Update(context.Context, T, UpdateOptions) (T, error)
}

// Updater is a type used for updating objects
type Updater[T Object] struct {
	client UpdateClient[T]
}

func NewUpdater[T Object](client UpdateClient[T]) *Updater[T] {
	return &Updater[T]{
		client: client,
	}
}

func (u *Updater[T]) Update(ctx context.Context, identifier Identifier, updateFunc func(*T) error, opts UpdateOptions) (T, error) {
	var empty T
	obj, err := u.client.Get(ctx, identifier)
	if err != nil {
		return empty, err
	}

	doUpdate := func(obj T) (T, error) {
		err = updateFunc(&obj)
		if err != nil {
			return empty, err
		}
		if obj.GetResourceVersion() == "" {
			return empty, ErrMissingResourceVersion
		}
		if obj.GetStaticMetadata().Identifier() != identifier {
			return empty, fmt.Errorf("resource identifier after updateFunc is run ('%s/%s') does not match provided resource identifier ('%s/%s')", obj.GetNamespace(), obj.GetName(), identifier.Namespace, identifier.Name)
		}
		return u.client.Update(ctx, obj, UpdateOptions{
			ResourceVersion: obj.GetResourceVersion(),
			Subresource:     opts.Subresource,
			DryRun:          opts.DryRun,
		})
	}
	obj, err = doUpdate(obj)
	retries := 0
	for err != nil && apierrors.IsConflict(err) && retries < 2 {
		retries++
		obj, err = doUpdate(obj)
	}
	return obj, err
}

// UpdateObject ensures an object is updated by calling Get on the provided client to get the current state of the object,
// then runs updateFunc on it to update it, and finally calls UpdateInto with the updated object and provided config,
// using the ResourceVersion of the updated object. If the update fails due to a 409/Conflict, it will retry the whole process up to two times.
func UpdateObject[T Object](ctx context.Context, client Client, identifier Identifier, updateFunc func(*T) error, opts UpdateOptions) (T, error) {
	var empty T
	rawObj, err := client.Get(ctx, identifier)
	if err != nil {
		return empty, err
	}
	obj, ok := rawObj.(T)
	if !ok {
		return empty, fmt.Errorf("unable to cast Object into provided type")
	}

	doUpdate := func(obj T) error {
		err = updateFunc(&obj)
		if err != nil {
			return err
		}
		if obj.GetResourceVersion() == "" {
			return ErrMissingResourceVersion
		}
		return client.UpdateInto(ctx, identifier, obj, UpdateOptions{
			ResourceVersion: obj.GetResourceVersion(),
			Subresource:     opts.Subresource,
			DryRun:          opts.DryRun,
		}, obj)
	}
	err = doUpdate(obj)
	retries := 0
	for err != nil && apierrors.IsConflict(err) && retries < 2 {
		retries++
		err = doUpdate(obj)
	}
	return obj, err
}
