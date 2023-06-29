package operator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
)

type ResourceAction string

const (
	ResourceActionCreate = ResourceAction("CREATE")
	ResourceActionUpdate = ResourceAction("UPDATE")
	ResourceActionDelete = ResourceAction("DELETE")
)

// ErrInformerAlreadyAdded indicates that there is already an informer for the resource kind mapped
var ErrInformerAlreadyAdded = errors.New("informer for resource kind already added")

// DefaultRetryPolicy is an Exponential Backoff RetryPolicy with an initial 5-second delay and a max of 5 attempts
var DefaultRetryPolicy = ExponentialBackoffRetryPolicy(5*time.Second, 5)

// Informer is an interface describing an informer which can be managed by InformerController
type Informer interface {
	AddEventHandler(handler ResourceWatcher) error
	Run(stopCh <-chan struct{}) error
}

// ResourceWatcher describes an object which handles Add/Update/Delete actions for a resource
type ResourceWatcher interface {
	Add(context.Context, resource.Object) error
	Update(ctx context.Context, old, new resource.Object) error
	Delete(context.Context, resource.Object) error
}

// RetryPolicy is a function that defines whether an event should be retried, based on the error and number of attempts.
// It returns a boolean indicating whether another attempt should be made, and a time.Duration after which that attempt should be made again.
type RetryPolicy func(err error, attempt int) (bool, time.Duration)

// ExponentialBackoffRetryPolicy returns an Exponential Backoff RetryPolicy function, which follows the following formula:
// retry time = initialDelay * (2^attempt).
// If maxAttempts is exceeded, it will return false for the retry.
func ExponentialBackoffRetryPolicy(initialDelay time.Duration, maxAttempts int) RetryPolicy {
	return func(err error, attempt int) (bool, time.Duration) {
		if attempt > maxAttempts {
			return false, 0
		}

		return true, initialDelay * time.Duration(math.Pow(2, float64(attempt)))
	}
}

// RetryDequeuePolicy is a function that defines when a retry should be dequeued when a new action is taken on a resource.
// It accepts information about the new action being taken, and information about the current queued retry,
// and returns `true` if the retry should be dequeued.
// A RetryDequeuePolicy may be called multiple times for the same action, depending on the number of pending retries for the object.
type RetryDequeuePolicy func(newAction ResourceAction, newObject resource.Object, retryAction ResourceAction, retryObject resource.Object, retryError error) bool

// OpinionatedRetryDequeuePolicy is a RetryDequeuePolicy which has the following logic:
// 1. If the newAction is a delete, dequeue the retry
// 2. If the newAction and retryAction are different, keep the retry (for example, a queued create retry, and a received update action)
// 3. If the generation of newObject and retryObject is the same, keep the retry
// 4. Otherwise, dequeue the retry
var OpinionatedRetryDequeuePolicy = func(newAction ResourceAction, newObject resource.Object, retryAction ResourceAction, retryObject resource.Object, retryError error) bool {
	if newAction == ResourceActionDelete {
		return true
	}
	if newAction != retryAction {
		return false
	}
	if getGeneration(newObject) != getGeneration(retryObject) {
		return false
	}
	return true
}

// InformerController is an object that handles coordinating informers and observers.
// Unlike adding a Watcher directly to an Informer with AddEventHandler, the InformerController
// guarantees sequential execution of watchers, based on add order.
type InformerController struct {
	// ErrorHandler is a user-specified error handling function. This is typically for logging/metrics use,
	// as retry logic is covered by the RetryPolicy.
	ErrorHandler func(error)
	// RetryPolicy is a user-specified retry logic function which will be used when ResourceWatcher function calls fail.
	RetryPolicy RetryPolicy
	// RetryDequeuePolicy is a user-specified retry dequeue logic function which will be used for new informer actions
	// when one or more retries for the object are still pending. If not present, existing retries are always dequeued.
	RetryDequeuePolicy  RetryDequeuePolicy
	informers           *ListMap[string, Informer]
	watchers            *ListMap[string, ResourceWatcher]
	toRetry             *ListMap[string, retryInfo]
	retryTickerInterval time.Duration
}

type retryInfo struct {
	retryAfter time.Time
	retryFunc  func() error
	attempt    int
	action     ResourceAction
	object     resource.Object
	err        error
}

// NewInformerController creates a new controller
func NewInformerController() *InformerController {
	return &InformerController{
		RetryPolicy:         DefaultRetryPolicy,
		informers:           NewListMap[Informer](),
		watchers:            NewListMap[ResourceWatcher](),
		toRetry:             NewListMap[retryInfo](),
		retryTickerInterval: time.Second,
	}
}

// AddInformer adds an informer for a specific resourceKind.
// The `resourceKind` string is used for internal tracking and correlation to observers,
// and does not necessarily need to match the informer's type.
//
// Multiple informers may be added for the same resource kind,
// and each will trigger all watchers for that resource kind.
// The most common usage of this is to have informers partitioned by namespace or labels for the same resource kind,
// which share a watcher.
//
//nolint:gocognit,funlen,dupl
func (c *InformerController) AddInformer(informer Informer, resourceKind string) error {
	if informer == nil {
		return fmt.Errorf("informer cannot be nil")
	}
	if resourceKind == "" {
		return fmt.Errorf("resourceKind cannot be empty")
	}

	err := informer.AddEventHandler(&SimpleWatcher{
		AddFunc: func(ctx context.Context, obj resource.Object) error {
			c.watchers.Range(resourceKind, func(idx int, watcher ResourceWatcher) {
				// Generate the unique key for this object
				retryKey := c.keyForWatcherEvent(resourceKind, idx, obj)

				// Dequeue retries according to the RetryDequeuePolicy
				if c.RetryDequeuePolicy != nil {
					c.toRetry.RemoveItems(retryKey, func(info retryInfo) bool {
						return c.RetryDequeuePolicy(ResourceActionCreate, obj, info.action, info.object, info.err)
					}, -1)
				} else {
					// If no RetryDequeuePolicy exists, dequeue all retries for the object
					c.toRetry.RemoveKey(retryKey)
				}

				// Do the watcher's Add, check for error
				err := watcher.Add(ctx, obj)
				if err != nil && c.ErrorHandler != nil {
					c.ErrorHandler(err) // TODO: improve ErrorHandler
				}
				if err != nil && c.RetryPolicy != nil {
					// Grab the exact watcher (rather than the range pointer) to use in the closure
					closureWatcher, ok := c.watchers.ItemAt(resourceKind, idx)
					if !ok {
						// What?
						return
					}
					c.queueRetry(retryKey, err, func() error {
						return closureWatcher.Add(ctx, obj)
					}, ResourceActionCreate, obj)
				}
			})
			return nil
		},
		UpdateFunc: func(ctx context.Context, oldObj, newObj resource.Object) error {
			c.watchers.Range(resourceKind, func(idx int, watcher ResourceWatcher) {
				// Generate the unique key for this object
				retryKey := c.keyForWatcherEvent(resourceKind, idx, newObj)

				// Dequeue retries according to the RetryDequeuePolicy
				if c.RetryDequeuePolicy != nil {
					c.toRetry.RemoveItems(retryKey, func(info retryInfo) bool {
						return c.RetryDequeuePolicy(ResourceActionUpdate, newObj, info.action, info.object, info.err)
					}, -1)
				} else {
					// If no RetryDequeuePolicy exists, dequeue all retries for the object
					c.toRetry.RemoveKey(retryKey)
				}

				// Do the watcher's Update, check for error
				err := watcher.Update(ctx, oldObj, newObj)
				if err != nil && c.ErrorHandler != nil {
					c.ErrorHandler(err)
				}
				if err != nil && c.RetryPolicy != nil {
					// Grab the exact watcher (rather than the range pointer) to use in the closure
					closureWatcher, ok := c.watchers.ItemAt(resourceKind, idx)
					if !ok {
						// What?
						return
					}
					c.queueRetry(retryKey, err, func() error {
						return closureWatcher.Update(ctx, oldObj, newObj)
					}, ResourceActionUpdate, newObj)
				}
			})
			return nil
		},
		DeleteFunc: func(ctx context.Context, obj resource.Object) error {
			c.watchers.Range(resourceKind, func(idx int, watcher ResourceWatcher) {
				// Generate the unique key for this object
				retryKey := c.keyForWatcherEvent(resourceKind, idx, obj)

				// Dequeue retries according to the RetryDequeuePolicy
				if c.RetryDequeuePolicy != nil {
					c.toRetry.RemoveItems(retryKey, func(info retryInfo) bool {
						return c.RetryDequeuePolicy(ResourceActionDelete, obj, info.action, info.object, info.err)
					}, -1)
				} else {
					// If no RetryDequeuePolicy exists, dequeue all retries for the object
					c.toRetry.RemoveKey(retryKey)
				}

				// Do the watcher's Delete, check for error
				err := watcher.Delete(ctx, obj)
				if err != nil && c.ErrorHandler != nil {
					c.ErrorHandler(err)
				}
				if err != nil && c.RetryPolicy != nil {
					// Grab the exact watcher (rather than the range pointer) to use in the closure
					closureWatcher, ok := c.watchers.ItemAt(resourceKind, idx)
					if !ok {
						// What?
						return
					}
					c.queueRetry(retryKey, err, func() error {
						return closureWatcher.Delete(ctx, obj)
					}, ResourceActionDelete, obj)
				}
			})
			return nil
		},
	})
	if err != nil {
		return err
	}

	c.informers.AddItem(resourceKind, informer)
	return nil
}

// AddWatcher adds an observer to an informer with a matching `resourceKind`.
// Any time the informer sees an add, update, or delete, it will call the observer's corresponding method.
// Multiple watchers can exist for the same resource kind.
// They will be run in the order they were added to the informer.
func (c *InformerController) AddWatcher(watcher ResourceWatcher, resourceKind string) error {
	if watcher == nil {
		return fmt.Errorf("watcher cannot be nil")
	}
	if resourceKind == "" {
		return fmt.Errorf("resourceKind cannot be empty")
	}
	c.watchers.AddItem(resourceKind, watcher)
	return nil
}

// RemoveWatcher removes the given ResourceWatcher from the list for the resourceKind, provided it exists in the list.
func (c *InformerController) RemoveWatcher(watcher ResourceWatcher, resourceKind string) {
	c.watchers.RemoveItem(resourceKind, func(w ResourceWatcher) bool {
		return watcher == w
	})
}

// RemoveAllWatchersForResource removes all watchers for a specific resourceKind
func (c *InformerController) RemoveAllWatchersForResource(resourceKind string) {
	c.watchers.RemoveKey(resourceKind)
}

// Run runs the controller, which starts all informers, until stopCh is closed
//
//nolint:errcheck
func (c *InformerController) Run(stopCh <-chan struct{}) error {
	c.informers.RangeAll(func(_ string, _ int, inf Informer) {
		go inf.Run(stopCh)
	})

	go c.retryTicker(stopCh)

	<-stopCh

	return nil
}

// retryTicker blocks until stopCh is closed or receives a message.
// It checks if there are function calls to be retried every second, and, if there are any, calls the function.
// If the function returns an error, it schedules a new retry according to the RetryPolicy.
func (c *InformerController) retryTicker(stopCh <-chan struct{}) {
	ticker := time.NewTicker(c.retryTickerInterval)
	defer ticker.Stop()
	for {
		select {
		case t := <-ticker.C:
			for _, key := range c.toRetry.Keys() {
				// To be simple, we retry all retries which should be done now, and remove them from the list
				// We then add back in retries which failed and need to be retried again
				toAdd := make([]retryInfo, 0)
				c.toRetry.RemoveItems(key, func(val retryInfo) bool {
					if t.After(val.retryAfter) {
						err := val.retryFunc()
						if err != nil && c.RetryPolicy != nil {
							ok, after := c.RetryPolicy(err, val.attempt+1)
							if ok {
								toAdd = append(toAdd, retryInfo{
									attempt:    val.attempt + 1,
									retryAfter: t.Add(after),
									retryFunc:  val.retryFunc,
									action:     val.action,
									object:     val.object,
								})
							}
						}
						return true
					}
					return false
				}, -1)
				for _, inf := range toAdd {
					c.toRetry.AddItem(key, inf)
				}
			}
		case <-stopCh:
			return
		}
	}
}

func (*InformerController) keyForWatcherEvent(resourceKind string, watcherIndex int, obj resource.Object) string {
	if obj == nil {
		return fmt.Sprintf("%s:%d:nil:nil", resourceKind, watcherIndex)
	}
	return fmt.Sprintf("%s:%d:%s:%s", resourceKind, watcherIndex, obj.StaticMetadata().Namespace, obj.StaticMetadata().Name)
}

func (c *InformerController) queueRetry(key string, err error, toRetry func() error, action ResourceAction, obj resource.Object) {
	if c.RetryPolicy == nil {
		return
	}

	if ok, after := c.RetryPolicy(err, 0); ok {
		c.toRetry.AddItem(key, retryInfo{
			retryAfter: time.Now().Add(after),
			retryFunc:  toRetry,
			action:     action,
			object:     obj,
			err:        err,
		})
	}
}
