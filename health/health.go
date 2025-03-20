package health

import (
	"context"
)

// Check is an interface that describes anything that has a health check for it
type Check interface {
	HealthCheck(context.Context) error
	HealthCheckName() string
}

// Checker
type Checker interface {
	// HealthChecks returns a slice of all Check instances which should be run for health checks
	HealthChecks() []Check
}
