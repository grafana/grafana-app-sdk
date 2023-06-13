package k8s

import (
	"fmt"
	"net/http"
	"time"

	"github.com/grafana/grafana-app-sdk/resource"
	"gomodules.xyz/jsonpatch/v2"
)

const (
	ErrReasonFieldNotAllowed = "field not allowed"
)

type SimpleAdmissionError struct {
	error
	statusCode int
	reason     string
}

func (s *SimpleAdmissionError) StatusCode() int {
	return s.statusCode
}

func (s *SimpleAdmissionError) Reason() string {
	return s.reason
}

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

type OpinionatedValidatingAdmissionController struct {
	ValidateFunc func(*resource.AdmissionRequest) error
}

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

func NewOpinionatedValidatingAdmissionController(validateFunc func(*resource.AdmissionRequest) error) *OpinionatedValidatingAdmissionController {
	return &OpinionatedValidatingAdmissionController{
		ValidateFunc: validateFunc,
	}
}
