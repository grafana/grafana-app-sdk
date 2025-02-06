package operator

// since operator is a serverless component on its own, its associated server
// is only used for hosting the /metrics, /livez and /readyz endpoints

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/grafana/grafana-app-sdk/health"
)

type OperatorServer struct {
	Mux         *http.ServeMux
	HealthCheck health.HealthCheck
	Port        int
}

func (s *OperatorServer) RegisterMetricsHandler(handler http.Handler) {
	s.Mux.Handle("/metrics", handler)
}

func (s *OperatorServer) registerHealthHandlers() error {
	s.Mux.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
		return
	}))
	s.Mux.Handle("/readyz", s.HealthCheck.HTTPHandler())
	return nil
}

func (s *OperatorServer) Run(context.Context) error {
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
	return server.ListenAndServe()
}
