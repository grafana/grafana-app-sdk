// The code in this file is copied from k8s.io/client-go/tools/cache/controller.go.
// It contains minor modifications to the original code.

/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cache

import (
	"context"
	"sync"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/clock"
)

var _ cache.Controller = &Controller{}

// Controller is a controller that uses a reflector to watch a resource and update a queue.
// It is copied over from k8s.io/client-go/tools/cache/controller.go.
type Controller struct {
	config         cache.Config
	clock          clock.Clock
	reflector      *Reflector
	reflectorMutex sync.RWMutex
}

// NewController makes a new Controller from the given Config.
func NewController(c *cache.Config) *Controller {
	ctlr := &Controller{
		config: *c,
		clock:  &clock.RealClock{},
	}
	return ctlr
}

// Run implements [Controller.Run].
func (c *Controller) Run(stopCh <-chan struct{}) {
	c.RunWithContext(wait.ContextForChannel(stopCh))
}

// RunWithContext implements [Controller.RunWithContext].
func (c *Controller) RunWithContext(ctx context.Context) {
	defer utilruntime.HandleCrashWithContext(ctx)

	go func() {
		<-ctx.Done()
		c.config.Queue.Close()
	}()

	r := NewReflectorWithOptions(
		cache.ToListerWatcherWithContext(c.config.ListerWatcher),
		c.config.ObjectType,
		c.config.Queue,
		ReflectorOptions{
			ResyncPeriod:    c.config.FullResyncPeriod,
			MinWatchTimeout: c.config.MinWatchTimeout,
			TypeDescription: c.config.ObjectDescription,
			Clock:           c.clock,
		},
	)
	r.ShouldResync = c.config.ShouldResync
	r.WatchListPageSize = c.config.WatchListPageSize

	c.reflectorMutex.Lock()
	c.reflector = r
	c.reflectorMutex.Unlock()

	var wg wait.Group

	wg.StartWithContext(ctx, r.RunWithContext)

	wait.UntilWithContext(ctx, c.processLoop, time.Second)
	wg.Wait()
}

// HasSynced returns true once this cache has completed an initial resource listing.
func (c *Controller) HasSynced() bool {
	return c.config.Queue.HasSynced()
}

// LastSyncResourceVersion returns the last sync resource version of the cache.
func (c *Controller) LastSyncResourceVersion() string {
	c.reflectorMutex.RLock()
	defer c.reflectorMutex.RUnlock()
	if c.reflector == nil {
		return ""
	}
	return c.reflector.LastSyncResourceVersion()
}

// processLoop drains the work queue.
func (c *Controller) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// If the context is canceled, we have to exit the loop, even without draining the queue.
			// So we close the queue and return.
			c.config.Queue.Close()
			return
		default:
			_, err := c.config.Queue.Pop(cache.PopProcessFunc(c.config.Process))
			if err != nil {
				if err == cache.ErrFIFOClosed {
					return
				}
			}
		}
	}
}
