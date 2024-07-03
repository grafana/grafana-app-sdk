package simple

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/grafana/grafana-app-sdk/apiserver"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"k8s.io/kube-openapi/pkg/common"
)

var _ resource.App = &App{}

type managedResource struct {
	kind              resource.Kind
	validator         resource.ValidatingAdmissionController
	mutator           resource.MutatingAdmissionController
	converter         apiserver.ResourceConverter // TODO: keep this this way?
	subresourceRoutes []SubresourceRoute
}

type SubresourceRoute struct {
	Path    string
	Handler AdditionalRouteHandler
}

type AdditionalRouteHandler func(w http.ResponseWriter, r *resource.SubresourceRequest)

type AdditionalRouteRequest struct {
	*backend.CallResourceRequest
	ResourceIdentifier resource.Identifier
}

type AppResource struct {
	Kind                  resource.Kind
	GetOpenAPIDefinitions common.GetOpenAPIDefinitions
	Validator             resource.ValidatingAdmissionController
	Mutator               resource.MutatingAdmissionController
	Converter             apiserver.ResourceConverter // TODO: still use this method?
	Subresources          []SubresourceRoute
	Reconciler            Reconciler
	Watcher               Watcher
}

type AppConfig struct {
}

// App implements app.App
type App struct {
	plurals       map[string]resource.Kind
	resources     map[string]managedResource
	internalKinds map[string]resource.Kind
	controller    operator.Controller
}

func NewApp() (*App, error) {
	app := &App{
		plurals:       make(map[string]resource.Kind),
		resources:     make(map[string]managedResource),
		internalKinds: make(map[string]resource.Kind),
	}
	return app, nil
}

func (a *App) Init() {}

func (a *App) Manage(resource AppResource) error {
	a.resources[gvk(resource.Kind.Group(), resource.Kind.Version(), resource.Kind.Kind())] = managedResource{
		kind:      resource.Kind,
		validator: resource.Validator,
		mutator:   resource.Mutator,
		converter: resource.Converter,
	}
	internalGK := gk(resource.Kind.Group(), resource.Kind.Kind())
	internal, ok := a.internalKinds[internalGK]
	if !ok {
		a.internalKinds[internalGK] = resource.Kind
		return nil
	}
	// TODO: simple string compare for the moment, will change this to handle kubernetes-style versions and others (like legacy thema)
	if strings.Compare(resource.Kind.Version(), internal.Version()) > 0 {
		a.internalKinds[internalGK] = resource.Kind
	}
	return nil
}

func (a *App) Validate(ctx context.Context, request *resource.AdmissionRequest) error {
	r, ok := a.resources[gvk(request.Group, request.Version, request.Kind)]
	if !ok {
		// TODO: Default validator
		return nil
	}
	return r.validator.Validate(ctx, request)
}

func (a *App) Mutate(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error) {
	r, ok := a.resources[gvk(request.Group, request.Version, request.Kind)]
	if !ok {
		// TODO: Default validator
		return nil, nil
	}
	return r.mutator.Mutate(ctx, request)
}

func (a *App) Convert(ctx context.Context, request resource.ConversionRequest) (*resource.RawObject, error) {
	if request.SourceGVK == request.TargetGVK {
		return &request.Raw, nil
	}
	if request.SourceGVK.Group != request.TargetGVK.Group {
		return nil, fmt.Errorf("cannot convert between different groups")
	}
	if request.SourceGVK.Kind != request.TargetGVK.Kind {
		return nil, fmt.Errorf("cannot convert between different kinds")
	}
	r, ok := a.resources[gvk(request.SourceGVK.Group, request.SourceGVK.Version, request.SourceGVK.Kind)]
	if !ok || r.converter == nil {
		// TODO: Default conversion
		return nil, nil
	}
	internal, ok := a.internalKinds[gk(request.SourceGVK.Group, request.SourceGVK.Kind)]
	if !ok {
		// Really shouldn't end up here
		return nil, fmt.Errorf("no internal version registered for kind")
	}
	internalInstance := internal.ZeroValue()
	srcInstance := request.Raw.Object
	if srcInstance == nil {
		var err error
		srcInstance, err = r.kind.Read(bytes.NewReader(request.Raw.Raw), request.Raw.Encoding)
		if err != nil {
			return nil, err
		}
	}
	err := r.converter.ToInternal(srcInstance, internalInstance)
	if err != nil {
		return nil, err
	}
	dst, ok := a.resources[gvk(request.TargetGVK.Group, request.TargetGVK.Version, request.TargetGVK.Kind)]
	if !ok || dst.converter == nil {
		// TODO: default conversion
		return nil, nil
	}
	dstInstance := dst.kind.ZeroValue()
	err = dst.converter.FromInternal(internalInstance, dstInstance)
	if err != nil {
		return nil, err
	}
	buf := bytes.Buffer{}
	if err := dst.kind.Write(dstInstance, &buf, request.Raw.Encoding); err != nil {
		return nil, err
	}
	return &resource.RawObject{
		Object:   dstInstance,
		Raw:      buf.Bytes(),
		Encoding: request.Raw.Encoding,
	}, nil
}

func (a *App) Kind(lookup resource.ResourceLookup) *resource.Kind {
	r, ok := a.getManagedResource(lookup)
	if !ok {
		return nil
	}
	return &r.kind
}

func (a *App) CallSubresource(ctx context.Context, responseWriter http.ResponseWriter, request *resource.SubresourceRequest) {
	route := a.GetSubroute(resource.ResourceLookup{Group: request.Group, Version: request.Version, Kind: request.Kind}, request.SubresourcePath)
	if route == nil {
		responseWriter.WriteHeader(http.StatusNotFound)
		return
	}
	route.Handler(responseWriter, request)
}

func (a *App) Controller() resource.Runnable {
	return a.controller
}

func (a *App) GetSubroute(lookup resource.ResourceLookup, subresource string) *SubresourceRoute {
	r, ok := a.getManagedResource(lookup)
	if !ok {
		return nil
	}
	for i, route := range r.subresourceRoutes {
		if strings.Trim(route.Path, "/") == strings.Trim(subresource, "/") {
			return &r.subresourceRoutes[i]
		}
	}
	return nil
}

func (a *App) getManagedResource(lookup resource.ResourceLookup) (*managedResource, bool) {
	key := gvk(lookup.Group, lookup.Version, lookup.Kind)
	if lookup.Kind == "" && lookup.Plural != "" {
		pl, ok := a.plurals[gvk(lookup.Group, lookup.Version, lookup.Plural)]
		if !ok {
			return nil, false
		}
		key = gvk(pl.Group(), pl.Version(), pl.Kind())
	}
	r, ok := a.resources[key]
	if !ok {
		return nil, false
	}
	return &r, true
}

func (a *App) Run() error {
	return nil
}

func gvk(group, version, kind string) string {
	return fmt.Sprintf("%s/%s/%s", group, version, kind)
}

func gk(group, kind string) string {
	return fmt.Sprintf("%s/%s", group, kind)
}
