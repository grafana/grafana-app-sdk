package v2

// Defines values for OperatorStateState.
const (
	OperatorStateStateFailed     OperatorStateState = "failed"
	OperatorStateStateInProgress OperatorStateState = "in_progress"
	OperatorStateStateSuccess    OperatorStateState = "success"
)

// Defines values for StatusOperatorStateState.
const (
	StatusOperatorStateStateFailed     StatusOperatorStateState = "failed"
	StatusOperatorStateStateInProgress StatusOperatorStateState = "in_progress"
	StatusOperatorStateStateSuccess    StatusOperatorStateState = "success"
)

// OperatorState defines model for OperatorState.
type OperatorState struct {
	// descriptiveState is an optional more descriptive state field which has no requirements on format
	DescriptiveState *string `json:"descriptiveState,omitempty"`

	// details contains any extra information that is operator-specific
	Details map[string]interface{} `json:"details,omitempty"`

	// lastEvaluation is the ResourceVersion last evaluated
	LastEvaluation string `json:"lastEvaluation"`

	// state describes the state of the lastEvaluation.
	// It is limited to three possible states for machine evaluation.
	State OperatorStateState `json:"state"`
}

// OperatorStateState state describes the state of the lastEvaluation.
// It is limited to three possible states for machine evaluation.
type OperatorStateState string

// Status defines model for Status.
type Status struct {
	// additionalFields is reserved for future use
	AdditionalFields map[string]interface{} `json:"additionalFields,omitempty"`

	// operatorStates is a map of operator ID to operator state evaluations.
	// Any operator which consumes this kind SHOULD add its state evaluation information to this field.
	OperatorStates map[string]StatusOperatorState `json:"operatorStates,omitempty"`
}

// StatusOperatorState defines model for status.#OperatorState.
type StatusOperatorState struct {
	// descriptiveState is an optional more descriptive state field which has no requirements on format
	DescriptiveState *string `json:"descriptiveState,omitempty"`

	// details contains any extra information that is operator-specific
	Details map[string]interface{} `json:"details,omitempty"`

	// lastEvaluation is the ResourceVersion last evaluated
	LastEvaluation string `json:"lastEvaluation"`

	// state describes the state of the lastEvaluation.
	// It is limited to three possible states for machine evaluation.
	State StatusOperatorStateState `json:"state"`
}

// StatusOperatorStateState state describes the state of the lastEvaluation.
// It is limited to three possible states for machine evaluation.
type StatusOperatorStateState string
