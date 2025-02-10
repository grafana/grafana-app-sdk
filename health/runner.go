package health

import (
	"context"
	"time"
)

// this is the runner to run the health check periodically - combined with the HealthCheck interface, it allows
// the HTTP handler to return a cached health result refreshed on the interval it is constructed with
type HealthCheckRunner struct {
	healthChecker HealthCheck
	interval      time.Duration
}

func NewHealthCheckRunner(hc HealthCheck, interval time.Duration) *HealthCheckRunner {
	return &HealthCheckRunner{
		healthChecker: hc,
		interval:      interval,
	}
}

func (h *HealthCheckRunner) Run(ctx context.Context) error {
	if err := h.healthChecker.RunHealthCheck(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(h.interval):
			_ = h.healthChecker.RunHealthCheck(ctx)
		}
	}
}
