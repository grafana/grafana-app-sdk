// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:openapi-gen=true
type GetMessageBody struct {
	Message string `json:"message"`
}

// NewGetMessageBody creates a new GetMessageBody object.
func NewGetMessageBody() *GetMessageBody {
	return &GetMessageBody{}
}

type GetMessage struct {
	metav1.TypeMeta `json:",inline"`
	GetMessageBody  `json:",inline"`
}

func NewGetMessage() *GetMessage {
	return &GetMessage{}
}
