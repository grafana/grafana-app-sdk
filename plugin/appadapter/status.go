package appadapter

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// toStatusResult converts err into a [pluginv3.StatusResult]. Errors which
// implement [apierrors.APIStatus] (such as those returned by the k8s
// apimachinery error helpers) keep their reason, code, and details so that
// callers get a structured failure rather than an opaque message.
func toStatusResult(err error) *pluginv3.StatusResult {
	r := &pluginv3.StatusResult{}
	r.SetStatus("Failure")

	var apiStatus apierrors.APIStatus
	if !errors.As(err, &apiStatus) {
		r.SetMessage(err.Error())
		return r
	}

	status := apiStatus.Status()
	r.SetStatus(status.Status)
	r.SetMessage(status.Message)
	r.SetReason(string(status.Reason))
	r.SetCode(status.Code)
	if status.Details != nil {
		details := &pluginv3.StatusDetails{}
		details.SetName(status.Details.Name)
		details.SetGroup(status.Details.Group)
		details.SetKind(status.Details.Kind)
		details.SetUid(string(status.Details.UID))
		details.SetRetryAfterSeconds(status.Details.RetryAfterSeconds)

		causes := make([]*pluginv3.StatusCause, len(status.Details.Causes))
		for i, statusCause := range status.Details.Causes {
			cause := &pluginv3.StatusCause{}
			cause.SetReason(string(statusCause.Type))
			cause.SetMessage(statusCause.Message)
			cause.SetField(statusCause.Field)
			causes[i] = cause
		}
		details.SetCauses(causes)
		r.SetDetails(details)
	}
	return r
}
