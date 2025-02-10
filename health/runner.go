package health

import (
	"context"
	"time"
)

type HealthCheckRunner struct {
	HealthChecker HealthCheck
	Interval      time.Duration
}

// this is the runner to run the health check periodically
func (h *HealthCheckRunner) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(h.Interval):
			h.HealthChecker.RunHealthCheck(ctx)
		}
	}
}
