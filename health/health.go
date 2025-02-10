package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
)

type HealthCheck interface {
	// RegisterHealthCheck adds a HealthCheck to be run by the HealthChecker
	RegisterHealthCheck(fn HealthCheckFunc)
	// HTTPHandler returns an http.Handler that returns a cached value of the health checks as they ran last (they are run periodically and not on demand)
	HTTPHandler() http.HandlerFunc
	// RunHealthCheck runs all registered health checks and returns any errors combined into a single error
	// This must be run on an interval by the caller or else the result returned by HTTP Handler will always be uninitialized
	RunHealthCheck(context.Context) error
}

type HealthCheckFunc = func(context.Context) error

type HealthCheckResult struct {
	succeeded bool
	error     error
}

type HealthChecker struct {
	fns []HealthCheckFunc
	// access to result is guarded with resultMu
	result   HealthCheckResult
	resultMu sync.RWMutex
}

var _ HealthCheck = (*HealthChecker)(nil)

// RegisterHealthCheck adds a HealthCheck to be run by the HealthChecker
func (c *HealthChecker) RegisterHealthCheck(fn HealthCheckFunc) {
	c.fns = append(c.fns, fn)
}

// HTTPHandler returns an http.Handler that performs the health checks
func (c *HealthChecker) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.resultMu.RLock()
		defer c.resultMu.RUnlock()

		if c.result.error == nil {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(500)
		_, _ = w.Write([]byte(c.result.error.Error()))
		return
	}
}

func (c *HealthChecker) RunHealthCheck(ctx context.Context) error {
	c.resultMu.Lock()
	defer c.resultMu.Unlock()

	fmt.Println("Running health check...")

	var allErrors error
	for _, fn := range c.fns {
		if err := fn(ctx); err != nil {
			allErrors = errors.Join(allErrors, err)
		}
	}
	if allErrors != nil {
		c.result.error = allErrors
	} else {
		c.result.succeeded = true
	}
	return nil
}
