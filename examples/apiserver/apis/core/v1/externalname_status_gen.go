package v1

// Defines values for ExternalNameOperatorStateState.
const (
	ExternalNameOperatorStateStateFailed     ExternalNameOperatorStateState = "failed"
	ExternalNameOperatorStateStateInProgress ExternalNameOperatorStateState = "in_progress"
	ExternalNameOperatorStateStateSuccess    ExternalNameOperatorStateState = "success"
)

// Defines values for ExternalNamestatusOperatorStateState.
const (
	ExternalNamestatusOperatorStateStateFailed     ExternalNamestatusOperatorStateState = "failed"
	ExternalNamestatusOperatorStateStateInProgress ExternalNamestatusOperatorStateState = "in_progress"
	ExternalNamestatusOperatorStateStateSuccess    ExternalNamestatusOperatorStateState = "success"
)

// ExternalNameOperatorState defines model for ExternalNameOperatorState.
// +k8s:openapi-gen=true
type ExternalNameOperatorState struct {
	// descriptiveState is an optional more descriptive state field which has no requirements on format
	DescriptiveState *string `json:"descriptiveState,omitempty"`

	// details contains any extra information that is operator-specific
	Details map[string]interface{} `json:"details,omitempty"`

	// lastEvaluation is the ResourceVersion last evaluated
	LastEvaluation string `json:"lastEvaluation"`

	// state describes the state of the lastEvaluation.
	// It is limited to three possible states for machine evaluation.
	State ExternalNameOperatorStateState `json:"state"`
}

// ExternalNameOperatorStateState state describes the state of the lastEvaluation.
// It is limited to three possible states for machine evaluation.
// +k8s:openapi-gen=true
type ExternalNameOperatorStateState string

// ExternalNameStatus defines model for ExternalNameStatus.
// +k8s:openapi-gen=true
type ExternalNameStatus struct {
	// additionalFields is reserved for future use
	AdditionalFields map[string]interface{} `json:"additionalFields,omitempty"`

	// operatorStates is a map of operator ID to operator state evaluations.
	// Any operator which consumes this kind SHOULD add its state evaluation information to this field.
	OperatorStates map[string]ExternalNamestatusOperatorState `json:"operatorStates,omitempty"`
}

// ExternalNamestatusOperatorState defines model for ExternalNamestatus.#OperatorState.
// +k8s:openapi-gen=true
type ExternalNamestatusOperatorState struct {
	// descriptiveState is an optional more descriptive state field which has no requirements on format
	DescriptiveState *string `json:"descriptiveState,omitempty"`

	// details contains any extra information that is operator-specific
	Details map[string]interface{} `json:"details,omitempty"`

	// lastEvaluation is the ResourceVersion last evaluated
	LastEvaluation string `json:"lastEvaluation"`

	// state describes the state of the lastEvaluation.
	// It is limited to three possible states for machine evaluation.
	State ExternalNamestatusOperatorStateState `json:"state"`
}

// ExternalNamestatusOperatorStateState state describes the state of the lastEvaluation.
// It is limited to three possible states for machine evaluation.
// +k8s:openapi-gen=true
type ExternalNamestatusOperatorStateState string
