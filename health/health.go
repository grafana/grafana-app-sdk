package health

import (
	"context"
	"net/http"
)

type HealthCheck interface {
	RegisterHealthCheck(fn HealthCheckFunc)
	HTTPHandler() http.HandlerFunc
}

type HealthCheckFunc = func(context.Context) error

type HealthChecker struct {
	fns []HealthCheckFunc
}

var _ HealthCheck = (*HealthChecker)(nil)

// RegisterHealthCheck adds a HealthCheck to be run by the HealthChecker
func (c *HealthChecker) RegisterHealthCheck(fn HealthCheckFunc) {
	c.fns = append(c.fns, fn)
}

// RunHealthCheck runs all registered health checks and returns any errors combined into a single error
func (c *HealthChecker) runHealthChecks(ctx context.Context) error {
	for _, fn := range c.fns {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return nil
}

// HTTPHandler returns an http.Handler that performs the health checks
func (c *HealthChecker) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c.runHealthChecks(r.Context()) != nil {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(500)
		_, _ = w.Write([]byte("error: not running"))
		return
	}
}
