// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha2

// +k8s:openapi-gen=true
type RouteBackendstatusOperatorState struct {
	// lastEvaluation is the ResourceVersion last evaluated
	LastEvaluation string `json:"lastEvaluation"`
	// state describes the state of the lastEvaluation.
	// It is limited to three possible states for machine evaluation.
	State RouteBackendStatusOperatorStateState `json:"state"`
	// descriptiveState is an optional more descriptive state field which has no requirements on format
	DescriptiveState *string `json:"descriptiveState,omitempty"`
	// details contains any extra information that is operator-specific
	Details map[string]interface{} `json:"details,omitempty"`
}

// NewRouteBackendstatusOperatorState creates a new RouteBackendstatusOperatorState object.
func NewRouteBackendstatusOperatorState() *RouteBackendstatusOperatorState {
	return &RouteBackendstatusOperatorState{}
}

// OpenAPIModelName returns the OpenAPI model name for RouteBackendstatusOperatorState.
func (RouteBackendstatusOperatorState) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendstatusOperatorState"
}

// +k8s:openapi-gen=true
type RouteBackendStatus struct {
	// operatorStates is a map of operator ID to operator state evaluations.
	// Any operator which consumes this kind SHOULD add its state evaluation information to this field.
	OperatorStates map[string]RouteBackendstatusOperatorState `json:"operatorStates,omitempty"`
	// additionalFields is reserved for future use
	AdditionalFields map[string]interface{} `json:"additionalFields,omitempty"`
}

// NewRouteBackendStatus creates a new RouteBackendStatus object.
func NewRouteBackendStatus() *RouteBackendStatus {
	return &RouteBackendStatus{}
}

// OpenAPIModelName returns the OpenAPI model name for RouteBackendStatus.
func (RouteBackendStatus) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendStatus"
}

// +k8s:openapi-gen=true
type RouteBackendStatusOperatorStateState string

const (
	RouteBackendStatusOperatorStateStateSuccess    RouteBackendStatusOperatorStateState = "success"
	RouteBackendStatusOperatorStateStateInProgress RouteBackendStatusOperatorStateState = "in_progress"
	RouteBackendStatusOperatorStateStateFailed     RouteBackendStatusOperatorStateState = "failed"
)

// OpenAPIModelName returns the OpenAPI model name for RouteBackendStatusOperatorStateState.
func (RouteBackendStatusOperatorStateState) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha2.RouteBackendStatusOperatorStateState"
}
