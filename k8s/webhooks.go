package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gomodules.xyz/jsonpatch/v2"
	admission "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/resource"
)

// WebhookServerConfig is the configuration object for a WebhookServer, used with NewWebhookServer.
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

// TLSConfig describes a set of TLS files
type TLSConfig struct {
	// CertPath is the path to the on-disk cert file
	CertPath string
	// KeyPath is the path to the on-disk key file for the cert
	KeyPath string
}

// WebhookServer is a kubernetes webhook server, which exposes /validate and /mutate HTTPS endpoints.
// It implements operator.Controller and can be run as a controller in an operator, or as a standalone process.
type WebhookServer struct {
	// DefaultValidatingController is the default ValidatingAdmissionController to use if one is not defined for the schema in the request.
	// If this is empty, the request will be rejected.
	DefaultValidatingController resource.ValidatingAdmissionController
	// DefaultMutatingController is the default MutatingAdmissionController to use if one is not defined for the schema in the request.
	// If this is empty, the request will be rejected.
	DefaultMutatingController resource.MutatingAdmissionController
	validatingControllers     map[string]validatingAdmissionControllerTuple
	mutatingControllers       map[string]mutatingAdmissionControllerTuple
	port                      int
	tlsConfig                 TLSConfig
}

// NewWebhookServer creates a new WebhookServer using the provided configuration.
// The only required parts of the config are the Port and TLSConfig, as all other parts
// (default controllers, schema-specific controllers) can be set post-initialization.
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

// AddValidatingAdmissionController adds a resource.ValidatingAdmissionController to the WebhookServer, associated with a given schema.
// The schema association associates all incoming requests of the same group and kind of the schema to the schema's ZeroValue object.
// If a ValidatingAdmissionController already exists for the provided schema, the one provided in this call will be used instead of the extant one.
func (w *WebhookServer) AddValidatingAdmissionController(controller resource.ValidatingAdmissionController, schema resource.Schema) {
	if w.validatingControllers == nil {
		w.validatingControllers = make(map[string]validatingAdmissionControllerTuple)
	}
	w.validatingControllers[gk(schema.Group(), schema.Kind())] = validatingAdmissionControllerTuple{
		schema:     schema,
		controller: controller,
	}
}

// AddMutatingAdmissionController adds a resource.MutatingAdmissionController to the WebhookServer, associated with a given schema.
// The schema association associates all incoming requests of the same group and kind of the schema to the schema's ZeroValue object.
// If a MutatingAdmissionController already exists for the provided schema, the one provided in this call will be used instead of the extant one.
func (w *WebhookServer) AddMutatingAdmissionController(controller resource.MutatingAdmissionController, schema resource.Schema) {
	if w.mutatingControllers == nil {
		w.mutatingControllers = make(map[string]mutatingAdmissionControllerTuple)
	}
	w.mutatingControllers[gk(schema.Group(), schema.Kind())] = mutatingAdmissionControllerTuple{
		schema:     schema,
		controller: controller,
	}
}

// Run establishes an HTTPS server on the configured port and exposes `/validate` and `/mutate` paths for kubernetes
// validating and mutating webhooks, respectively. It will block until either closeChan is closed (in which case it returns nil),
// or the server encounters an unrecoverable error (in which case it returns the error).
func (w *WebhookServer) Run(closeChan <-chan struct{}) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/validate", w.HandleValidateHTTP)
	mux.HandleFunc("/mutate", w.HandleMutateHTTP)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", w.port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
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

// HandleValidateHTTP is the HTTP HandlerFunc for a kubernetes validating webhook call
// nolint:errcheck,revive
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
	} else if w.DefaultValidatingController != nil {
		// If we have a default controller, create a SimpleObject schema and use the default controller
		schema = resource.NewSimpleSchema(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Version, &resource.SimpleObject[any]{}, resource.WithKind(admRev.Request.RequestKind.Kind))
		controller = w.DefaultValidatingController
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
	err = controller.Validate(req.Context(), admReq)
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

// HandleMutateHTTP is the HTTP HandlerFunc for a kubernetes mutating webhook call
// nolint:errcheck,revive,funlen
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
		fmt.Println(err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Look up the schema and controller
	var schema resource.Schema
	var controller resource.MutatingAdmissionController
	if tpl, ok := w.mutatingControllers[gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]; ok {
		schema = tpl.schema
		controller = tpl.controller
	} else if w.DefaultMutatingController != nil {
		// If we have a default controller, create a SimpleObject schema and use the default controller
		schema = resource.NewSimpleSchema(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Version, &resource.SimpleObject[any]{}, resource.WithKind(admRev.Request.RequestKind.Kind))
		controller = w.DefaultMutatingController
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
	mResp, err := controller.Mutate(context.Background(), admReq)
	adResp := admission.AdmissionResponse{
		UID:     admRev.Request.UID,
		Allowed: true,
	}
	if err == nil && mResp != nil && mResp.UpdatedObject != nil {
		pt := admission.PatchTypeJSONPatch
		adResp.PatchType = &pt
		// Re-use `err` here, we handle it below
		adResp.Patch, err = w.generatePatch(admRev, mResp.UpdatedObject)
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

func (*WebhookServer) generatePatch(admRev *admission.AdmissionReview, alteredObject resource.Object) ([]byte, error) {
	// We need to generate a list of JSONPatch operations for updating the existing object to the provided one.
	// To start, we need to translate the provided object into its kubernetes bytes representation
	newObjBytes, err := marshalJSON(alteredObject, nil, ClientConfig{})
	if err != nil {
		return nil, err
	}
	// Now, we generate a patch using the bytes provided to us in the admission request
	patch, err := jsonpatch.CreatePatch(admRev.Request.Object.Raw, newObjBytes)
	if err != nil {
		return nil, err
	}
	return json.Marshal(patch)
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
