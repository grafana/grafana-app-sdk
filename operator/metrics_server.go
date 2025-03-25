package operator

// since operator is a serverless component on its own, its associated server
// is only used for hosting the /metrics, /livez and /readyz endpoints

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/grafana/grafana-app-sdk/health"
)

// MetricsServer exports metrics as well as health checks under the same mux
type MetricsServer struct {
	mux  *http.ServeMux
	Port int

	observer *health.Observer
}

type MetricsServerConfig struct {
	// Server port for metrics and health endpoints
	Port int

	// HealthCheckInterval is the duration at which the server will periodically run the registered health checks
	HealthCheckInterval time.Duration
}

func NewMetricsServer(config MetricsServerConfig) *MetricsServer {
	if config.Port <= 0 {
		config.Port = 9090
	}

	return &MetricsServer{
		Port:     config.Port,
		mux:      http.NewServeMux(),
		observer: health.NewObserver(config.HealthCheckInterval),
	}
}

func (s *MetricsServer) RegisterHealthChecks(checks ...health.Check) {
	s.observer.AddChecks(checks...)
}

func (s *MetricsServer) RegisterMetricsHandler(handler http.Handler) {
	s.mux.Handle("/metrics", handler)
}

func (s *MetricsServer) registerHealthHandlers() {
	s.mux.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	s.mux.Handle("/readyz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		status := s.observer.Status()
		if !status.Successful {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("readiness check failed: " + status.String()))
			return
		}
		_, _ = w.Write([]byte(status.String()))
	}))
}

func (s *MetricsServer) Run(ctx context.Context) error {
	// Run creates an HTTP server which exposes
	// 1. if enabled, a /metrics endpoint on the configured port
	// 2. health endpoints for liveness and readiness
	// (if port <=0, uses the default 9090)
	s.registerHealthHandlers()
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.Port),
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	observerCtx, cancelObserver := context.WithCancel(ctx)
	defer cancelObserver()

	wg := &sync.WaitGroup{}
	// channel size is 2 since there is a possibility of 2 error writes if we can't read from it for some reason
	processErr := make(chan error, 2)

	wg.Add(1)
	go func() {
		// is it worth it to return the last check result when this server is shutting down? I don't think so,
		// the error here should pertain to the metrics server having encountered an error in its own Run, related to its own aggregation logic
		// currently, the aggregation logic is pretty barebones and doesn't create its own errors
		if err := s.observer.Run(observerCtx); err != nil {
			processErr <- err
		}
		_ = server.Shutdown(ctx)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		processErr <- server.ListenAndServe()
		cancelObserver()
		wg.Done()
	}()

	var returnedErr error
	select {
	case err := <-processErr:
		returnedErr = err
	case <-ctx.Done():
		_ = server.Shutdown(ctx)
	}

	wg.Wait()

	return returnedErr
}
