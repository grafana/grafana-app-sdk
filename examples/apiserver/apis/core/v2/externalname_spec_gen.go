package v2

// ExternalNameSpec defines model for ExternalNameSpec.
// +k8s:openapi-gen=true
type ExternalNameSpec struct {
	Host      string `json:"host"`
	OtherData string `json:"otherData"`
}
