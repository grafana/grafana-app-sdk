package plugin

import (
	"context"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// AdmissionReview implements [pluginv3.AdmissionServiceServer].
func (*App) AdmissionReview(_ context.Context, _ *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	rsp := &pluginv3.AdmissionReviewResponse{}
	rsp.SetAllowed(true)
	rsp.SetWarnings([]string{"warning 1", "warning 2"})
	return rsp, nil
}

// ConvertObjects implements [pluginv3.ConversionServiceServer].
func (*App) ConvertObjects(_ context.Context, req *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	rsp := &pluginv3.ConvertObjectsResponse{}
	rsp.SetUid(req.GetUid())

	converted := make([]*pluginv3.ConvertObjectsResponse_Object, len(req.GetObjects()))
	for i, obj := range req.GetObjects() {
		tmp := &pluginv3.ConvertObjectsResponse_Object{}
		tmp.SetRaw(obj.GetRaw())
		tmp.SetWarnings([]string{"noop for now"})
		converted[i] = tmp
	}
	rsp.SetConverted(converted)
	return rsp, nil
}
