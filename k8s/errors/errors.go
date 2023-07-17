package k8s

// NewServerResponseError creates a new instance of ServerResponseError
func NewServerResponseError(err error, statusCode int) *ServerResponseError {
	return &ServerResponseError{
		err:        err,
		statusCode: statusCode,
	}
}

// ServerResponseError represents an HTTP error from the kubernetes control plane.
// It contains the underlying error returned by the kubernetes go client, and the status code returned from the API.
type ServerResponseError struct {
	err        error
	statusCode int
}

// Error returns the Error() of the underlying kubernetes client error
func (s *ServerResponseError) Error() string {
	return s.err.Error()
}

// StatusCode returns the status code returned by the kubernetes API associated with this error
func (s *ServerResponseError) StatusCode() int {
	return s.statusCode
}

// Unwrap returns the underlying kubernetes go client error
func (s *ServerResponseError) Unwrap() error {
	return s.err
}
