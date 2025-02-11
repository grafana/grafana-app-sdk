package health

import (
	"context"
)

type Check interface {
	HealthCheck(context.Context) error
	HealthCheckName() string
}

type Checker interface {
	// HealthChecks runs all registered health checks and returns any errors combined into a single error
	HealthChecks() []Check
}
