package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	admission "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// WrapControllers instructs the WebhookServer to wrap the provided Validating and Mutating AdmissionControllers
	// in their Opinionated variants. This is done at setup time.
	WrapControllers bool
}

type TLSConfig struct {
	CertPath string
	KeyPath  string
}

func NewWebhookServer(config WebhookServerConfig) (*WebhookServer, error) {
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
	return &WebhookServer{
		validatingHandler: validatingHandler,
		mutatingHandler:   mutatingHandler,
		certPath:          config.TLSConfig.CertPath,
		keyPath:           config.TLSConfig.KeyPath,
		addr:              fmt.Sprintf(":%d", config.Port),
	}, nil
}

// WebhookServer is an HTTPS server that exposes webhooks for kubernetes admission.
// It wraps ValidatingAdmissionHandler and MutatingAdmissionHandler and exposes their Handle() methods
// as paths in an HTTPS webserver.
// It implements operator.Controller and can be added to an operator.Operator and run alongside your other controllers.
type WebhookServer struct {
	validatingHandler *ValidatingAdmissionHandler
	mutatingHandler   *MutatingAdmissionHandler
	certPath          string
	keyPath           string
	addr              string
}

func (s *WebhookServer) Run(closeChan <-chan struct{}) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", s.mutatingHandler.Handle)
	mux.HandleFunc("/validate", s.validatingHandler.Handle)
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

type ValidatingAdmissionHandler struct {
	DefaultController resource.ValidatingAdmissionController
	controllers       map[string]validatingAdmissionControllerTuple
}

type validatingAdmissionControllerTuple struct {
	schema     resource.Schema
	controller resource.ValidatingAdmissionController
}

func NewValidatingAdmissionHandler() *ValidatingAdmissionHandler {
	return &ValidatingAdmissionHandler{
		controllers: make(map[string]validatingAdmissionControllerTuple),
	}
}

func (v *ValidatingAdmissionHandler) AddController(controller resource.ValidatingAdmissionController, schema resource.Schema) {
	v.controllers[gk(schema.Group(), schema.Kind())] = validatingAdmissionControllerTuple{
		schema:     schema,
		controller: controller,
	}
}

func (v *ValidatingAdmissionHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Only POST is allowed
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Read the body
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Unmarshal the admission review
	admRev, err := unmarshalKubernetesAdmissionReview(body, resource.WireFormatJSON)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Look up the schema and controller
	var schema resource.Schema
	var controller resource.ValidatingAdmissionController
	if tpl, ok := v.controllers[gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]; ok {
		schema = tpl.schema
		controller = tpl.controller
	} else {
		// If we have a default controller, create a SimpleObject schema and use the default controller
		if v.DefaultController != nil {
			schema = resource.NewSimpleSchema(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Version, &resource.SimpleObject[any]{}, resource.WithKind(admRev.Request.RequestKind.Kind))
			controller = v.DefaultController
		}
	}

	// If we didn't get a controller, return a failure
	if controller == nil {
		// TODO
	}

	// Translate the kubernetes admission request to one with a resource.Object in it, using the schema
	admReq, err := translateKubernetesAdmissionRequest(admRev.Request, schema)
	if err != nil {
		// TODO: different error?
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Run the controller
	err = controller.Validate(admReq)
	adResp := admission.AdmissionResponse{
		UID:     admRev.Request.UID,
		Allowed: true,
	}
	if err != nil {
		addAdmissionError(&adResp, err)
	}
	bytes, err := json.Marshal(&admission.AdmissionReview{
		TypeMeta: admRev.TypeMeta,
		Response: &adResp,
	})
	if err != nil {
		// Bad news
		w.Write([]byte(err.Error())) // TODO: better
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(bytes)
	w.WriteHeader(http.StatusOK)
}

type MutatingAdmissionHandler struct {
	DefaultController resource.MutatingAdmissionController
	controllers       map[string]mutatingAdmissionControllerTuple
}

type mutatingAdmissionControllerTuple struct {
	schema     resource.Schema
	controller resource.MutatingAdmissionController
}

func NewMutatingAdmissionHandler() *MutatingAdmissionHandler {
	return &MutatingAdmissionHandler{
		controllers: make(map[string]mutatingAdmissionControllerTuple),
	}
}

func (m *MutatingAdmissionHandler) AddController(controller resource.MutatingAdmissionController, schema resource.Schema) {
	m.controllers[gk(schema.Group(), schema.Kind())] = mutatingAdmissionControllerTuple{
		schema:     schema,
		controller: controller,
	}
}

func (m *MutatingAdmissionHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Only POST is allowed
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Read the body
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Unmarshal the admission review
	admRev, err := unmarshalKubernetesAdmissionReview(body, resource.WireFormatJSON)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Look up the schema and controller
	var schema resource.Schema
	var controller resource.MutatingAdmissionController
	if tpl, ok := m.controllers[gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]; ok {
		schema = tpl.schema
		controller = tpl.controller
	} else {
		// If we have a default controller, create a SimpleObject schema and use the default controller
		if m.DefaultController != nil {
			schema = resource.NewSimpleSchema(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Version, &resource.SimpleObject[any]{}, resource.WithKind(admRev.Request.RequestKind.Kind))
			controller = m.DefaultController
		}
	}

	// If we didn't get a controller, return a failure
	if controller == nil {
		// TODO
	}

	// Translate the kubernetes admission request to one with a resource.Object in it, using the schema
	admReq, err := translateKubernetesAdmissionRequest(admRev.Request, schema)
	if err != nil {
		// TODO: different error?
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Run the controller
	mResp, err := controller.Mutate(admReq)
	adResp := admission.AdmissionResponse{
		UID:     admRev.Request.UID,
		Allowed: true,
	}
	if err == nil && len(mResp.PatchOperations) > 0 {
		pt := admission.PatchTypeJSONPatch
		adResp.PatchType = &pt
		// Re-use err here, because if we error on the JSON marshal, we'll return an error
		// admission response, rather than silently fail the patch.
		adResp.Patch, err = marshalJSONPatch(resource.PatchRequest{
			Operations: mResp.PatchOperations,
		})
	}
	if err != nil {
		addAdmissionError(&adResp, err)
	}
	bytes, err := json.Marshal(&admission.AdmissionReview{
		TypeMeta: admRev.TypeMeta,
		Response: &adResp,
	})
	if err != nil {
		// Bad news
		w.Write([]byte(err.Error())) // TODO: better
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(bytes)
	w.WriteHeader(http.StatusOK)
}

func gk(group, kind string) string {
	return fmt.Sprintf("%s.%s", kind, group)
}

func addAdmissionError(resp *admission.AdmissionResponse, err error) {
	if err == nil || resp == nil {
		return
	}
	resp.Allowed = false
	resp.Result = &metav1.Status{
		Status:  "Failure",
		Message: err.Error(),
	}
	if cast, ok := err.(resource.AdmissionError); ok {
		resp.Result.Code = int32(cast.StatusCode())
		resp.Result.Reason = metav1.StatusReason(cast.Reason())
	}
}
