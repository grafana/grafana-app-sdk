package operator

// since operator is a serverless component on its own, its associated server
// is only used for hosting the /metrics, /livez and /readyz endpoints

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/grafana/grafana-app-sdk/health"
)

type OperatorServer struct {
	Checks              []health.Check
	Mux                 *http.ServeMux
	Port                int
	HealthCheckInterval time.Duration

	// access to healthCheckErr is guarded with healthCheckErrMu
	healthCheckErr   error
	healthCheckErrMu sync.RWMutex
}

func (s *OperatorServer) RegisterMetricsHandler(handler http.Handler) {
	s.Mux.Handle("/metrics", handler)
}

func (s *OperatorServer) registerHealthHandlers() error {
	s.Mux.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
		return
	}))
	s.Mux.Handle("/readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.cachedChecks(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("readiness check failed: " + err.Error()))
			return
		}
		_, _ = w.Write([]byte("ok"))
		return
	}))
	return nil
}

func (s *OperatorServer) Run(ctx context.Context) error {
	wg := &sync.WaitGroup{}
	// Run creates an HTTP server which exposes
	// 1. if enabled, a /metrics endpoint on the configured port
	// 2. health endpoints for liveness and readiness
	// (if port <=0, uses the default 9090)
	if err := s.registerHealthHandlers(); err != nil {
		return err
	}
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.Port),
		Handler:           s.Mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	var serverErr error
	wg.Add(1)
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErr = err
		}
		wg.Done()
	}()

	// hydrate the result on startup, so that we have something set before the periodic checks start happening
	if err := s.runChecks(ctx); err != nil {
		return err
	}
	wg.Add(1)
	go func() {
		for {
			select {
			case <-time.After(s.HealthCheckInterval):
				_ = s.runChecks(ctx)
			case <-ctx.Done():
				_ = server.Shutdown(ctx)
				wg.Done()
			}

		}
	}()

	wg.Wait()
	return serverErr
}

func (s *OperatorServer) cachedChecks() error {
	s.healthCheckErrMu.RLock()
	defer s.healthCheckErrMu.RUnlock()

	return s.healthCheckErr
}

func (s *OperatorServer) runChecks(ctx context.Context) error {
	s.healthCheckErrMu.Lock()
	defer s.healthCheckErrMu.Unlock()

	fmt.Println("Running health check...")

	var allErrors error
	for _, check := range s.Checks {
		if err := check.HealthCheck(ctx); err != nil {
			allErrors = errors.Join(allErrors, err)
		}
	}
	if allErrors != nil {
		s.healthCheckErr = allErrors
	}
	return nil
}
