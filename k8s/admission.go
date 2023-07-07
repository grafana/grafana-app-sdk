package k8s

import (
	"fmt"
	"net/http"
	"time"

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

// OpinionatedMutatingAdmissionController is a MutatingAdmissionController which wraps an optional user-defined
// Mutate() function with a set of additional PatchOperations which set metadata and label properties.
type OpinionatedMutatingAdmissionController struct {
	Underlying resource.MutatingAdmissionController
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
	if o.Underlying != nil {
		resp, err = o.Underlying.Mutate(request)
		if err != nil {
			return resp, err
		}
	}
	if resp == nil || resp.UpdatedObject == nil {
		resp = &resource.MutatingResponse{
			UpdatedObject: request.Object,
		}
	}

	// Get the CommonMetadata, so we can update it
	cmd := resp.UpdatedObject.CommonMetadata()

	if cmd.Labels == nil {
		cmd.Labels = make(map[string]string)
	}
	cmd.Labels[versionLabel] = request.Version

	// Operation-based changes
	switch request.Action {
	case resource.AdmissionActionCreate:
		cmd.CreatedBy = request.UserInfo.Username
		cmd.UpdateTimestamp = cmd.CreationTimestamp
	case resource.AdmissionActionUpdate:
		cmd.UpdatedBy = request.UserInfo.Username
		cmd.UpdateTimestamp = now()
	default:
		// Do nothing
	}
	resp.UpdatedObject.SetCommonMetadata(cmd)
	return resp, nil
}

// NewOpinionatedMutatingAdmissionController creates a pointer to a new OpinionatedMutatingAdmissionController wrapping the
// provided mutateFunc (nil mutateFunc argument is allowed, and will cause the controller to not call the underlying function)
func NewOpinionatedMutatingAdmissionController(wrap resource.MutatingAdmissionController) *OpinionatedMutatingAdmissionController {
	return &OpinionatedMutatingAdmissionController{
		Underlying: wrap,
	}
}

// OpinionatedValidatingAdmissionController implements resource.ValidatingAdmissionController and performs initial
// validation on reserved metadata fields which are stores as annotations in kubernetes, ensuring that if any changes are made,
// they are allowed, before calling the underlying admission validate function.
type OpinionatedValidatingAdmissionController struct {
	Underlying resource.ValidatingAdmissionController
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
	if o.Underlying != nil {
		return o.Underlying.Validate(request)
	}
	return nil
}

// NewOpinionatedValidatingAdmissionController returns a new OpinionatedValidatingAdmissionController which wraps the provided
// validateFunc. If validateFunc is nil, no extra validation after the opinionated initial validation will be performed.
func NewOpinionatedValidatingAdmissionController(wrap resource.ValidatingAdmissionController) *OpinionatedValidatingAdmissionController {
	return &OpinionatedValidatingAdmissionController{
		Underlying: wrap,
	}
}
