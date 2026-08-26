package appadapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/grafana/grafana-app-sdk/app"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/resource"
)

// Make sure AdmissionAdapter implements the service interface. This is important to
// do since otherwise we will only get a not implemented error response from
// the plugin at runtime.
var _ pluginv3.AdmissionServiceServer = (*AdmissionAdapter)(nil)

// AdmissionAdapter implements the v3 admission service in terms of an app-sdk
// App.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type AdmissionAdapter struct {
	pluginv3.UnimplementedAdmissionServiceServer

	app app.App
}

// NewAdmissionAdapter returns a [pluginv3.AdmissionServiceServer] backed by a.
func NewAdmissionAdapter(a app.App) *AdmissionAdapter {
	return &AdmissionAdapter{app: a}
}

// AdmissionReview implements [pluginv3.AdmissionServiceServer] by translating
// the request into an app.AdmissionRequest and delegating to the app-sdk
// App's Validate and Mutate.
func (a *AdmissionAdapter) AdmissionReview(ctx context.Context, req *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	gvk := req.GetKind()
	kind, ok := findManagedKind(a.app, gvk.GetGroup(), gvk.GetVersion(), gvk.GetKind())
	if !ok {
		return nil, fmt.Errorf("no managed kind for %s/%s %s", gvk.GetGroup(), gvk.GetVersion(), gvk.GetKind())
	}

	obj, err := decodeObject(kind, req.GetObjectBytes())
	if err != nil {
		return admissionErrorResponse(err), nil
	}
	oldObj, err := decodeObject(kind, req.GetOldObjectBytes())
	if err != nil {
		return admissionErrorResponse(err), nil
	}

	admissionReq := &app.AdmissionRequest{
		Action:    admissionAction(req.GetOperation()),
		Kind:      gvk.GetKind(),
		Group:     gvk.GetGroup(),
		Version:   gvk.GetVersion(),
		Object:    obj,
		OldObject: oldObj,
	}

	if err := a.app.Validate(ctx, admissionReq); err != nil && !errors.Is(err, app.ErrNotImplemented) {
		return admissionErrorResponse(err), nil
	}

	rsp := &pluginv3.AdmissionReviewResponse{}
	rsp.SetAllowed(true)

	mutated, err := a.app.Mutate(ctx, admissionReq)
	if err != nil && !errors.Is(err, app.ErrNotImplemented) {
		return admissionErrorResponse(err), nil
	}
	if mutated != nil && mutated.UpdatedObject != nil {
		objectBytes, err := encodeObject(kind, mutated.UpdatedObject)
		if err != nil {
			return admissionErrorResponse(err), nil
		}
		rsp.SetObjectBytes(objectBytes)
	}

	return rsp, nil
}

// findManagedKind looks up the resource.Kind managed by a that matches the
// given group, version, and kind.
func findManagedKind(a app.App, group, version, kind string) (resource.Kind, bool) {
	for _, k := range a.ManagedKinds() {
		if k.Group() == group && k.Version() == version && k.Kind() == kind {
			return k, true
		}
	}
	return resource.Kind{}, false
}

// decodeObject decodes raw into a resource.Object using kind's JSON codec.
// It returns nil if raw is empty.
func decodeObject(kind resource.Kind, raw []byte) (resource.Object, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	return kind.Read(bytes.NewReader(raw), resource.KindEncodingJSON)
}

// encodeObject encodes obj using kind's JSON codec.
func encodeObject(kind resource.Kind, obj resource.Object) ([]byte, error) {
	var buf bytes.Buffer
	if err := kind.Write(obj, &buf, resource.KindEncodingJSON); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// admissionAction maps the v3 wire operation to a resource.AdmissionAction.
func admissionAction(op pluginv3.AdmissionReviewRequest_Operation) resource.AdmissionAction {
	switch op {
	case pluginv3.AdmissionReviewRequest_OPERATION_CREATE:
		return resource.AdmissionActionCreate
	case pluginv3.AdmissionReviewRequest_OPERATION_UPDATE:
		return resource.AdmissionActionUpdate
	case pluginv3.AdmissionReviewRequest_OPERATION_DELETE:
		return resource.AdmissionActionDelete
	default:
		return ""
	}
}

// admissionErrorResponse builds a denying AdmissionReviewResponse carrying
// err's message as the failure status.
func admissionErrorResponse(err error) *pluginv3.AdmissionReviewResponse {
	status := &pluginv3.StatusResult{}
	status.SetStatus("Failure")
	status.SetMessage(err.Error())

	rsp := &pluginv3.AdmissionReviewResponse{}
	rsp.SetAllowed(false)
	rsp.SetError(status)
	return rsp
}
