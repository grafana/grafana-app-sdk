// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha3

// +k8s:openapi-gen=true
type RuntimeConfigstatusOperatorState struct {
	// lastEvaluation is the ResourceVersion last evaluated
	LastEvaluation string `json:"lastEvaluation"`
	// state describes the state of the lastEvaluation.
	// It is limited to three possible states for machine evaluation.
	State RuntimeConfigStatusOperatorStateState `json:"state"`
	// descriptiveState is an optional more descriptive state field which has no requirements on format
	DescriptiveState *string `json:"descriptiveState,omitempty"`
	// details contains any extra information that is operator-specific
	Details map[string]interface{} `json:"details,omitempty"`
}

// NewRuntimeConfigstatusOperatorState creates a new RuntimeConfigstatusOperatorState object.
func NewRuntimeConfigstatusOperatorState() *RuntimeConfigstatusOperatorState {
	return &RuntimeConfigstatusOperatorState{}
}

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigstatusOperatorState.
func (RuntimeConfigstatusOperatorState) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigstatusOperatorState"
}

// +k8s:openapi-gen=true
type RuntimeConfigStatus struct {
	// operatorStates is a map of operator ID to operator state evaluations.
	// Any operator which consumes this kind SHOULD add its state evaluation information to this field.
	OperatorStates map[string]RuntimeConfigstatusOperatorState `json:"operatorStates,omitempty"`
	// additionalFields is reserved for future use
	AdditionalFields map[string]interface{} `json:"additionalFields,omitempty"`
}

// NewRuntimeConfigStatus creates a new RuntimeConfigStatus object.
func NewRuntimeConfigStatus() *RuntimeConfigStatus {
	return &RuntimeConfigStatus{}
}

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigStatus.
func (RuntimeConfigStatus) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigStatus"
}

// +k8s:openapi-gen=true
type RuntimeConfigStatusOperatorStateState string

const (
	RuntimeConfigStatusOperatorStateStateSuccess    RuntimeConfigStatusOperatorStateState = "success"
	RuntimeConfigStatusOperatorStateStateInProgress RuntimeConfigStatusOperatorStateState = "in_progress"
	RuntimeConfigStatusOperatorStateStateFailed     RuntimeConfigStatusOperatorStateState = "failed"
)

// OpenAPIModelName returns the OpenAPI model name for RuntimeConfigStatusOperatorStateState.
func (RuntimeConfigStatusOperatorStateState) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.app.appmanifest.v1alpha3.RuntimeConfigStatusOperatorStateState"
}
