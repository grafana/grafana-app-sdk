package v1

// ExternalNameSpec defines model for ExternalNameSpec.
// +k8s:openapi-gen=true
type ExternalNameSpec struct {
	Host string `json:"host"`
}
