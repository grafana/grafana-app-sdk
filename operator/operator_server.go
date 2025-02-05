package operator

// since operator is a serverless component on its own, its associated server
// is only used for hosting the /metrics, /livez and /readyz endpoints

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type OperatorServer struct {
	Mux  *http.ServeMux
	Port int
}

func (s *OperatorServer) RegisterLivenessHandler(handler func(http.ResponseWriter) error) error {
	if handler == nil {
		return errors.New("liveness handler cannot be nil")
	}
	s.Mux.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = handler(w)
	}))
	return nil
}

func (s *OperatorServer) RegisterReadinessHandler(handler func(http.ResponseWriter) error) error {
	if handler == nil {
		return errors.New("readiness handler cannot be nil")
	}
	s.Mux.Handle("/readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = handler(w)
	}))
	return nil
}

func (s *OperatorServer) Run(stopCh <-chan struct{}) error {
	// Run creates an HTTP server which exposes
	// 1. if enabled, a /metrics endpoint on the configured port
	// 2. health endpoints for liveness and readiness
	// (if port <=0, uses the default 9090)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.Port),
		Handler:           s.Mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	go func() {
		for range stopCh {
			// do nothing until closeCh is closed or receives a message
			break
		}
		ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second)
		defer cancelFunc()
		errCh <- server.Shutdown(ctx)
	}()
	err := <-errCh
	return err
}
