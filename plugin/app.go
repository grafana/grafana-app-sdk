package plugin

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/tracing"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	_ backend.AdmissionHandler = &App{}
)

type App struct {
	provider       resource.AppProvider
	app            resource.App
	initialized    bool
	initializedMux sync.RWMutex
	runnerStopCh   chan struct{}
	gvkKinds       map[string]resource.Kind
	gvrKinds       map[string]resource.Kind
}

type Options struct {
	GRPCSettings backend.GRPCSettings
	TracingOpts  tracing.Opts
}

func (a *App) Run(pluginID string, opts Options) error {
	return app.Manage(pluginID, a.instanceFactory, app.ManageOpts{
		GRPCSettings: opts.GRPCSettings,
		TracingOpts:  opts.TracingOpts,
	})
}

func (a *App) ValidateAdmission(ctx context.Context, req *backend.AdmissionRequest) (*backend.ValidationResponse, error) {
	if !a.isInitialized() {
		return nil, fmt.Errorf("app is not initialized")
	}
	validator, ok := a.app.(resource.ValidatorApp)
	if !ok {
		return &backend.ValidationResponse{
			Allowed: true,
		}, nil
	}
	adm, _, err := a.pluginAdmissionRequestToResourceAdmissionRequest(req)
	if err != nil {
		return nil, err
	}
	err = validator.Validate(ctx, adm)
	if err != nil {
		return &backend.ValidationResponse{
			Allowed: false,
			Result: &backend.StatusResult{
				Message: err.Error(),
			},
		}, nil
	}
	return &backend.ValidationResponse{
		Allowed: true,
	}, nil
}
func (a *App) MutateAdmission(ctx context.Context, req *backend.AdmissionRequest) (*backend.MutationResponse, error) {
	if !a.isInitialized() {
		return nil, fmt.Errorf("app is not initialized")
	}
	mutator, ok := a.app.(resource.MutatorApp)
	if !ok {
		return &backend.MutationResponse{
			Allowed: true,
		}, nil
	}
	adm, kind, err := a.pluginAdmissionRequestToResourceAdmissionRequest(req)
	if err != nil {
		return nil, err
	}
	resp, err := mutator.Mutate(ctx, adm)
	if err != nil {
		return &backend.MutationResponse{
			Allowed: false,
			Result: &backend.StatusResult{
				Message: err.Error(),
			},
		}, nil
	}
	var codec resource.Codec = resource.NewJSONCodec()
	if kind != nil {
		if c := kind.Codec(resource.KindEncodingJSON); c != nil {
			codec = c
		}
	}
	buf := bytes.Buffer{}
	err = codec.Write(&buf, resp.UpdatedObject)
	if err != nil {
		return &backend.MutationResponse{
			Allowed: false,
			Result: &backend.StatusResult{
				Message: err.Error(),
			},
		}, nil
	}
	return &backend.MutationResponse{
		Allowed:     true,
		ObjectBytes: buf.Bytes(),
	}, nil
}
func (a *App) ConvertObject(ctx context.Context, req *backend.ConversionRequest) (*backend.ConversionResponse, error) {
	if !a.isInitialized() {
		return nil, fmt.Errorf("app is not initialized")
	}
	convReq := resource.ConversionRequest{
		SourceGVK: schema.GroupVersionKind{
			Group:   req.Kind.Group,
			Version: req.Kind.Version,
			Kind:    req.Kind.Kind,
		},
		TargetGVK: schema.GroupVersionKind{
			Group:   req.Kind.Group,
			Version: req.TargetVersion,
			Kind:    req.Kind.Kind,
		},
		Raw: resource.RawObject{
			Raw:      req.ObjectBytes,
			Encoding: resource.KindEncodingJSON,
		},
	}
	converter, ok := a.app.(resource.ConverterApp)
	if !ok {
		// Do a basic conversion
		dstKind, ok := a.gvkKinds[convReq.TargetGVK.String()]
		if !ok {
			return &backend.ConversionResponse{
				Allowed: false,
				Result: &backend.StatusResult{
					Message: "invalid target GroupVersionKind",
				},
			}, nil
		}
		obj, err := dstKind.Read(bytes.NewReader(req.ObjectBytes), resource.KindEncodingJSON)
		if err != nil {
			return &backend.ConversionResponse{
				Allowed: false,
				Result: &backend.StatusResult{
					Message: err.Error(),
				},
			}, nil
		}
		obj.SetGroupVersionKind(dstKind.GroupVersionKind())
		buf := bytes.Buffer{}
		err = dstKind.Write(obj, &buf, resource.KindEncodingJSON)
		if err != nil {
			return &backend.ConversionResponse{
				Allowed: false,
				Result: &backend.StatusResult{
					Message: err.Error(),
				},
			}, nil
		}
		return &backend.ConversionResponse{
			Allowed:     true,
			ObjectBytes: buf.Bytes(),
		}, nil
	}
	converted, err := converter.Convert(ctx, convReq)
	if err != nil {
		return nil, err
	}
	return &backend.ConversionResponse{
		Allowed:     true,
		ObjectBytes: converted.Raw,
	}, nil
}

func (a *App) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if !a.isInitialized() {
		return fmt.Errorf("app is not initialized")
	}

	caller, ok := a.app.(resource.SubresourceApp)
	if !ok {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusNotFound,
		})
	}

	segments := strings.Split(req.Path, "/")
	if len(segments) < 5 {
		// TODO: check if it's a webhook call routed here?
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusNotFound,
		})
	}
	id := resource.FullIdentifier{
		Group:   segments[0],
		Version: segments[1],
	}
	path := ""
	if segments[2] == "namespaces" {
		id.Namespace = segments[3]
		id.Plural = segments[4]
		id.Name = segments[5]
		path = segments[6]
	} else {
		id.Plural = segments[2]
		id.Name = segments[3]
		path = segments[4]
	}
	kind, ok := a.gvrKinds[id.Plural]
	if !ok {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusNotFound,
		})
	}
	id.Kind = kind.Kind()

	w := responseWriter{
		status:  http.StatusOK,
		headers: make(http.Header),
	}

	err := caller.CallSubresource(ctx, &w, &resource.SubresourceRequest{
		ResourceIdentifier: id,
		SubresourcePath:    path,
		Method:             req.Method,
		Headers:            req.Headers,
		Body:               req.Body,
	})
	if err != nil {
		return err
	}

	return sender.Send(w.CallResourceResponse())
}

// TODO: need to map plugin context into a non-plugin version
//func (a *App) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
//
//}

func (a *App) instanceFactory(ctx context.Context, settings backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	// For now, we have to hard-code the kubeconfig loading as it's not provided in settings
	a.initializedMux.Lock()
	defer a.initializedMux.Unlock()
	app, err := a.provider.NewApp(resource.AppConfig{})
	if err != nil {
		return nil, err
	}
	a.app = app
	for _, kind := range a.app.ManagedKinds() {
		a.gvkKinds[kind.GroupVersionKind().String()] = kind
		a.gvrKinds[kind.GroupVersionResource().String()] = kind
	}
	a.initialized = true
	a.runnerStopCh = make(chan struct{})
	runner := a.app.Runner()
	if runner != nil {
		go runner.Run(a.runnerStopCh)
	}
	return a, nil
}

func (a *App) isInitialized() bool {
	a.initializedMux.RLock()
	defer a.initializedMux.RUnlock()
	return a.initialized
}

func (a *App) pluginAdmissionRequestToResourceAdmissionRequest(req *backend.AdmissionRequest) (*resource.AdmissionRequest, *resource.Kind, error) {
	var op resource.AdmissionAction
	switch req.Operation {
	case backend.AdmissionRequestCreate:
		op = resource.AdmissionActionCreate
	case backend.AdmissionRequestUpdate:
		op = resource.AdmissionActionUpdate
	case backend.AdmissionRequestDelete:
		op = resource.AdmissionActionDelete
	}
	kind, ok := a.gvkKinds[schema.GroupVersionKind{
		Group:   req.Kind.Group,
		Version: req.Kind.Version,
		Kind:    req.Kind.Kind,
	}.String()]
	if !ok {
		// TODO: error instead?
		kind = resource.Kind{
			Schema: resource.NewSimpleSchema(req.Kind.Group, req.Kind.Version, &resource.UntypedObject{}, &resource.UntypedList{}, resource.WithKind(req.Kind.Kind)),
			Codecs: map[resource.KindEncoding]resource.Codec{
				resource.KindEncodingJSON: resource.NewJSONCodec(),
			},
		}
	}
	var object, oldObject resource.Object
	var err error
	if len(req.ObjectBytes) > 0 {
		object, err = kind.Read(bytes.NewReader(req.ObjectBytes), resource.KindEncodingJSON)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(req.OldObjectBytes) > 0 {
		oldObject, err = kind.Read(bytes.NewReader(req.OldObjectBytes), resource.KindEncodingJSON)
		if err != nil {
			return nil, nil, err
		}
	}

	return &resource.AdmissionRequest{
		Action:    op,
		Group:     req.Kind.Kind,
		Version:   req.Kind.Version,
		Kind:      req.Kind.Kind,
		Object:    object,
		OldObject: oldObject,
		UserInfo: resource.AdmissionUserInfo{
			Username: req.PluginContext.User.Name,
		},
	}, &kind, nil
}

type responseWriter struct {
	status  int
	headers http.Header
	body    bytes.Buffer
}

func (r *responseWriter) Header() http.Header {
	return r.headers
}

func (r *responseWriter) WriteHeader(statusCode int) {
	r.status = statusCode
}

func (r *responseWriter) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseWriter) CallResourceResponse() *backend.CallResourceResponse {
	return &backend.CallResourceResponse{
		Status:  r.status,
		Headers: r.headers,
		Body:    r.body.Bytes(),
	}
}
