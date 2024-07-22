package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"gomodules.xyz/jsonpatch/v2"
	admission "k8s.io/api/admission/v1beta1"
	conversion "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	PathConversion = "convert"
	PathValidation = "validate"
	PathMutation   = "mutate"
)

const (
	// ErrReasonFieldNotAllowed is the "field not allowed" admission error reason string
	ErrReasonFieldNotAllowed = "field_not_allowed"

	errStringNoAdmissionControllerDefined = "no %s admission controller defined for group '%s' and kind '%s'"
)

type ControllerConfig struct {
	// ValidatingControllers is a map of schemas to their corresponding ValidatingAdmissionController.
	ValidatingControllers map[*resource.Kind]resource.ValidatingAdmissionController
	// MutatingControllers is a map of schemas to their corresponding MutatingAdmissionController.
	MutatingControllers map[*resource.Kind]resource.MutatingAdmissionController
	// KindConverters is a map of GroupKind to a Converter which can parse any valid version of the kind
	// and return any valid version of the kind.
	KindConverters map[metav1.GroupKind]k8s.Converter
	// DefaultValidatingController is called for any /validate requests received which don't have an entry in ValidatingControllers.
	// If left nil, an error will be returned to the caller instead.
	DefaultValidatingController resource.ValidatingAdmissionController
	// DefaultMutatingController is called for any /validate requests received which don't have an entry in MutatingControllers.
	// If left nil, an error will be returned to the caller instead.
	DefaultMutatingController resource.MutatingAdmissionController
}

type Controller struct {
	// DefaultValidatingController is the default ValidatingAdmissionController to use if one is not defined for the schema in the request.
	// If this is empty, the request will be rejected.
	DefaultValidatingController resource.ValidatingAdmissionController
	// DefaultMutatingController is the default MutatingAdmissionController to use if one is not defined for the schema in the request.
	// If this is empty, the request will be rejected.
	DefaultMutatingController resource.MutatingAdmissionController
	validatingControllers     map[string]validatingAdmissionControllerTuple
	mutatingControllers       map[string]mutatingAdmissionControllerTuple
	converters                map[string]k8s.Converter
}

func NewController(config ControllerConfig) (*Controller, error) {
	c := Controller{
		DefaultValidatingController: config.DefaultValidatingController,
		DefaultMutatingController:   config.DefaultMutatingController,
		validatingControllers:       make(map[string]validatingAdmissionControllerTuple),
		mutatingControllers:         make(map[string]mutatingAdmissionControllerTuple),
		converters:                  make(map[string]k8s.Converter),
	}

	for sch, controller := range config.ValidatingControllers {
		c.AddValidatingAdmissionController(controller, *sch)
	}

	for sch, controller := range config.MutatingControllers {
		c.AddMutatingAdmissionController(controller, *sch)
	}

	for gv, conv := range config.KindConverters {
		c.AddConverter(conv, gv)
	}

	return &c, nil
}

// AddValidatingAdmissionController adds a resource.ValidatingAdmissionController to the WebhookServer, associated with a given schema.
// The schema association associates all incoming requests of the same group and kind of the schema to the schema's ZeroValue object.
// If a ValidatingAdmissionController already exists for the provided schema, the one provided in this call will be used instead of the extant one.
func (c *Controller) AddValidatingAdmissionController(controller resource.ValidatingAdmissionController, kind resource.Kind) {
	if c.validatingControllers == nil {
		c.validatingControllers = make(map[string]validatingAdmissionControllerTuple)
	}
	c.validatingControllers[gk(kind.Group(), kind.Kind())] = validatingAdmissionControllerTuple{
		schema:     kind,
		controller: controller,
	}
}

// AddMutatingAdmissionController adds a resource.MutatingAdmissionController to the WebhookServer, associated with a given schema.
// The schema association associates all incoming requests of the same group and kind of the schema to the schema's ZeroValue object.
// If a MutatingAdmissionController already exists for the provided schema, the one provided in this call will be used instead of the extant one.
func (c *Controller) AddMutatingAdmissionController(controller resource.MutatingAdmissionController, kind resource.Kind) {
	if c.mutatingControllers == nil {
		c.mutatingControllers = make(map[string]mutatingAdmissionControllerTuple)
	}
	c.mutatingControllers[gk(kind.Group(), kind.Kind())] = mutatingAdmissionControllerTuple{
		schema:     kind,
		controller: controller,
	}
}

// AddConverter adds a Converter to the WebhookServer, associated with the given group and kind.
func (c *Controller) AddConverter(converter k8s.Converter, groupKind metav1.GroupKind) {
	if c.converters == nil {
		c.converters = make(map[string]k8s.Converter)
	}
	c.converters[gk(groupKind.Group, groupKind.Kind)] = converter
}

func (c *Controller) Run(_ <-chan struct{}) {
	backend.Serve(backend.ServeOpts{
		CallResourceHandler: c,
	})
}

func (c *Controller) RunPlugin() {}

func (c *Controller) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	// Decode admission request type by CallResourceRequest path
	logging.FromContext(ctx).Error("HELLO", "path", req.Path)
	switch strings.Trim(req.Path, "/") {
	case PathConversion:
		return c.handleConvert(ctx, req, sender)
	case PathValidation:
		return c.handleValidate(ctx, req, sender)
	case PathMutation:
		return c.handleMutate(ctx, req, sender)
	}
	return sender.Send(&backend.CallResourceResponse{
		Status: http.StatusNotFound,
		Body:   []byte(`{"error":"page not found"}`),
	})
}

func (c *Controller) handleValidate(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	// Only POST is allowed
	if req.Method != http.MethodPost {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusMethodNotAllowed,
		})
	}

	// Unmarshal the admission review
	admRev, err := unmarshalKubernetesAdmissionReview(req.Body, resource.WireFormatJSON)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusBadRequest,
		})
	}

	// Look up the schema and controller
	var schema resource.Kind
	var controller resource.ValidatingAdmissionController
	if tpl, ok := c.validatingControllers[gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]; ok {
		schema = tpl.schema
		controller = tpl.controller
	} else if c.DefaultMutatingController != nil {
		// If we have a default controller, create a SimpleObject schema and use the default controller
		schema.Schema = resource.NewSimpleSchema(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Version, &resource.TypedSpecObject[any]{}, &resource.TypedList[*resource.TypedSpecObject[any]]{}, resource.WithKind(admRev.Request.RequestKind.Kind))
		schema.Codecs = map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()}
		controller = c.DefaultValidatingController
	}

	// If we didn't get a controller, return a failure
	if controller == nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusInternalServerError,
			Body:   []byte(fmt.Sprintf(errStringNoAdmissionControllerDefined, "validating", admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)),
		})
	}

	// Translate the kubernetes admission request to one with a resource.Object in it, using the schema
	admReq, err := translateKubernetesAdmissionRequest(admRev.Request, schema)
	if err != nil {
		// TODO: different error?
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusBadRequest,
		})
	}

	// Run the controller
	err = controller.Validate(ctx, admReq)
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
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusInternalServerError,
			Body:   []byte(err.Error()),
		})
	}
	return sender.Send(&backend.CallResourceResponse{
		Status: http.StatusOK,
		Body:   bytes,
	})
}

func (c *Controller) handleMutate(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	// Only POST is allowed
	if req.Method != http.MethodPost {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusMethodNotAllowed,
		})
	}

	// Unmarshal the admission review
	admRev, err := unmarshalKubernetesAdmissionReview(req.Body, resource.WireFormatJSON)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusBadRequest,
		})
	}

	// Look up the schema and controller
	var schema resource.Kind
	var controller resource.MutatingAdmissionController
	if tpl, ok := c.mutatingControllers[gk(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)]; ok {
		schema = tpl.schema
		controller = tpl.controller
	} else if c.DefaultMutatingController != nil {
		// If we have a default controller, create a SimpleObject schema and use the default controller
		schema.Schema = resource.NewSimpleSchema(admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Version, &resource.TypedSpecObject[any]{}, &resource.TypedList[*resource.TypedSpecObject[any]]{}, resource.WithKind(admRev.Request.RequestKind.Kind))
		schema.Codecs = map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()}
		controller = c.DefaultMutatingController
	}

	// If we didn't get a controller, return a failure
	if controller == nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusInternalServerError,
			Body:   []byte(fmt.Sprintf(errStringNoAdmissionControllerDefined, "mutating", admRev.Request.RequestKind.Group, admRev.Request.RequestKind.Kind)),
		})
	}

	// Translate the kubernetes admission request to one with a resource.Object in it, using the schema
	admReq, err := translateKubernetesAdmissionRequest(admRev.Request, schema)
	if err != nil {
		// TODO: different error?
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusBadRequest,
		})
	}

	// Run the controller
	mResp, err := controller.Mutate(ctx, admReq)
	adResp := admission.AdmissionResponse{
		UID:     admRev.Request.UID,
		Allowed: true,
	}
	if err == nil && mResp != nil && mResp.UpdatedObject != nil {
		pt := admission.PatchTypeJSONPatch
		adResp.PatchType = &pt
		// Re-use `err` here, we handle it below
		adResp.Patch, err = c.generatePatch(admRev, mResp.UpdatedObject, schema.Codec(resource.KindEncodingJSON))
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
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusInternalServerError,
			Body:   []byte(err.Error()),
		})
	}
	return sender.Send(&backend.CallResourceResponse{
		Status: http.StatusOK,
		Body:   bytes,
	})
}

func (c *Controller) handleConvert(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	// Only POST is allowed
	if req.Method != http.MethodPost {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusMethodNotAllowed,
		})
	}

	// Unmarshal the ConversionReview
	rev := conversion.ConversionReview{}
	err := json.Unmarshal(req.Body, &rev)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusBadRequest,
			Body:   []byte(err.Error()),
		})
	}

	if rev.Response == nil {
		rev.Response = &conversion.ConversionResponse{
			ConvertedObjects: make([]runtime.RawExtension, 0),
		}
	}
	// Pre-fill the response
	rev.Response.UID = rev.Request.UID
	// We'll update this away from a success if there is an error along the way
	rev.Response.Result.Code = http.StatusOK
	rev.Response.Result.Status = metav1.StatusSuccess

	// Go through each object in the request
	for _, obj := range rev.Request.Objects {
		// Partly unmarshal to find the kind and APIVersion
		tm := metav1.TypeMeta{}
		err = json.Unmarshal(obj.Raw, &tm)
		if err != nil {
			rev.Response.Result.Status = metav1.StatusFailure
			rev.Response.Result.Code = http.StatusBadRequest
			rev.Response.Result.Message = err.Error()
			logging.FromContext(ctx).Error("Error unmarshaling basic type data from object for conversion", "error", err.Error())
			break
		}
		// Get the associated converter for this kind
		conv, ok := c.converters[gk(tm.GroupVersionKind().Group, tm.Kind)]
		if !ok {
			// No converter for this kind
			rev.Response.Result.Status = metav1.StatusFailure
			rev.Response.Result.Code = http.StatusUnprocessableEntity
			rev.Response.Result.Message = fmt.Sprintf("No converter registered for kind %s", tm.Kind)
			logging.FromContext(ctx).Error("No converter has been registered for this groupKind", "kind", tm.Kind, "group", tm.GetObjectKind().GroupVersionKind().Group)
			break
		}
		// Do the conversion
		// Partial unmarshal to get kind and APIVersion
		res, err := conv.Convert(k8s.RawKind{
			Kind:       tm.Kind,
			APIVersion: tm.APIVersion,
			Group:      tm.GroupVersionKind().Group,
			Version:    tm.GroupVersionKind().Version,
			Raw:        obj.Raw,
		}, rev.Request.DesiredAPIVersion)
		if err != nil {
			// Conversion error
			rev.Response.Result.Status = metav1.StatusFailure
			rev.Response.Result.Code = http.StatusInternalServerError
			rev.Response.Result.Message = "Error converting object"
			logging.FromContext(ctx).Error("Error converting object", "error", err.Error())
			break
		}
		rev.Response.ConvertedObjects = append(rev.Response.ConvertedObjects, runtime.RawExtension{
			Raw: res,
		})
	}
	resp, err := json.Marshal(rev)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusInternalServerError,
		})
	}
	return sender.Send(&backend.CallResourceResponse{
		Status: http.StatusOK,
		Body:   resp,
	})
}

func (*Controller) generatePatch(admRev *admission.AdmissionReview, alteredObject resource.Object, codec resource.Codec) ([]byte, error) {
	// We need to generate a list of JSONPatch operations for updating the existing object to the provided one.
	// To start, we need to translate the provided object into its kubernetes bytes representation
	buf := &bytes.Buffer{}
	err := codec.Write(buf, alteredObject)
	if err != nil {
		return nil, err
	}
	// Now, we generate a patch using the bytes provided to us in the admission request
	patch, err := jsonpatch.CreatePatch(admRev.Request.Object.Raw, buf.Bytes())
	if err != nil {
		return nil, err
	}
	return json.Marshal(patch)
}

type validatingAdmissionControllerTuple struct {
	schema     resource.Kind
	controller resource.ValidatingAdmissionController
}

type mutatingAdmissionControllerTuple struct {
	schema     resource.Kind
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

func unmarshalKubernetesAdmissionReview(raw []byte, format resource.WireFormat) (*admission.AdmissionReview, error) {
	if format != resource.WireFormatJSON {
		return nil, fmt.Errorf("unsupported WireFormat '%s'", fmt.Sprint(format))
	}

	rev := admission.AdmissionReview{}
	err := json.Unmarshal(raw, &rev)
	if err != nil {
		return nil, err
	}
	return &rev, nil
}

func translateKubernetesAdmissionRequest(req *admission.AdmissionRequest, sch resource.Kind) (*resource.AdmissionRequest, error) {
	var err error
	var obj, old resource.Object

	if len(req.Object.Raw) > 0 {
		obj, err = sch.Read(bytes.NewReader(req.Object.Raw), resource.KindEncodingJSON)
		if err != nil {
			return nil, err
		}
	}
	if len(req.OldObject.Raw) > 0 {
		old, err = sch.Read(bytes.NewReader(req.OldObject.Raw), resource.KindEncodingJSON)
		if err != nil {
			return nil, err
		}
	}

	var action resource.AdmissionAction
	switch req.Operation {
	case admission.Create:
		action = resource.AdmissionActionCreate
	case admission.Update:
		action = resource.AdmissionActionUpdate
	case admission.Delete:
		action = resource.AdmissionActionDelete
	case admission.Connect:
		action = resource.AdmissionActionConnect
	}

	return &resource.AdmissionRequest{
		Action:  action,
		Kind:    req.Kind.Kind,
		Group:   req.Kind.Group,
		Version: req.Kind.Version,
		UserInfo: resource.AdmissionUserInfo{
			Username: req.UserInfo.Username,
			UID:      req.UserInfo.UID,
			Groups:   req.UserInfo.Groups,
		},
		Object:    obj,
		OldObject: old,
	}, nil
}
