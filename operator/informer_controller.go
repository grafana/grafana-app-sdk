package operator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/puzpuzpuz/xsync/v2"

	"github.com/grafana/grafana-app-sdk/resource"
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

// InformerController is an object that handles coordinating informers and observers.
// Unlike adding a Watcher directly to an Informer with AddEventHandler, the InformerController
// guarantees sequential execution of watchers, based on add order.
type InformerController struct {
	// ErrorHandler is a user-specified error handling function. This is typically for logging/metrics use,
	// as retry logic is covered by the RetryPolicy.
	ErrorHandler func(error)
	// RetryPolicy is a user-specified retry logic function which will be used when ResourceWatcher function calls fail.
	RetryPolicy RetryPolicy
	informers   *ListMap[string, Informer]
	watchers    *ListMap[string, ResourceWatcher]
	toRetry     *xsync.MapOf[string, retryInfo]
}

type retryInfo struct {
	retryAfter time.Time
	retryFunc  func() error
	attempt    int
}

// NewInformerController creates a new controller
func NewInformerController() *InformerController {
	return &InformerController{
		RetryPolicy: DefaultRetryPolicy,
		informers:   NewListMap[Informer](),
		watchers:    NewListMap[ResourceWatcher](),
		toRetry:     xsync.NewMapOf[retryInfo](),
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
//nolint:gocognit
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
					c.queueRetry(c.keyForWatcherEvent(resourceKind, idx, obj), err, func() error {
						return closureWatcher.Add(ctx, obj)
					})
				}
			})
			return nil
		},
		UpdateFunc: func(ctx context.Context, oldObj, newObj resource.Object) error {
			c.watchers.Range(resourceKind, func(idx int, watcher ResourceWatcher) {
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
					c.queueRetry(c.keyForWatcherEvent(resourceKind, idx, newObj), err, func() error {
						return closureWatcher.Update(ctx, oldObj, newObj)
					})
				}
			})
			return nil
		},
		DeleteFunc: func(ctx context.Context, obj resource.Object) error {
			c.watchers.Range(resourceKind, func(idx int, watcher ResourceWatcher) {
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
					c.queueRetry(c.keyForWatcherEvent(resourceKind, idx, obj), err, func() error {
						return closureWatcher.Delete(ctx, obj)
					})
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
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case t := <-ticker.C:
			c.toRetry.Range(func(key string, val retryInfo) bool {
				if t.After(val.retryAfter) {
					err := val.retryFunc()
					if err != nil {
						if c.RetryPolicy == nil {
							// RetryPolicy was removed for some reason
							c.toRetry.Delete(key)
							return true
						}
						ok, after := c.RetryPolicy(err, val.attempt+1)
						if !ok {
							// Don't retry anymore
							c.toRetry.Delete(key)
							return true
						}

						c.toRetry.Store(key, retryInfo{
							attempt:    val.attempt + 1,
							retryAfter: time.Now().Add(after),
							retryFunc:  val.retryFunc,
						})
					}
				}
				return true
			})
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

func (c *InformerController) queueRetry(key string, err error, toRetry func() error) {
	if c.RetryPolicy == nil {
		return
	}

	if ok, after := c.RetryPolicy(err, 0); ok {
		c.toRetry.Store(key, retryInfo{
			retryAfter: time.Now().Add(after),
			retryFunc:  toRetry,
		})
	} else {
		c.toRetry.Delete(key)
	}
}
