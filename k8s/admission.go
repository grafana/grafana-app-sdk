package k8s

import (
	"fmt"
	"net/http"
	"time"

	"gomodules.xyz/jsonpatch/v2"

	"github.com/grafana/grafana-app-sdk/resource"
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
var now = time.Now

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
		// Patch is tricky when it comes to add vs replace operations for maps, like labels and annotations
		resp.PatchOperations = append(resp.PatchOperations, resource.PatchOperation{
			Path:      "/metadata/createdBy", // Set createdBy to the request user
			Operation: resource.PatchOpReplace,
			Value:     request.UserInfo.Username,
		}, resource.PatchOperation{
			Path:      "/metadata/updateTimestamp", // Set the updateTimestamp to the creationTimestamp
			Operation: resource.PatchOpReplace,
			Value:     request.Object.CommonMetadata().CreationTimestamp.Format(time.RFC3339Nano),
		})
		// TODO: unsure on this
		if len(request.Object.CommonMetadata().Labels) == 0 {
			ops := append(make([]resource.PatchOperation, 0), resource.PatchOperation{
				Path:      "/metadata/labels",
				Operation: resource.PatchOpAdd,
				Value: map[string]string{
					versionLabel: request.Version,
				},
			})
			ops = append(ops, resp.PatchOperations...)
			resp.PatchOperations = ops
		} else {
			resp.PatchOperations = append(resp.PatchOperations, resource.PatchOperation{
				Path:      "/metadata/labels/" + versionLabel, // Set the internal version label to the version of the endpoint
				Operation: resource.PatchOpAdd,
				Value:     request.Version,
			})
		}
	case resource.AdmissionActionUpdate:
		// Patch is tricky when it comes to add vs replace operations for maps, like labels and annotations
		resp.PatchOperations = append(resp.PatchOperations, resource.PatchOperation{
			Path:      "/metadata/updatedBy", // Set createdBy to the request user
			Operation: resource.PatchOpReplace,
			Value:     request.UserInfo.Username,
		}, resource.PatchOperation{
			Path:      "/metadata/updateTimestamp", // Set the updateTimestamp to the creationTimestamp
			Operation: resource.PatchOpReplace,
			Value:     now().Format(time.RFC3339Nano),
		})
		// TODO: unsure on this
		if len(request.Object.CommonMetadata().Labels) == 0 {
			ops := append(make([]resource.PatchOperation, 0), resource.PatchOperation{
				Path:      "/metadata/labels",
				Operation: resource.PatchOpAdd,
				Value: map[string]string{
					versionLabel: request.Version,
				},
			})
			ops = append(ops, resp.PatchOperations...)
			resp.PatchOperations = ops
		} else {
			resp.PatchOperations = append(resp.PatchOperations, resource.PatchOperation{
				Path:      "/metadata/labels/" + versionLabel, // Set the internal version label to the version of the endpoint
				Operation: resource.PatchOpAdd,
				Value:     request.Version,
			})
		}
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
