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
}

type TLSConfig struct {
	CertPath string
	KeyPath  string
}

type WebhookServer struct {
	DefaultValidatingController resource.ValidatingAdmissionController
	DefaultMutatingController   resource.MutatingAdmissionController
	validatingControllers       map[string]validatingAdmissionControllerTuple
	mutatingControllers         map[string]mutatingAdmissionControllerTuple
	port                        int
	tlsConfig                   TLSConfig
}

func NewWebhookServer(config WebhookServerConfig) (*WebhookServer, error) {
	if config.Port < 1 || config.Port > 65536 {
		return nil, fmt.Errorf("config.Port must be a valid port number (between 1 and 65536)")
	}
	if config.TLSConfig.CertPath == "" {
		return nil, fmt.Errorf("config.TLSConfig.CertPath is required")
	}
	if config.TLSConfig.KeyPath == "" {
		return nil, fmt.Errorf("config.TLSConfig.KeyPath is required")
	}

	ws := WebhookServer{
		DefaultValidatingController: config.DefaultValidatingController,
		DefaultMutatingController:   config.DefaultMutatingController,
		validatingControllers:       make(map[string]validatingAdmissionControllerTuple),
		mutatingControllers:         make(map[string]mutatingAdmissionControllerTuple),
		port:                        config.Port,
		tlsConfig:                   config.TLSConfig,
	}

	for sch, controller := range config.ValidatingControllers {
		ws.AddValidatingAdmissionController(controller, sch)
	}

	for sch, controller := range config.MutatingControllers {
		ws.AddMutatingAdmissionController(controller, sch)
	}

	return &ws, nil
}

func (w *WebhookServer) AddValidatingAdmissionController(controller resource.ValidatingAdmissionController, schema resource.Schema) {
	if w.validatingControllers == nil {
		w.validatingControllers = make(map[string]validatingAdmissionControllerTuple)
	}
	w.validatingControllers[gk(schema.Group(), schema.Kind())] = validatingAdmissionControllerTuple{
		schema:     schema,
		controller: controller,
	}
}

func (w *WebhookServer) AddMutatingAdmissionController(controller resource.MutatingAdmissionController, schema resource.Schema) {
	if w.mutatingControllers == nil {
		w.mutatingControllers = make(map[string]mutatingAdmissionControllerTuple)
	}
	w.mutatingControllers[gk(schema.Group(), schema.Kind())] = mutatingAdmissionControllerTuple{
		schema:     schema,
		controller: controller,
	}
}

func (w *WebhookServer) Run(closeChan <-chan struct{}) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/validate", w.HandleValidateHTTP)
	mux.HandleFunc("/mutate", w.HandleMutateHTTP)
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", w.port),
		Handler: mux,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServeTLS(w.tlsConfig.CertPath, w.tlsConfig.KeyPath)
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

func (w *WebhookServer) HandleValidateHTTP(writer http.ResponseWriter, req *http.Request) {
	// Only POST is allowed
	if req.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Read the body
	body, err := io.ReadAll(req.Body)
	defer req.Body.Close()
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Unmarshal the admission review
	admRev, err := unmarshalKubernetesAdmissionReview(body, resource.WireFormatJSON)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Look up the schema and controller
	var schema resource.Schema
	var controller resource.ValidatingAdmissionController
	if tpl, ok := w.validatingControllers[gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]; ok {
		schema = tpl.schema
		controller = tpl.controller
	} else {
		// If we have a default controller, create a SimpleObject schema and use the default controller
		if w.DefaultValidatingController != nil {
			schema = resource.NewSimpleSchema(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Version, &resource.SimpleObject[any]{}, resource.WithKind(admRev.Request.RequestKind.Kind))
			controller = w.DefaultValidatingController
		}
	}

	// If we didn't get a controller, return a failure
	if controller == nil {
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Write([]byte(fmt.Sprintf(errStringNoAdmissionControllerDefined, "validating", admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)))
		return
	}

	// Translate the kubernetes admission request to one with a resource.Object in it, using the schema
	admReq, err := translateKubernetesAdmissionRequest(admRev.Request, schema)
	if err != nil {
		// TODO: different error?
		writer.WriteHeader(http.StatusBadRequest)
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
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Write([]byte(err.Error())) // TODO: better
		return
	}
	writer.WriteHeader(http.StatusOK)
	writer.Write(bytes)
}

func (w *WebhookServer) HandleMutateHTTP(writer http.ResponseWriter, req *http.Request) {
	// Only POST is allowed
	if req.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Read the body
	body, err := io.ReadAll(req.Body)
	defer req.Body.Close()
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Unmarshal the admission review
	admRev, err := unmarshalKubernetesAdmissionReview(body, resource.WireFormatJSON)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Look up the schema and controller
	var schema resource.Schema
	var controller resource.MutatingAdmissionController
	if tpl, ok := w.mutatingControllers[gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]; ok {
		schema = tpl.schema
		controller = tpl.controller
	} else {
		// If we have a default controller, create a SimpleObject schema and use the default controller
		if w.DefaultMutatingController != nil {
			schema = resource.NewSimpleSchema(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Version, &resource.SimpleObject[any]{}, resource.WithKind(admRev.Request.RequestKind.Kind))
			controller = w.DefaultMutatingController
		}
	}

	// If we didn't get a controller, return a failure
	if controller == nil {
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Write([]byte(fmt.Sprintf(errStringNoAdmissionControllerDefined, "mutating", admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)))
		return
	}

	// Translate the kubernetes admission request to one with a resource.Object in it, using the schema
	admReq, err := translateKubernetesAdmissionRequest(admRev.Request, schema)
	if err != nil {
		// TODO: different error?
		writer.WriteHeader(http.StatusBadRequest)
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
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Write([]byte(err.Error())) // TODO: better
		return
	}
	writer.WriteHeader(http.StatusOK)
	writer.Write(bytes)
}

type validatingAdmissionControllerTuple struct {
	schema     resource.Schema
	controller resource.ValidatingAdmissionController
}

type mutatingAdmissionControllerTuple struct {
	schema     resource.Schema
	controller resource.MutatingAdmissionController
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
