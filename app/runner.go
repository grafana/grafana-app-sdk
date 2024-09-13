package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var RunnableCollectorDefaultErrorHandler = func(ctx context.Context, err error) bool {
	logging.FromContext(ctx).Error("runner exited with error", "error", err)
	return true
}

// NewMultiRunner creates a new MultiRunner with Runners as an empty slice and ErrorHandler set to RunnableCollectorDefaultErrorHandler
func NewMultiRunner() *MultiRunner {
	return &MultiRunner{
		Runners:      make([]Runnable, 0),
		ErrorHandler: RunnableCollectorDefaultErrorHandler,
	}
}

// MultiRunner implements Runnable for running multiple Runnable instances.
type MultiRunner struct {
	Runners []Runnable
	// ErrorHandler is called if one of the Runners returns an error. If the function call returns true,
	// the context will be canceled and all other Runners will also be prompted to exit.
	// If ErrorHandler is nil, RunnableCollectorDefaultErrorHandler is used.
	ErrorHandler func(context.Context, error) bool
	// ExitWait is how long to wait for Runners to exit after ErrorHandler returns true or the context is canceled
	// before stopping execution and returning a timeout error instead of exiting gracefully.
	// If ExitWait is nil, Run execution will always block until all Runners have exited.
	ExitWait *time.Duration
}

// Run runs all Runners in separate goroutines, and calls ErrorHandler if any of them exits early with an error.
// If ErrorHandler returns true (or if there is no ErrorHandler), the other Runners are canceled and the error is returned.
func (m *MultiRunner) Run(ctx context.Context) error {
	propagatedContext, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, len(m.Runners))
	defer close(errs)
	wg := &sync.WaitGroup{}
	for _, runner := range m.Runners {
		wg.Add(1)
		go func(r Runnable) {
			err := r.Run(propagatedContext)
			wg.Done()
			if err != nil {
				errs <- err
			}
		}(runner)
	}
	for {
		select {
		case err := <-errs:
			handler := m.ErrorHandler
			if handler == nil {
				handler = RunnableCollectorDefaultErrorHandler
			}
			if handler(propagatedContext, err) {
				cancel()
				if m.ExitWait != nil {
					if waitOrTimeout(wg, *m.ExitWait) {
						return fmt.Errorf("exit wait time exceeded waiting for Runners to complete: %w", err)
					}
				} else {
					wg.Wait() // Wait for all the runners to stop
				}
				return err
			}
		case <-ctx.Done():
			cancel()
			if m.ExitWait != nil {
				if waitOrTimeout(wg, *m.ExitWait) {
					return fmt.Errorf("exit wait time exceeded waiting for Runners to complete")
				}
			} else {
				wg.Wait() // Wait for all the runners to stop
			}
			return nil
		}
	}
}

// PrometheusCollectors implements metrics.Provider by returning prometheus collectors for all Runners that also
// implement metrics.Provider.
func (m *MultiRunner) PrometheusCollectors() []prometheus.Collector {
	collectors := make([]prometheus.Collector, 0)
	for _, runner := range m.Runners {
		if cast, ok := runner.(metrics.Provider); ok {
			collectors = append(collectors, cast.PrometheusCollectors()...)
		}
	}
	return collectors
}

// AddRunnable adds the provided Runnable to the Runners slice. If the slice is nil, it will create it.
func (m *MultiRunner) AddRunnable(runnable Runnable) {
	if m.Runners == nil {
		m.Runners = make([]Runnable, 0)
	}
	m.Runners = append(m.Runners, runnable)
}

func waitOrTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		wg.Wait()
	}()
	select {
	case <-ch:
		return false
	case <-time.After(timeout):
		return true
	}
}
