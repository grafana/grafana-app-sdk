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
	// HealthChecks runs all registered health checks and returns any errors combined into a single error
	HealthChecks() []Check
}
