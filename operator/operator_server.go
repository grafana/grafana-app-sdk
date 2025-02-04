package operator

// since operator is a serverless component on its own, its associated server
// is only used for hosting the /metrics, /livez and /readyz endpoints, whichever of these apply for a given config

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type operatorServer struct {
	mux  *http.ServeMux
	Port int
}

func (s *operatorServer) RegisterLivenessHandler(handler func(http.ResponseWriter) error) error {
	if handler == nil {
		return errors.New("liveness handler cannot be nil")
	}
	s.mux.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = handler(w)
	}))
	return nil
}

func (s *operatorServer) RegisterReadinessHandler(handler func(http.ResponseWriter) error) error {
	if handler == nil {
		return errors.New("readiness handler cannot be nil")
	}
	s.mux.Handle("/readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = handler(w)
	}))
	return nil
}

func (s *operatorServer) Run(stopCh <-chan struct{}) error {
	// Run creates an HTTP server which exposes a /metrics endpoint on the configured port (if <=0, uses the default 9090)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.Port),
		Handler:           s.mux,
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
