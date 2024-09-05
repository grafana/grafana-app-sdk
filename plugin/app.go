package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	pluginapp "github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/tracing"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime/schema"

	sdkapp "github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

var (
	_ backend.AdmissionHandler = &App{}
)

type capabilities struct {
	conversion bool
	mutation   bool
	validation bool
}

type App struct {
	provider       sdkapp.Provider
	app            sdkapp.App
	manifestData   *sdkapp.ManifestData
	capabilities   map[string]capabilities
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
	return pluginapp.Manage(pluginID, a.instanceFactory, pluginapp.ManageOpts{
		GRPCSettings: opts.GRPCSettings,
		TracingOpts:  opts.TracingOpts,
	})
}

func (a *App) ValidateAdmission(ctx context.Context, req *backend.AdmissionRequest) (*backend.ValidationResponse, error) {
	if !a.isInitialized() {
		return nil, fmt.Errorf("app is not initialized")
	}
	// If validation isn't supported, just return allowed
	if c, ok := a.getCapabilities(req.Kind.Kind, req.Kind.Version); !ok || !c.validation {
		return &backend.ValidationResponse{
			Allowed: true,
		}, nil
	}
	adm, _, err := a.pluginAdmissionRequestToResourceAdmissionRequest(req)
	if err != nil {
		return nil, err
	}
	err = a.app.Validate(ctx, adm)
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
	// If mutation isn't supported, just return allowed with no changes
	if c, ok := a.getCapabilities(req.Kind.Kind, req.Kind.Version); !ok || !c.mutation {
		return &backend.MutationResponse{
			Allowed: true,
		}, nil
	}
	adm, kind, err := a.pluginAdmissionRequestToResourceAdmissionRequest(req)
	if err != nil {
		return nil, err
	}
	resp, err := a.app.Mutate(ctx, adm)
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

type mdUnmarshaler struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
}

func (a *App) ConvertObject(ctx context.Context, req *backend.ConversionRequest) (*backend.ConversionResponse, error) {
	if !a.isInitialized() {
		return nil, fmt.Errorf("app is not initialized")
	}
	resp := &backend.ConversionResponse{}
	for _, obj := range req.Objects {
		srcGVK := schema.GroupVersionKind{}
		enc := resource.KindEncodingUnknown
		// We don't have kind information in the request, so we need to do a partial unmarshal on the object to get
		// source GroupVersion, and Kind
		switch obj.ContentType {
		case "application/json":
			enc = resource.KindEncodingJSON
			dst := mdUnmarshaler{}
			err := json.Unmarshal(obj.Raw, &mdUnmarshaler{})
			if err != nil {
				return nil, err
			}
			srcGVK = schema.FromAPIVersionAndKind(dst.APIVersion, dst.Kind)
		case "application/x-yaml", "application/yaml":
			enc = resource.KindEncodingYAML
			dst := mdUnmarshaler{}
			err := yaml.Unmarshal(obj.Raw, &mdUnmarshaler{})
			if err != nil {
				return nil, err
			}
			srcGVK = schema.FromAPIVersionAndKind(dst.APIVersion, dst.Kind)
		}
		convReq := sdkapp.ConversionRequest{
			SourceGVK: srcGVK,
			TargetGVK: schema.GroupVersionKind{
				Group:   req.TargetVersion.Group,
				Version: req.TargetVersion.Version,
				Kind:    srcGVK.Kind,
			},
			Raw: sdkapp.RawObject{
				Raw:      obj.Raw,
				Encoding: enc,
			},
		}
		if c, ok := a.getCapabilities(srcGVK.Kind, srcGVK.Version); !ok || !c.conversion {
			// The app doesn't have conversion capabilities, but we received a conversion request.
			// If we error here, the app will error, so instead do a basic conversion
			dstKind, ok := a.gvkKinds[convReq.TargetGVK.String()]
			if !ok {
				resp.Result = &backend.StatusResult{
					Status:  "Failure",
					Message: "invalid target GroupVersionKind",
				}
				return resp, nil
			}
			converted, err := dstKind.Read(bytes.NewReader(obj.Raw), resource.KindEncoding(obj.ContentType))
			if err != nil {
				resp.Result = &backend.StatusResult{
					Status:  "Failure",
					Message: err.Error(),
				}
				return resp, nil
			}
			converted.SetGroupVersionKind(dstKind.GroupVersionKind())
			buf := bytes.Buffer{}
			err = dstKind.Write(converted, &buf, resource.KindEncoding(obj.ContentType))
			if err != nil {
				resp.Result = &backend.StatusResult{
					Status:  "Failure",
					Message: err.Error(),
				}
				return resp, nil
			}
			resp.Objects = append(resp.Objects, backend.RawObject{
				Raw:         buf.Bytes(),
				ContentType: obj.ContentType,
			})
		} else {
			converted, err := a.app.Convert(ctx, convReq)
			if err != nil {
				resp.Result = &backend.StatusResult{
					Status:  "Failure",
					Message: err.Error(),
				}
				return resp, nil
			}
			resp.Objects = append(resp.Objects, backend.RawObject{
				Raw:         converted.Raw,
				ContentType: string(converted.Encoding),
			})
		}
	}
	resp.Result = &backend.StatusResult{
		Status: "Success",
	}
	return resp, nil
}

func (a *App) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if !a.isInitialized() {
		return fmt.Errorf("app is not initialized")
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

	err := a.app.CallSubresource(ctx, &w, &sdkapp.SubresourceRequest{
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
// func (a *App) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
//
// }

func (a *App) instanceFactory(_ context.Context, _ backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	// For now, we have to hard-code the kubeconfig loading as it's not provided in settings
	a.initializedMux.Lock()
	defer a.initializedMux.Unlock()
	// TODO: get kubeconfig?
	var err error
	a.manifestData, err = a.fetchManifestData()
	if err != nil {
		return nil, err
	}
	app, err := a.provider.NewApp(sdkapp.Config{
		ManifestData: *a.manifestData,
	})
	if err != nil {
		return nil, err
	}
	a.setCapabilities(a.manifestData)
	a.app = app
	for _, kind := range a.app.ManagedKinds() {
		a.gvkKinds[kind.GroupVersionKind().String()] = kind
		a.gvrKinds[kind.GroupVersionResource().String()] = kind
	}
	a.initialized = true
	a.runnerStopCh = make(chan struct{})
	runner := a.app.Runner()
	if runner != nil {
		go func() {
			err := runner.Run(a.runnerStopCh)
			if err != nil {
				logging.DefaultLogger.With("error", err).Error("runner stopped unexpectedly")
			} else {
				logging.DefaultLogger.Info("runner stopped")
			}
		}()
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

func (a *App) getCapabilities(kind, version string) (capabilities, bool) {
	c, ok := a.capabilities[fmt.Sprintf("%s/%s", kind, version)]
	return c, ok
}

func (a *App) setCapabilities(manifestData *sdkapp.ManifestData) {
	if manifestData == nil {
		return
	}
	for _, kind := range manifestData.Kinds {
		for _, version := range kind.Versions {
			if version.Admission == nil {
				continue
			}
			a.capabilities[fmt.Sprintf("%s/%s", kind.Kind, version.Name)] = capabilities{
				conversion: kind.Conversion,
				mutation:   version.Admission.SupportsAnyMutation(),
				validation: version.Admission.SupportsAnyValidation(),
			}
		}
	}
}

func (a *App) fetchManifestData() (*sdkapp.ManifestData, error) {
	// TODO: get from various places
	manifest := a.provider.Manifest()
	data := sdkapp.ManifestData{}
	switch manifest.Location.Type {
	case sdkapp.ManifestLocationEmbedded:
		if manifest.ManifestData == nil {
			return nil, fmt.Errorf("no ManifestData in Manifest")
		}
		data = *manifest.ManifestData
	case sdkapp.ManifestLocationFilePath:
		// TODO: more correct version?
		dir := os.DirFS(".")
		contents, err := fs.ReadFile(dir, manifest.Location.Path)
		if err != nil {
			return nil, fmt.Errorf("error reading manifest file from disk (path: %s): %w", manifest.Location.Path, err)
		}
		m := sdkapp.Manifest{}
		if err = json.Unmarshal(contents, &m); err == nil && m.ManifestData != nil {
			data = *m.ManifestData
		} else {
			return nil, fmt.Errorf("unable to unmarshal manifest data: %w", err)
		}
	case sdkapp.ManifestLocationAPIServerResource:
		// TODO: fetch from API server
		return nil, fmt.Errorf("apiserver location not supported yet")
	}
	return &data, nil
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
