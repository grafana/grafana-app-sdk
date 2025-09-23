// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v0alpha1

// +k8s:openapi-gen=true
type TestKindstatusOperatorState struct {
	// lastEvaluation is the ResourceVersion last evaluated
	LastEvaluation string `json:"lastEvaluation"`
	// state describes the state of the lastEvaluation.
	// It is limited to three possible states for machine evaluation.
	State TestKindStatusOperatorStateState `json:"state"`
	// descriptiveState is an optional more descriptive state field which has no requirements on format
	DescriptiveState *string `json:"descriptiveState,omitempty"`
	// details contains any extra information that is operator-specific
	Details map[string]interface{} `json:"details,omitempty"`
}

// NewTestKindstatusOperatorState creates a new TestKindstatusOperatorState object.
func NewTestKindstatusOperatorState() *TestKindstatusOperatorState {
	return &TestKindstatusOperatorState{}
}

// +k8s:openapi-gen=true
type TestKindStatus struct {
	// operatorStates is a map of operator ID to operator state evaluations.
	// Any operator which consumes this kind SHOULD add its state evaluation information to this field.
	OperatorStates map[string]TestKindstatusOperatorState `json:"operatorStates,omitempty"`
	// additionalFields is reserved for future use
	AdditionalFields map[string]interface{} `json:"additionalFields,omitempty"`
}

// NewTestKindStatus creates a new TestKindStatus object.
func NewTestKindStatus() *TestKindStatus {
	return &TestKindStatus{}
}

// +k8s:openapi-gen=true
type TestKindStatusOperatorStateState string

const (
	TestKindStatusOperatorStateStateSuccess    TestKindStatusOperatorStateState = "success"
	TestKindStatusOperatorStateStateInProgress TestKindStatusOperatorStateState = "in_progress"
	TestKindStatusOperatorStateStateFailed     TestKindStatusOperatorStateState = "failed"
)
