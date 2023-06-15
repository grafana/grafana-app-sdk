package k8s

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
)

type WebhookServerConfig struct {
	// The Port to run the HTTPS server on
	Port int
	// TLSConfig contains cert information for running the HTTPS server
	TLSConfig TLSConfig
	// ValidatingControllers is a map of schemas to their corresponding ValidatingAdmissionController.
	ValidatingControllers map[resource.Schema]resource.ValidatingAdmissionController
	// MutatingControllers is a map of schemas to their corresponding MutatingAdmissionController.
	MutatingControllers map[resource.Schema]resource.MutatingAdmissionController
	// DefaultValidatingController is called for any /validate requests received which don't have an entry in ValidatingControllers.
	// If left nil, an error will be returned to the caller instead.
	DefaultValidatingController resource.ValidatingAdmissionController
	// DefaultMutatingController is called for any /validate requests received which don't have an entry in MutatingControllers.
	// If left nil, an error will be returned to the caller instead.
	DefaultMutatingController resource.MutatingAdmissionController
	// WrapControllers instructs the WebhookServerController to wrap the provided Validating and Mutating AdmissionControllers
	// in their Opinionated variants. This is done at setup time.
	WrapControllers bool
}

type TLSConfig struct {
	CertPath string
	KeyPath  string
}

func NewWebhookServer(config WebhookServerConfig) (*WebhookServerController, error) {
	validatingHandler := NewValidatingAdmissionHandler()
	for sch, controller := range config.ValidatingControllers {
		if config.WrapControllers {
			controller = NewOpinionatedValidatingAdmissionController(controller.Validate)
		}
		validatingHandler.AddController(controller, sch)
	}
	validatingHandler.DefaultController = config.DefaultValidatingController
	mutatingHandler := NewMutatingAdmissionHandler()
	for sch, controller := range config.MutatingControllers {
		if config.WrapControllers {
			controller = NewOpinionatedMutatingAdmissionController(controller.Mutate)
		}
		mutatingHandler.AddController(controller, sch)
	}
	mutatingHandler.DefaultController = config.DefaultMutatingController
	return &WebhookServerController{
		validatingHandler: validatingHandler,
		mutatingHandler:   mutatingHandler,
		certPath:          config.TLSConfig.CertPath,
		keyPath:           config.TLSConfig.KeyPath,
		addr:              fmt.Sprintf(":%d", config.Port),
	}, nil
}

func NewWebhookServerController(tlsConfig TLSConfig, port int) *WebhookServerController {
	return &WebhookServerController{
		validatingHandler: NewValidatingAdmissionHandler(),
		mutatingHandler:   NewMutatingAdmissionHandler(),
		certPath:          tlsConfig.CertPath,
		keyPath:           tlsConfig.KeyPath,
		addr:              fmt.Sprintf(":%d", port),
	}
}

// WebhookServerController is an HTTPS server that exposes webhooks for kubernetes admission.
// It wraps ValidatingAdmissionHandler and MutatingAdmissionHandler and exposes their HTTPHandler() methods
// as paths in an HTTPS webserver.
// It implements operator.Controller and can be added to an operator.Operator and run alongside your other controllers.
type WebhookServerController struct {
	validatingHandler *ValidatingAdmissionHandler
	mutatingHandler   *MutatingAdmissionHandler
	certPath          string
	keyPath           string
	addr              string
}

func (s *WebhookServerController) AddValidatingAdmissionController(controller resource.ValidatingAdmissionController, schema resource.Schema) {
	if s.validatingHandler == nil {
		s.validatingHandler = NewValidatingAdmissionHandler()
	}
	s.validatingHandler.AddController(controller, schema)
}

func (s *WebhookServerController) AddMutatingAdmissionController(controller resource.MutatingAdmissionController, schema resource.Schema) {
	if s.mutatingHandler == nil {
		s.mutatingHandler = NewMutatingAdmissionHandler()
	}
	s.mutatingHandler.AddController(controller, schema)
}

// Run implements the operator.Controller interface and runs the HTTPS server until either the closeCh is closed,
// or the HTTPS server's process returns an error.
func (s *WebhookServerController) Run(closeChan <-chan struct{}) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/validate", s.validatingHandler.HTTPHandler())
	mux.HandleFunc("/mutate", s.mutatingHandler.HTTPHandler)
	server := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServeTLS(s.certPath, s.keyPath)
	}()
	go func() {
		for range closeChan {
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
