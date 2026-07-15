// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha3

// +k8s:openapi-gen=true
type AppRoutestatusOperatorState struct {
	// lastEvaluation is the ResourceVersion last evaluated
	LastEvaluation string `json:"lastEvaluation"`
	// state describes the state of the lastEvaluation.
	// It is limited to three possible states for machine evaluation.
	State AppRouteStatusOperatorStateState `json:"state"`
	// descriptiveState is an optional more descriptive state field which has no requirements on format
	DescriptiveState *string `json:"descriptiveState,omitempty"`
	// details contains any extra information that is operator-specific
	Details map[string]interface{} `json:"details,omitempty"`
}

// NewAppRoutestatusOperatorState creates a new AppRoutestatusOperatorState object.
func NewAppRoutestatusOperatorState() *AppRoutestatusOperatorState {
	return &AppRoutestatusOperatorState{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRoutestatusOperatorState.
func (AppRoutestatusOperatorState) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRoutestatusOperatorState"
}

// +k8s:openapi-gen=true
type AppRouteStatus struct {
	// operatorStates is a map of operator ID to operator state evaluations.
	// Any operator which consumes this kind SHOULD add its state evaluation information to this field.
	OperatorStates map[string]AppRoutestatusOperatorState `json:"operatorStates,omitempty"`
	// additionalFields is reserved for future use
	AdditionalFields map[string]interface{} `json:"additionalFields,omitempty"`
}

// NewAppRouteStatus creates a new AppRouteStatus object.
func NewAppRouteStatus() *AppRouteStatus {
	return &AppRouteStatus{}
}

// OpenAPIModelName returns the OpenAPI model name for AppRouteStatus.
func (AppRouteStatus) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteStatus"
}

// +k8s:openapi-gen=true
type AppRouteStatusOperatorStateState string

const (
	AppRouteStatusOperatorStateStateSuccess    AppRouteStatusOperatorStateState = "success"
	AppRouteStatusOperatorStateStateInProgress AppRouteStatusOperatorStateState = "in_progress"
	AppRouteStatusOperatorStateStateFailed     AppRouteStatusOperatorStateState = "failed"
)

// OpenAPIModelName returns the OpenAPI model name for AppRouteStatusOperatorStateState.
func (AppRouteStatusOperatorStateState) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.AppRouteStatusOperatorStateState"
}
