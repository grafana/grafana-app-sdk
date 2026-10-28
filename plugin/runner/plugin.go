package runner

import (
	"bytes"
	"context"
	"errors"
	"net/http"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

var (
	_ backend.ConversionHandler   = (*PluginRunner)(nil)
	_ backend.AdmissionHandler    = (*PluginRunner)(nil)
	_ backend.QueryDataHandler    = (*PluginRunner)(nil)
	_ backend.CheckHealthHandler  = (*PluginRunner)(nil)
	_ backend.CallResourceHandler = (*PluginRunner)(nil)
)

func NewPluginRunner(app app.App) *PluginRunner {
	return &PluginRunner{
		app:   app,
		codec: resource.NewJSONCodec(),
	}
}

type PluginRunner struct {
	app   app.App
	codec *resource.JSONCodec
}

func (r *PluginRunner) Run(ctx context.Context) error {
	<-ctx.Done()
	if ctx.Err() == context.Canceled {
		return nil
	}
	return ctx.Err()
}

func (r *PluginRunner) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return nil, errors.New("not implemented")
}

func (r *PluginRunner) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	return &backend.CheckHealthResult{
		Status: backend.HealthStatusOk,
	}, nil
}

func (r *PluginRunner) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	customReq := &app.ResourceCustomRouteRequest{
		// TODO: why is this needed?
		// ResourceIdentifier: resource.FullIdentifier{},
		SubresourcePath: req.Path,
		Method:          req.Method,
		Headers:         req.Headers,
		Body:            req.Body,
	}

	res, err := r.app.CallResourceCustomRoute(ctx, customReq)
	if err != nil {
		return err
	}

	return sender.Send(&backend.CallResourceResponse{
		Status:  res.StatusCode,
		Headers: res.Headers,
		Body:    res.Body,
	})
}

func (r *PluginRunner) MutateAdmission(ctx context.Context, req *backend.AdmissionRequest) (*backend.MutationResponse, error) {
	res := &backend.MutationResponse{
		Allowed: false,
		Result: &backend.StatusResult{
			Status:  "Failure",
			Message: "",
			Reason:  "",
			Code:    http.StatusBadRequest,
		},
		Warnings:    []string{},
		ObjectBytes: []byte{},
	}
	admissionReq, err := r.translateAdmissionRequest(req)
	if err != nil {
		res.Result.Message = err.Error()
		return res, nil
	}

	mutatingResponse, err := r.app.Mutate(ctx, admissionReq)
	if err != nil {
		res.Result.Message = err.Error()
		return res, nil
	}

	raw := bytes.NewBuffer([]byte{})
	if err := r.codec.Write(raw, mutatingResponse.UpdatedObject); err != nil {
		res.Result.Message = err.Error()
		return res, nil
	}

	res.Allowed = true
	res.Result.Status = "Success"
	res.Result.Code = http.StatusOK
	res.ObjectBytes = raw.Bytes()
	return res, nil
}

func (r *PluginRunner) ValidateAdmission(ctx context.Context, req *backend.AdmissionRequest) (*backend.ValidationResponse, error) {
	admissionReq, err := r.translateAdmissionRequest(req)
	if err != nil {
		return nil, err
	}

	err = r.app.Validate(ctx, admissionReq)
	code := http.StatusBadRequest
	statusMessage := "Failure"
	errorMessage := ""
	if err == nil {
		statusMessage = "Success"
		code = http.StatusOK
	} else {
		errorMessage = err.Error()
	}

	status := backend.StatusResult{
		Status: statusMessage,
		Reason: errorMessage,
		Code:   int32(code),
	}

	return &backend.ValidationResponse{
		Allowed: err == nil,
		Result:  &status,
	}, nil
}

func (r *PluginRunner) ConvertObjects(ctx context.Context, req *backend.ConversionRequest) (*backend.ConversionResponse, error) {
	return nil, errors.New("not implemented")
}

func (r *PluginRunner) translateAdmissionRequest(req *backend.AdmissionRequest) (*app.AdmissionRequest, error) {
	var action resource.AdmissionAction

	switch req.Operation {
	case backend.AdmissionRequestCreate:
		action = resource.AdmissionActionCreate
	case backend.AdmissionRequestUpdate:
		action = resource.AdmissionActionUpdate
	case backend.AdmissionRequestDelete:
		action = resource.AdmissionActionDelete
	}

	var newObj resource.Object
	var oldObj resource.Object

	if err := r.codec.Read(bytes.NewReader(req.ObjectBytes), newObj); err != nil {
		return nil, err
	}

	if req.OldObjectBytes != nil {
		if err := r.codec.Read(bytes.NewReader(req.OldObjectBytes), oldObj); err != nil {
			return nil, err
		}
	}

	return &app.AdmissionRequest{
		Action:    action,
		Object:    newObj,
		OldObject: oldObj,
		Kind:      req.Kind.Kind,
		Group:     req.Kind.Group,
		Version:   req.Kind.Version,
	}, nil
}
