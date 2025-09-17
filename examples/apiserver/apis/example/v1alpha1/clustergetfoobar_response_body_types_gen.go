// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type VersionsV1alpha1RoutesClusterFoobarGETResponseExtra struct {
	Foo string `json:"foo"`
}

// NewVersionsV1alpha1RoutesClusterFoobarGETResponseExtra creates a new VersionsV1alpha1RoutesClusterFoobarGETResponseExtra object.
func NewVersionsV1alpha1RoutesClusterFoobarGETResponseExtra() *VersionsV1alpha1RoutesClusterFoobarGETResponseExtra {
	return &VersionsV1alpha1RoutesClusterFoobarGETResponseExtra{}
}

// +k8s:openapi-gen=true
type ClustergetfoobarBody struct {
	Bar   string                                                         `json:"bar"`
	Extra map[string]VersionsV1alpha1RoutesClusterFoobarGETResponseExtra `json:"extra"`
}

// NewClustergetfoobarBody creates a new ClustergetfoobarBody object.
func NewClustergetfoobarBody() *ClustergetfoobarBody {
	return &ClustergetfoobarBody{
		Extra: map[string]VersionsV1alpha1RoutesClusterFoobarGETResponseExtra{},
	}
}
