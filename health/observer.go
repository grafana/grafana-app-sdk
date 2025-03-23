package health

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Observer struct {
	checks               []Check
	checkLock            sync.RWMutex
	runInterval          time.Duration
	runLock              sync.RWMutex
	runErr               error
	initialChecksRunOnce sync.Once // is used to ensure collectInitialChecks is run exactly once by a consuming entity
}

func NewObserver(interval time.Duration) *Observer {
	return &Observer{
		checks:      []Check{},
		runInterval: interval,
	}
}

func (c *Observer) AddChecks(checks ...Check) {
	c.checkLock.Lock()
	c.checks = append(c.checks, checks...)
	c.checkLock.Unlock()
}

func (c *Observer) Status() error {
	c.runLock.RLock()
	defer c.runLock.RUnlock()

	return c.runErr
}

func (c *Observer) Run(ctx context.Context) error {
	t := time.NewTicker(c.runInterval)
	defer t.Stop()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return lastErr
		case <-t.C:
			lastErr = c.collectChecks(ctx)

			c.runLock.Lock()
			c.runErr = lastErr
			c.runLock.Unlock()
		}
	}
}

func (c *Observer) collectChecks(ctx context.Context) error {
	c.checkLock.RLock()
	defer c.checkLock.RUnlock()

	var errs error

	for _, check := range c.checks {
		if err := check.HealthCheck(ctx); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

func (c *Observer) CollectInitialChecks(ctx context.Context) error {
	var result error
	// is this right? am I overthinking it with sync.Once?
	c.initialChecksRunOnce.Do(func() {
		result = c.collectChecks(ctx)
	})
	return result
}
