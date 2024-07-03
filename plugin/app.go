package plugin

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// App wraps an App in a context which runs as a grafana plugin.
// App can be run as a grafana plugin with something like app.Manage, or using its standalone Run() method.
type App struct {
	app                 resource.App
	controllerStopCh    chan struct{}
	controllerStartFunc func()
}

type PluginAppConfig struct {
}

func NewPluginApp(app resource.App, cfg PluginAppConfig) *App {
	stopCh := make(chan struct{})
	return &App{
		app:              app,
		controllerStopCh: stopCh,
		// Needed for current lazy-load plugin design. Once init call exists we can ditch this and call Run on init
		controllerStartFunc: sync.OnceFunc(func() {
			app.Controller().Run(stopCh)
		}),
	}
}

func (p *App) ValidateAdmission(ctx context.Context, req *backend.AdmissionRequest) (*backend.ValidationResponse, error) {
	p.controllerStartFunc()
	adm, err := pluginAdmissionRequestToResourceAdmissionRequest(p.app, req)
	if err != nil {
		return nil, err
	}
	err = p.app.Validate(ctx, adm)
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

func (p *App) MutateAdmission(ctx context.Context, req *backend.AdmissionRequest) (*backend.MutationResponse, error) {
	p.controllerStartFunc()
	adm, err := pluginAdmissionRequestToResourceAdmissionRequest(p.app, req)
	if err != nil {
		return nil, err
	}
	resp, err := p.app.Mutate(ctx, adm)
	if err != nil {
		return &backend.MutationResponse{
			Allowed: false,
			Result: &backend.StatusResult{
				Message: err.Error(),
			},
		}, nil
	}
	var codec resource.Codec = resource.NewJSONCodec()
	kind := p.app.Kind(resource.ResourceLookup{Group: req.Kind.Group, Version: req.Kind.Version, Kind: req.Kind.Kind})
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
func (p *App) ConvertObject(ctx context.Context, req *backend.ConversionRequest) (*backend.ConversionResponse, error) {
	p.controllerStartFunc()
	converted, err := p.app.Convert(ctx, resource.ConversionRequest{
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
	})
	if err != nil {
		return nil, err
	}
	return &backend.ConversionResponse{
		Allowed:     true,
		ObjectBytes: converted.Raw,
	}, nil
}

func (p *App) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	p.controllerStartFunc()
	// CallResource is used for subroute calls (this should probably change at some point to something that has GVK in the typed part of the payload)
	// TODO: we expect the path to be <group>/<version>/namespaces/<ns>/<plural>/<name>/<subroute>
	// or <group>/<version>/<plural>/<name>/<subroute> when the resource if cluster-scoped
	segments := strings.Split(req.Path, "/")
	if len(segments) < 5 {
		// TODO: check if it's a webhook call routed here?
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusNotFound,
		})
	}
	lookup := resource.ResourceLookup{
		Group:   segments[0],
		Version: segments[1],
	}
	id := resource.Identifier{}
	path := ""
	if segments[2] == "namespaces" {
		id.Namespace = segments[3]
		lookup.Plural = segments[4]
		id.Name = segments[5]
		path = segments[6]
	} else {
		lookup.Plural = segments[2]
		id.Name = segments[3]
		path = segments[4]
	}
	kind := p.app.Kind(lookup)
	if kind == nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusNotFound,
		})
	}
	r := resource.SubresourceRequest{
		CallResourceRequest: req,
		ResourceIdentifier:  id,
		SubresourcePath:     path,
		Group:               kind.Group(),
		Version:             kind.Version(),
		Kind:                kind.Kind(),
	}
	w := responseWriter{
		status:  http.StatusOK,
		headers: make(http.Header),
	}

	p.app.CallSubresource(ctx, &w, &r)

	return sender.Send(w.CallResourceResponse())
}

func (p *App) Run() error {
	return backend.Serve(backend.ServeOpts{
		CallResourceHandler: p,
		AdmissionHandler:    p,
	})
}

func pluginAdmissionRequestToResourceAdmissionRequest(a resource.App, req *backend.AdmissionRequest) (*resource.AdmissionRequest, error) {
	var op resource.AdmissionAction
	switch req.Operation {
	case backend.AdmissionRequestCreate:
		op = resource.AdmissionActionCreate
	case backend.AdmissionRequestUpdate:
		op = resource.AdmissionActionUpdate
	case backend.AdmissionRequestDelete:
		op = resource.AdmissionActionDelete
	}
	kind := a.Kind(resource.ResourceLookup{Group: req.Kind.Group, Version: req.Kind.Version, Kind: req.Kind.Kind})
	if kind == nil {
		kind = &resource.Kind{
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
			return nil, err
		}
	}
	if len(req.OldObjectBytes) > 0 {
		oldObject, err = kind.Read(bytes.NewReader(req.OldObjectBytes), resource.KindEncodingJSON)
		if err != nil {
			return nil, err
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
	}, nil
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

var (
	_ backend.AdmissionHandler    = &App{}
	_ backend.CallResourceHandler = &App{}
	_ http.ResponseWriter         = &responseWriter{}
)
