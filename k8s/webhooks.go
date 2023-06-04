package k8s

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/grafana/grafana-app-sdk/resource"
	admission "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TLSConfig struct {
	CertPath string
	KeyPath  string
}

func NewWebhookServer() {

}

type WebhookServer struct {
	handler  *WebhookHandler
	certPath string
	keyPath  string
}

func (s *WebhookServer) ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/mutate", s.handler.MutatingAdmissionHandler())
	mux.Handle("/validate", s.handler.MutatingAdmissionHandler())
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return server.ListenAndServeTLS(s.certPath, s.keyPath)
}

type WebhookHandler struct {
	schemas              map[string]resource.Schema
	admissionControllers map[string]ValidatingAdmissionController
	mutatingControllers  map[string]MutatingAdmissionController
	mux                  sync.RWMutex
}

func (w *WebhookHandler) RegisterValidatingController(controller ValidatingAdmissionController, schema resource.Schema) {
	w.mux.Lock()
	defer w.mux.Unlock()
	gk := w.gk(schema.Group(), schema.Kind())
	w.schemas[gk] = schema
	w.admissionControllers[gk] = controller
}

func (w *WebhookHandler) RegisterMutatingController(controller MutatingAdmissionController, schema resource.Schema) {
	w.mux.Lock()
	defer w.mux.Unlock()
	gk := w.gk(schema.Group(), schema.Kind())
	w.schemas[gk] = schema
	w.mutatingControllers[gk] = controller
}

func (w *WebhookHandler) ValidatingAdmissionHandler() http.HandlerFunc {
	// TODO: de-dup code between this and MutatingAdmissionHandler
	return func(writer http.ResponseWriter, req *http.Request) {
		// Only POST is allowed
		if req.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.mux.RLock()
		defer w.mux.RUnlock()

		admRev, admReq, err := w.unmarshal(req)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusInternalServerError)
		}

		controller, ok := w.admissionControllers[w.gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]
		if !ok {
			// No mutating web hooks
			writer.WriteHeader(http.StatusOK)
			return
		}

		err = controller.Validate(admReq)
		adResp := admission.AdmissionResponse{
			UID:     admRev.Request.UID,
			Allowed: true,
		}
		if err != nil {
			w.addAdmissionError(&adResp, err)
		}
		bytes, err := json.Marshal(&admission.AdmissionReview{
			TypeMeta: admRev.TypeMeta,
			Response: &adResp,
		})
		if err != nil {
			// Bad news
			writer.Write([]byte(err.Error())) // TODO: better
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.Write(bytes)
	}
}

func (w *WebhookHandler) MutatingAdmissionHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		// Only POST is allowed
		if req.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.mux.RLock()
		defer w.mux.RUnlock()

		admRev, admReq, err := w.unmarshal(req)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusInternalServerError)
		}

		controller, ok := w.mutatingControllers[w.gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]
		if !ok {
			// No mutating web hooks
			writer.WriteHeader(http.StatusOK)
			return
		}

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
			w.addAdmissionError(&adResp, err)
		}
		bytes, err := json.Marshal(&admission.AdmissionReview{
			TypeMeta: admRev.TypeMeta,
			Response: &adResp,
		})
		if err != nil {
			// Bad news
			writer.Write([]byte(err.Error())) // TODO: better
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.Write(bytes)
	}
}

func (w *WebhookHandler) unmarshal(req *http.Request) (*admission.AdmissionReview, *AdmissionRequest, error) {
	body, err := io.ReadAll(req.Body)
	defer req.Body.Close()
	if err != nil {
		// TODO: better error?
		return nil, nil, err
	}

	// Unmarshal the admission review
	admRev, err := unmarshalKubernetesAdmissionReview(body, resource.WireFormatJSON)
	if err != nil {
		return nil, nil, err
	}
	// Look up the schema
	var schema resource.Schema
	if sch, ok := w.schemas[w.gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]; ok {
		schema = sch
	} else {
		// If we don't have a schema for this, create a simple one
		// TODO: warn logging?
		schema = resource.NewSimpleSchema(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Version, &resource.SimpleObject[any]{}, resource.WithKind(admRev.Request.RequestKind.Kind))
	}

	admReq, err := translateKubernetesAdmissionRequest(admRev.Request, schema)
	if err != nil {
		return nil, nil, err
	}

	return admRev, admReq, nil
}

func (*WebhookHandler) addAdmissionError(resp *admission.AdmissionResponse, err error) {
	if err == nil || resp == nil {
		return
	}
	resp.Allowed = false
	resp.Result = &metav1.Status{
		Status:  "Failure",
		Message: err.Error(),
	}
	if cast, ok := err.(AdmissionError); ok {
		resp.Result.Code = int32(cast.StatusCode())
		resp.Result.Reason = metav1.StatusReason(cast.Reason())
	}
}

func (w *WebhookHandler) gk(group, kind string) string {
	return fmt.Sprintf("%s.%s", kind, group)
}
