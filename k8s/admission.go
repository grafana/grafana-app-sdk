package k8s

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"gomodules.xyz/jsonpatch/v2"
	admission "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ErrReasonFieldNotAllowed is the "field not allowed" admission error reason string
	ErrReasonFieldNotAllowed = "field_not_allowed"

	errStringNoAdmissionControllerDefined = "no %s admission controller defined for group '%s' and kind '%s'"
)

// SimpleAdmissionError implements resource.AdmissionError
type SimpleAdmissionError struct {
	error
	statusCode int
	reason     string
}

// StatusCode returns the error's HTTP status code
func (s *SimpleAdmissionError) StatusCode() int {
	return s.statusCode
}

// Reason returns a machine-readable reason for the error
func (s *SimpleAdmissionError) Reason() string {
	return s.reason
}

// NewAdmissionError returns a new SimpleAdmissionError, which implements resource.AdmissionError
func NewAdmissionError(err error, statusCode int, reason string) *SimpleAdmissionError {
	return &SimpleAdmissionError{
		error:      err,
		statusCode: statusCode,
		reason:     reason,
	}
}

// NewMutatingResponseFromChange returns a pointer to a new MutatingResponse containing PatchOperations based on the
// change between `from` and `to` Objects.
// Note that if you already know the exact nature of your change, this operation is costlier than writing the PatchOperations yourself.
func NewMutatingResponseFromChange(from, to resource.Object) (*resource.MutatingResponse, error) {
	fromJSON, err := marshalJSON(from, nil, ClientConfig{})
	if err != nil {
		return nil, err
	}
	toJSON, err := marshalJSON(to, nil, ClientConfig{})
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.CreatePatch(fromJSON, toJSON)
	if err != nil {
		return nil, err
	}
	resp := resource.MutatingResponse{
		PatchOperations: make([]resource.PatchOperation, len(patch)),
	}
	for idx, op := range patch {
		resp.PatchOperations[idx] = resource.PatchOperation{
			Path:      op.Path,
			Operation: resource.PatchOp(op.Operation),
			Value:     op.Value,
		}
	}
	return &resp, nil
}

// OpinionatedMutatingAdmissionController is a MutatingAdmissionController which wraps an optional user-defined
// Mutate() function with a set of additional PatchOperations which set metadata and label properties.
type OpinionatedMutatingAdmissionController struct {
	MutateFunc func(request *resource.AdmissionRequest) (*resource.MutatingResponse, error)
}

// now is used to wrap time.Now so it can be altered for testing
var now = func() time.Time {
	return time.Now()
}

// Mutate runs the underlying MutateFunc() function (if non-nil), and if that returns successfully,
// appends additional patch operations to the MutatingResponse for CommonMetadata fields not in kubernetes standard metadata,
// and labels internally used by the SDK, such as the stored version.
func (o *OpinionatedMutatingAdmissionController) Mutate(request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
	// Get the response from the underlying controller, if it exists
	var err error
	var resp *resource.MutatingResponse
	if o.MutateFunc != nil {
		resp, err = o.MutateFunc(request)
		if err != nil {
			return resp, err
		}
	}
	if resp == nil || resp.PatchOperations == nil {
		resp = &resource.MutatingResponse{
			PatchOperations: make([]resource.PatchOperation, 0),
		}
	}

	switch request.Action {
	case resource.AdmissionActionCreate:
		resp.PatchOperations = append(resp.PatchOperations, resource.PatchOperation{
			Path:      "/metadata/createdBy", // Set createdBy to the request user
			Operation: resource.PatchOpReplace,
			Value:     request.UserInfo.Username,
		}, resource.PatchOperation{
			Path:      "/metadata/updateTimestamp", // Set the updateTimestamp to the creationTimestamp
			Operation: resource.PatchOpReplace,
			Value:     request.Object.CommonMetadata().CreationTimestamp.Format(time.RFC3339Nano),
		}, resource.PatchOperation{
			Path:      "/metadata/labels/" + versionLabel, // Set the internal version label to the version of the endpoint
			Operation: resource.PatchOpReplace,
			Value:     request.Version,
		})
	case resource.AdmissionActionUpdate:
		resp.PatchOperations = append(resp.PatchOperations, resource.PatchOperation{
			Path:      "/metadata/updatedBy", // Set updatedBy to the request user
			Operation: resource.PatchOpReplace,
			Value:     request.UserInfo.Username,
		}, resource.PatchOperation{
			Path:      "/metadata/updateTimestamp", // Set updateTimestamp to the current time
			Operation: resource.PatchOpReplace,
			Value:     now().Format(time.RFC3339Nano),
		}, resource.PatchOperation{
			Path:      "/metadata/labels/" + versionLabel, // Set the internal version label to the version of the endpoint
			Operation: resource.PatchOpReplace,
			Value:     request.Version,
		})
	default:
		// Do nothing
	}
	return resp, nil
}

// NewOpinionatedMutatingAdmissionController creates a pointer to a new OpinionatedMutatingAdmissionController wrapping the
// provided mutateFunc (nil mutateFunc argument is allowed, and will cause the controller to not call the underlying function)
func NewOpinionatedMutatingAdmissionController(mutateFunc func(request *resource.AdmissionRequest) (*resource.MutatingResponse, error)) *OpinionatedMutatingAdmissionController {
	return &OpinionatedMutatingAdmissionController{
		MutateFunc: mutateFunc,
	}
}

// OpinionatedValidatingAdmissionController implements resource.ValidatingAdmissionController and performs initial
// validation on reserved metadata fields which are stores as annotations in kubernetes, ensuring that if any changes are made,
// they are allowed, before calling the underlying admission validate function.
type OpinionatedValidatingAdmissionController struct {
	ValidateFunc func(*resource.AdmissionRequest) error
}

// Validate performs validation on metadata-as-annotations fields before calling the underlying admission validate function.
func (o *OpinionatedValidatingAdmissionController) Validate(request *resource.AdmissionRequest) error {
	// Check that none of the protected metadata in annotations has been changed
	switch request.Action {
	case resource.AdmissionActionCreate:
		// Not allowed to set createdBy, updatedBy, or updateTimestamp
		// createdBy can be set, but only to the username of the request
		if request.Object.CommonMetadata().CreatedBy != "" && request.Object.CommonMetadata().CreatedBy != request.UserInfo.Username {
			return NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"createdBy"), http.StatusBadRequest, ErrReasonFieldNotAllowed)
		}
		// updatedBy can be set, but only to the username of the request
		if request.Object.CommonMetadata().UpdatedBy != "" && request.Object.CommonMetadata().UpdatedBy != request.UserInfo.Username {
			return NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"updatedBy"), http.StatusBadRequest, ErrReasonFieldNotAllowed)
		}
		emptyTime := time.Time{}
		// updateTimestamp cannot be set
		if request.Object.CommonMetadata().UpdateTimestamp != emptyTime {
			return NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"updateTimestamp"), http.StatusBadRequest, ErrReasonFieldNotAllowed)
		}
	case resource.AdmissionActionUpdate:
		// Not allowed to set createdBy, updatedBy, or updateTimestamp
		// createdBy can be set, but only to the username of the request
		if request.Object.CommonMetadata().CreatedBy != request.OldObject.CommonMetadata().CreatedBy {
			return NewAdmissionError(fmt.Errorf("cannot change /metadata/annotations/"+annotationPrefix+"createdBy"), http.StatusBadRequest, ErrReasonFieldNotAllowed)
		}
		// updatedBy can be set, but only to the username of the request
		if request.Object.CommonMetadata().UpdatedBy != request.OldObject.CommonMetadata().UpdatedBy && request.Object.CommonMetadata().UpdatedBy != request.UserInfo.Username {
			return NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"updatedBy"), http.StatusBadRequest, ErrReasonFieldNotAllowed)
		}
		// updateTimestamp cannot be set
		if request.Object.CommonMetadata().UpdateTimestamp != request.OldObject.CommonMetadata().UpdateTimestamp {
			return NewAdmissionError(fmt.Errorf("cannot set /metadata/annotations/"+annotationPrefix+"updateTimestamp"), http.StatusBadRequest, ErrReasonFieldNotAllowed)
		}
	default:
		// Do nothing
	}
	// Return the result of the underlying func, if it exists
	if o.ValidateFunc != nil {
		return o.ValidateFunc(request)
	}
	return nil
}

// NewOpinionatedValidatingAdmissionController returns a new OpinionatedValidatingAdmissionController which wraps the provided
// validateFunc. If validateFunc is nil, no extra validation after the opinionated initial validation will be performed.
func NewOpinionatedValidatingAdmissionController(validateFunc func(*resource.AdmissionRequest) error) *OpinionatedValidatingAdmissionController {
	return &OpinionatedValidatingAdmissionController{
		ValidateFunc: validateFunc,
	}
}

// ValidatingAdmissionHandler provides multi-resource.ValidatingAdmissionController handling for admission requests.
// ValidatingAdmissionControllers are added in conjunction with a resource.Schema, and/or a DefaultController may be provided.
// The type exposes functions to use in admission control, such as HTTPHandler, which exposes a http.HandlerFunc.
// TODO: include other handler functions as needed
type ValidatingAdmissionHandler struct {
	DefaultController resource.ValidatingAdmissionController
	controllers       map[string]validatingAdmissionControllerTuple
}

type validatingAdmissionControllerTuple struct {
	schema     resource.Schema
	controller resource.ValidatingAdmissionController
}

// NewValidatingAdmissionHandler returns a pointer to a new ValidatingAdmissionHandler which has been properly initialized.
func NewValidatingAdmissionHandler() *ValidatingAdmissionHandler {
	return &ValidatingAdmissionHandler{
		controllers: make(map[string]validatingAdmissionControllerTuple),
	}
}

// AddController registers a ValidatingAdmissionController to be associated with a specific Schema.
// Only one ValidatingAdmissionController can be associated to a Schema. If a subsequent AddController call
// uses the same Schema, the associated ValidatingAdmissionController will be overwritten.
func (v *ValidatingAdmissionHandler) AddController(controller resource.ValidatingAdmissionController, schema resource.Schema) {
	if v.controllers == nil {
		v.controllers = make(map[string]validatingAdmissionControllerTuple)
	}
	v.controllers[gk(schema.Group(), schema.Kind())] = validatingAdmissionControllerTuple{
		schema:     schema,
		controller: controller,
	}
}

// HTTPHandler returns a http.HandlerFunc which will handle HTTP kubernetes admission requests.
// It parses the payload, unmarshals the object to the appropriate Schema's resource.Object type,
// and then calls the correlated resource.ValidatingAdmissionController.
// If there is no correlated resource.ValidatingAdmissionController, the DefaultController will be used instead,
// and the resource.Object underlying type in the resource.AdmissionRequest will be a *resource.SimpleObject.
// If no DefaultController is provided, a 500 error is returned with the content of errStringNoAdmissionControllerDefined.
// TODO: should it be a successful response instead?
func (v *ValidatingAdmissionHandler) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			w.Write([]byte(fmt.Sprintf(errStringNoAdmissionControllerDefined, admRev.APIVersion, admRev.Kind, "validating")))
			w.WriteHeader(http.StatusInternalServerError)
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
}

// MutatingAdmissionHandler provides multi-resource.MutatingAdmissionController handling for admission requests.
// MutatingAdmissionControllers are added in conjunction with a resource.Schema, and/or a DefaultController may be provided.
// The type exposes functions to use in admission control, such as HTTPHandler, which exposes a http.HandlerFunc.
// TODO: include other handler functions as needed
type MutatingAdmissionHandler struct {
	DefaultController resource.MutatingAdmissionController
	controllers       map[string]mutatingAdmissionControllerTuple
}

type mutatingAdmissionControllerTuple struct {
	schema     resource.Schema
	controller resource.MutatingAdmissionController
}

// NewMutatingAdmissionHandler returns a pointer to a new MutatingAdmissionHandler which has been properly initialized.
func NewMutatingAdmissionHandler() *MutatingAdmissionHandler {
	return &MutatingAdmissionHandler{
		controllers: make(map[string]mutatingAdmissionControllerTuple),
	}
}

// AddController registers a resource.MutatingAdmissionController to be associated with a specific Schema.
// Only one ValidatingAdmissionController can be associated to a Schema. If a subsequent AddController call
// uses the same Schema, the associated MutatingAdmissionController will be overwritten.
func (m *MutatingAdmissionHandler) AddController(controller resource.MutatingAdmissionController, schema resource.Schema) {
	if m.controllers == nil {
		m.controllers = make(map[string]mutatingAdmissionControllerTuple)
	}
	m.controllers[gk(schema.Group(), schema.Kind())] = mutatingAdmissionControllerTuple{
		schema:     schema,
		controller: controller,
	}
}

// HTTPHandler returns a http.HandlerFunc which will handle HTTP kubernetes admission requests.
// It parses the payload, unmarshals the object to the appropriate Schema's resource.Object type,
// and then calls the correlated resource.ValidatingAdmissionController.
// If there is no correlated resource.MutatingAdmissionController, the DefaultController will be used instead,
// and the resource.Object underlying type in the resource.AdmissionRequest will be a *resource.SimpleObject.
// If no DefaultController is provided, a 500 error is returned with the content of errStringNoAdmissionControllerDefined.
// TODO: should it be a successful response with no mutations instead?
func (m *MutatingAdmissionHandler) HTTPHandler(w http.ResponseWriter, r *http.Request) {
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
		w.Write([]byte(fmt.Sprintf(errStringNoAdmissionControllerDefined, admRev.APIVersion, admRev.Kind, "mutating")))
		w.WriteHeader(http.StatusInternalServerError)
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
