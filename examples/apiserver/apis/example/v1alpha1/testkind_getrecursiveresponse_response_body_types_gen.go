// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type VersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType struct {
	Message string                                                       `json:"message"`
	Next    *VersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType `json:"next,omitempty"`
}

// NewVersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType creates a new VersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType object.
func NewVersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType() *VersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType {
	return &VersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType{}
}

// +k8s:openapi-gen=true
type GetRecursiveResponseBody struct {
	Message string                                                       `json:"message"`
	Next    *VersionsV1alpha1Kinds0RoutesRecurseGETResponseRecursiveType `json:"next,omitempty"`
}

// NewGetRecursiveResponseBody creates a new GetRecursiveResponseBody object.
func NewGetRecursiveResponseBody() *GetRecursiveResponseBody {
	return &GetRecursiveResponseBody{}
}
