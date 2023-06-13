package resource

type AdmissionAction string

const (
	AdmissionActionCreate  = AdmissionAction("CREATE")
	AdmissionActionUpdate  = AdmissionAction("UPDATE")
	AdmissionActionDelete  = AdmissionAction("DELETE")
	AdmissionActionConnect = AdmissionAction("CONNECT")
)

// AdmissionRequest contains information from a kubernetes Admission request and decoded object(s).
type AdmissionRequest struct {
	// Action is the type of request being checked for admission
	Action AdmissionAction
	// Kind is the object's kind
	Kind string
	// Group is the object's group
	Group string
	// Version is the object's version
	Version string
	// UserInfo is user information about the user making the request
	UserInfo AdmissionUserInfo
	// Object is the object in the request
	Object Object
	// OldObject is the object as it currently exists in storage
	OldObject Object
}

// AdmissionUserInfo contains user information for an admission request
type AdmissionUserInfo struct {
	// Username is the username of the user
	Username string
	// UID is the UID of the user in the API server's system
	UID string
	// Groups is a list of all groups the user is a part of (if any)
	Groups []string
	// Extra is a map of extra information, implementation-specific
	Extra map[string]any
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
	PatchOperations []PatchOperation
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
