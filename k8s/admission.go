package k8s

import (
	"encoding/json"
	"fmt"

	"gomodules.xyz/jsonpatch/v2"
	admission "k8s.io/api/admission/v1beta1"

	"github.com/grafana/grafana-app-sdk/resource"
)

// AdmissionRequest contains information from a kubernetes Admission request and decoded object(s).
type AdmissionRequest struct {
	Action    string
	Kind      string
	Group     string
	Version   string
	UserInfo  AdmissionUserInfo
	Object    resource.Object
	OldObject resource.Object
}

// AdmissionUserInfo contains user information for an admission request
type AdmissionUserInfo struct {
	Username string
	UID      string
	Groups   []string
	Extra    map[string]any
}

// AdmissionError is an interface which extends error to add more details for admission request rejections
type AdmissionError interface {
	error
	// StatusCode should return an HTTP status code to reject with
	StatusCode() int
	// Reason should be a machine-readable reason for the rejection
	Reason() string
}

// MutatingResponse is the mutation to perform on a request
type MutatingResponse struct {
	// PatchOperations is the list of patch ops to perform on the request as part of the mutation
	PatchOperations []resource.PatchOperation
	// corrected is a flag to dictate whether the patch has already been corrected from resource.Object representation into kubernetes representation
	// if true, we can avoid the costlier marshalJSONPatch call (calling it wouldn't alter the patch in this case)
	corrected bool
}

// ValidatingAdmissionController is an interface that describes any object which should validate admission of
// a request to manipulate a resource.Object.
type ValidatingAdmissionController interface {
	// Validate consumes an AdmissionRequest, then returns an error if the request should be denied.
	// The returned error SHOULD satisfy the AdmissionError interface, but callers will fallback
	// to using only the information in a simple error if not.
	Validate(request *AdmissionRequest) error
}

// MutatingAdmissionController is an interface that describes any object which should mutate a request to
// manipulate a resource.Object.
type MutatingAdmissionController interface {
	// Mutate consumes an AdmissionRequest, then returns a MutatingResponse with the relevant patch operations
	// to apply. If the request should not be admitted, ths function should return an error.
	// The returned error SHOULD satisfy the AdmissionError interface, but callers will fallback
	// to using only the information in a simple error if not.
	Mutate(request *AdmissionRequest) (*MutatingResponse, error)
}

// NewMutatingResponseFromChange returns a pointer to a new MutatingResponse containing PatchOperations based on the
// change between `from` and `to` Objects.
// Note that if you already know the exact nature of your change, this operation is costlier than writing the PatchOperations yourself.
func NewMutatingResponseFromChange(from, to resource.Object) (*MutatingResponse, error) {
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
	resp := MutatingResponse{
		PatchOperations: make([]resource.PatchOperation, len(patch)),
		corrected:       true,
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

func unmarshalKubernetesAdmissionReview(bytes []byte, format resource.WireFormat) (*admission.AdmissionReview, error) {
	if format != resource.WireFormatJSON {
		return nil, fmt.Errorf("unsupported WireFormat '%s'", fmt.Sprint(format))
	}

	rev := admission.AdmissionReview{}
	err := json.Unmarshal(bytes, &rev)
	if err != nil {
		return nil, err
	}
	return &rev, nil
}

func translateKubernetesAdmissionRequest(req *admission.AdmissionRequest, schema resource.Schema) (*AdmissionRequest, error) {
	var obj, old resource.Object

	obj = schema.ZeroValue()
	err := rawToObject(req.Object.Raw, obj)
	if err != nil {
		return nil, err
	}
	if len(req.OldObject.Raw) > 0 {
		old = schema.ZeroValue()
		err = rawToObject(req.OldObject.Raw, old)
		if err != nil {
			return nil, err
		}
	}

	return &AdmissionRequest{
		Action:  string(req.Operation),
		Kind:    req.Kind.Kind,
		Group:   req.Kind.Group,
		Version: req.Kind.Version,
		UserInfo: AdmissionUserInfo{
			Username: req.UserInfo.Username,
			UID:      req.UserInfo.UID,
			Groups:   req.UserInfo.Groups,
		},
		Object:    obj,
		OldObject: old,
	}, nil
}
