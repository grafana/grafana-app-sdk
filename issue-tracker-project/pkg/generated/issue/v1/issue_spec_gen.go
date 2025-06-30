// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1

// spec is the schema of our resource.
// We could include `status` or `metadata` top-level fields here as well,
// but `status` is for state information, which we don't need to track,
// and `metadata` is for kind/schema-specific custom metadata in addition to the existing
// common metadata, and we don't need to track any specific custom metadata.
// +k8s:openapi-gen=true
type Spec struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// NewSpec creates a new Spec object.
func NewSpec() *Spec {
	return &Spec{}
}
