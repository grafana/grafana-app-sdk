package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type CheckResult struct {
	Name  string
	Error error
}

func (c CheckResult) String() string {
	status := "ok"
	if c.Error != nil {
		status = c.Error.Error()
	}
	return fmt.Sprintf("%s: %s", c.Name, status)
}

type CheckStatus struct {
	Successful bool
	Results    []CheckResult
}

func (c CheckStatus) String() string {
	var status string
	for i, result := range c.Results {
		if i == 0 {
			status = result.String()
		} else {
			status = fmt.Sprintf(status, fmt.Sprintf("%s\n%s", status, result.String()))
		}
	}
	return status
}

type Observer struct {
	checks      []Check
	checkLock   sync.RWMutex
	runInterval time.Duration
	runLock     sync.RWMutex
	runStatus   *CheckStatus
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

func (c *Observer) Status() CheckStatus {
	c.runLock.RLock()
	defer c.runLock.RUnlock()

	return *c.runStatus
}

func (c *Observer) Run(ctx context.Context) {
	t := time.NewTicker(c.runInterval)
	defer t.Stop()

	// collect initial checks before we start listening to the ticker
	initialStatus := c.collectChecks(ctx)
	c.runLock.Lock()
	c.runStatus = initialStatus
	c.runLock.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			lastStatus := c.collectChecks(ctx)

			c.runLock.Lock()
			c.runStatus = lastStatus
			c.runLock.Unlock()
		}
	}
}

func (c *Observer) collectChecks(ctx context.Context) *CheckStatus {
	c.checkLock.RLock()
	defer c.checkLock.RUnlock()

	runStatus := &CheckStatus{
		Successful: false,
		Results:    []CheckResult{},
	}

	for _, check := range c.checks {
		err := check.HealthCheck(ctx)
		runStatus.Results = append(c.runStatus.Results, CheckResult{
			Name:  check.HealthCheckName(),
			Error: err,
		})
	}
	return runStatus
}
