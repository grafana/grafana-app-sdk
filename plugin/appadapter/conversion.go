package appadapter

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/app"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/resource"
)

// Make sure ConversionAdapter implements the service interface. This is important to
// do since otherwise we will only get a not implemented error response from
// the plugin at runtime.
var _ pluginv3.ConversionServiceServer = (*ConversionAdapter)(nil)

// ConversionAdapter implements the v3 conversion service in terms of an
// app-sdk App.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type ConversionAdapter struct {
	pluginv3.UnimplementedConversionServiceServer

	app app.App
}

// NewConversionAdapter returns a [pluginv3.ConversionServiceServer] backed by a.
func NewConversionAdapter(a app.App) *ConversionAdapter {
	return &ConversionAdapter{app: a}
}

// ConvertObjects implements [pluginv3.ConversionServiceServer] by translating
// each object into an app.ConversionRequest and delegating to the app-sdk
// App's Convert. Conversion stops at the first object that fails to convert.
func (a *ConversionAdapter) ConvertObjects(ctx context.Context, req *pluginv3.ConvertObjectsRequest) (*pluginv3.ConvertObjectsResponse, error) {
	rsp := &pluginv3.ConvertObjectsResponse{}
	rsp.SetUid(req.GetUid())

	targetVersion := req.GetTargetVersion()
	converted := make([]*pluginv3.ConvertObjectsResponse_Object, 0, len(req.GetObjects()))
	for _, obj := range req.GetObjects() {
		gvk := obj.GetGvk()
		convReq := app.ConversionRequest{
			SourceGVK: schema.GroupVersionKind{
				Group:   gvk.GetGroup(),
				Version: gvk.GetVersion(),
				Kind:    gvk.GetKind(),
			},
			TargetGVK: schema.GroupVersionKind{
				Group:   gvk.GetGroup(),
				Version: targetVersion,
				Kind:    gvk.GetKind(),
			},
			Raw: app.RawObject{
				Raw:      obj.GetRaw(),
				Encoding: resource.KindEncodingJSON,
			},
		}

		result, err := a.app.Convert(ctx, convReq)
		if err != nil {
			rsp.SetError(conversionErrorStatus(err))
			rsp.SetConverted(nil)
			return rsp, nil
		}

		item := &pluginv3.ConvertObjectsResponse_Object{}
		item.SetRaw(result.Raw)
		converted = append(converted, item)
	}

	rsp.SetConverted(converted)
	return rsp, nil
}

// conversionErrorStatus builds a StatusResult carrying err's message as the
// conversion failure status.
func conversionErrorStatus(err error) *pluginv3.StatusResult {
	status := &pluginv3.StatusResult{}
	status.SetStatus("Failure")
	status.SetMessage(err.Error())
	return status
}
