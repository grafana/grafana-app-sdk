// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type GetMessageBody struct {
	Message string `json:"message"`
}

// NewGetMessageBody creates a new GetMessageBody object.
func NewGetMessageBody() *GetMessageBody {
	return &GetMessageBody{}
}
func (GetMessageBody) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.examples.apiserver.apis.example.v1alpha1.GetMessageBody"
}
