// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type AppManifeststatusApplyStatus struct {
	Status  AppManifestStatusApplyStatusStatus `json:"status"`
	Details *string                            `json:"details,omitempty"`
}

// NewAppManifeststatusApplyStatus creates a new AppManifeststatusApplyStatus object.
func NewAppManifeststatusApplyStatus() *AppManifeststatusApplyStatus {
	return &AppManifeststatusApplyStatus{}
}

// +k8s:openapi-gen=true
type AppManifeststatusOperatorState struct {
	// lastEvaluation is the ResourceVersion last evaluated
	LastEvaluation string `json:"lastEvaluation"`
	// state describes the state of the lastEvaluation.
	// It is limited to three possible states for machine evaluation.
	State AppManifestStatusOperatorStateState `json:"state"`
	// descriptiveState is an optional more descriptive state field which has no requirements on format
	DescriptiveState *string `json:"descriptiveState,omitempty"`
	// details contains any extra information that is operator-specific
	Details map[string]interface{} `json:"details,omitempty"`
}

// NewAppManifeststatusOperatorState creates a new AppManifeststatusOperatorState object.
func NewAppManifeststatusOperatorState() *AppManifeststatusOperatorState {
	return &AppManifeststatusOperatorState{}
}

// +k8s:openapi-gen=true
type AppManifestStatus struct {
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
	// operatorStates is a map of operator ID to operator state evaluations.
	// Any operator which consumes this kind SHOULD add its state evaluation information to this field.
	OperatorStates map[string]AppManifeststatusOperatorState `json:"operatorStates,omitempty"`
	Resources      map[string]AppManifeststatusApplyStatus   `json:"resources"`
	// additionalFields is reserved for future use
	AdditionalFields map[string]interface{} `json:"additionalFields,omitempty"`
}

// NewAppManifestStatus creates a new AppManifestStatus object.
func NewAppManifestStatus() *AppManifestStatus {
	return &AppManifestStatus{}
}

// +k8s:openapi-gen=true
type AppManifestStatusApplyStatusStatus string

const (
	AppManifestStatusApplyStatusStatusSuccess AppManifestStatusApplyStatusStatus = "success"
	AppManifestStatusApplyStatusStatusFailure AppManifestStatusApplyStatusStatus = "failure"
)

// +k8s:openapi-gen=true
type AppManifestStatusOperatorStateState string

const (
	AppManifestStatusOperatorStateStateSuccess    AppManifestStatusOperatorStateState = "success"
	AppManifestStatusOperatorStateStateInProgress AppManifestStatusOperatorStateState = "in_progress"
	AppManifestStatusOperatorStateStateFailed     AppManifestStatusOperatorStateState = "failed"
)
