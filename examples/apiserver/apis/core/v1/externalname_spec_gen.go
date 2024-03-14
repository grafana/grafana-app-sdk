package v1

// Spec defines model for Spec.
// +k8s:openapi-gen=true
type Spec struct {
	Host string `json:"host"`
}
