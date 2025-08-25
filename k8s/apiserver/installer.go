package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/emicklei/go-restful/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/admission"
	genericregistry "k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	clientrest "k8s.io/client-go/rest"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

// ManagedKindResolver resolves a kind and version into a resource.Kind instance.
// group is not provided as a ManagedKindResolver function is expected to exist on a per-group basis.
type ManagedKindResolver func(kind, ver string) (resource.Kind, bool)

// CustomRouteResponseResolver resolves the kind, version, path, and method into a go type which is returned
// from that custom route call. kind may be empty for resource routes.
// group is not provided as a CustomRouteResponseResolver function is expected to exist on a per-group basis.
type CustomRouteResponseResolver func(kind, ver, path, method string) (any, bool)

// AppInstaller represents an App which can be installed on a kubernetes API server.
// It provides all the methods needed to configure and install an App onto an API server.
type AppInstaller interface {
	// AddToScheme registers all the kinds provided by the App to the runtime.Scheme.
	// Other functionality which relies on a runtime.Scheme may use the last scheme provided in AddToScheme for this purpose.
	AddToScheme(scheme *runtime.Scheme) error
	// GetOpenAPIDefinitions gets a map of OpenAPI definitions for use with kubernetes OpenAPI
	GetOpenAPIDefinitions(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition
	// InstallAPIs installs the API endpoints to an API server
	InstallAPIs(server *genericapiserver.GenericAPIServer, optsGetter genericregistry.RESTOptionsGetter) error
	// AdmissionPlugin returns an admission.Factory to use for the Admission Plugin.
	// If the App does not provide admission control, it should return nil
	AdmissionPlugin() admission.Factory
	// InitializeApp initializes the underlying App for the AppInstaller using the provided kube config.
	// This should only be called once, if the App is already initialized the method should return ErrAppAlreadyInitialized.
	// App initialization should only be done once the final kube config is ready, as it cannot be changed after initialization.
	InitializeApp(clientrest.Config) error
	// App returns the underlying App, if initialized, or ErrAppNotInitialized if not.
	// Callers which depend on the App should account for the App not yet being initialized and do lazy loading or delayed retries.
	App() (app.App, error)
	// GroupVersions returns the list of all GroupVersions supported by this AppInstaller
	GroupVersions() []schema.GroupVersion
	// ManifestData returns the App's ManifestData
	ManifestData() *app.ManifestData
}

// GenericAPIServer describes a generic API server which can have an API Group installed onto it
type GenericAPIServer interface {
	// InstallAPIGroup installs the provided APIGroupInfo onto the API Server
	InstallAPIGroup(apiGroupInfo *genericapiserver.APIGroupInfo) error
}

// GoTypeResolver is an interface which describes an object which catalogs the relationship between different aspects of an app
// and its go types which need to be used by the API server.
type GoTypeResolver interface {
	// KindToGoType resolves a kind and version into a resource.Kind instance.
	// group is not provided as a KindToGoType function is expected to exist on a per-group basis.
	//nolint:revive
	KindToGoType(kind, version string) (goType resource.Kind, exists bool)
	// CustomRouteReturnGoType resolves the kind, version, path, and method into a go type which is returned
	// from that custom route call. kind may be empty for resource routes.
	// group is not provided as a CustomRouteReturnGoType function is expected to exist on a per-group basis.
	//nolint:revive
	CustomRouteReturnGoType(kind, version, path, verb string) (goType any, exists bool)
	// CustomRouteQueryGoType resolves the kind, version, path, and method into a go type which is returned
	// used for the query parameters of the route.
	// group is not provided as a CustomRouteQueryGoType function is expected to exist on a per-group basis.
	//nolint:revive
	CustomRouteQueryGoType(kind, version, path, verb string) (goType runtime.Object, exists bool)
	// CustomRouteRequestBodyGoType resolves the kind, version, path, and method into a go type which is
	// the accepted body type for the request.
	// group is not provided as a CustomRouteRequestBodyGoType function is expected to exist on a per-group basis.
	//nolint:revive
	CustomRouteRequestBodyGoType(kind, version, path, verb string) (goType any, exists bool)
}

var (
	// ErrAppNotInitialized is returned if the app.App has not been initialized
	ErrAppNotInitialized = errors.New("app not initialized")
	// ErrAppAlreadyInitialized is returned if the app.App has already been initialized and cannot be initialized again
	ErrAppAlreadyInitialized = errors.New("app already initialized")
)

var _ AppInstaller = (*defaultInstaller)(nil)

type defaultInstaller struct {
	appProvider app.Provider
	appConfig   app.Config
	resolver    GoTypeResolver

	app    app.App
	appMux sync.Mutex
	scheme *runtime.Scheme
	codecs serializer.CodecFactory
}

// NewDefaultAppInstaller creates a new AppInstaller with default behavior for an app.Provider and app.Config.
//
//nolint:revive
func NewDefaultAppInstaller(appProvider app.Provider, appConfig app.Config, resolver GoTypeResolver) (*defaultInstaller, error) {
	installer := &defaultInstaller{
		appProvider: appProvider,
		appConfig:   appConfig,
		resolver:    resolver,
	}
	if installer.appConfig.ManifestData.IsEmpty() {
		// Fill in the manifest data from the Provider if we can
		m := appProvider.Manifest()
		if m.ManifestData != nil {
			installer.appConfig.ManifestData = *m.ManifestData
		}
	}
	if installer.appConfig.SpecificConfig == nil {
		installer.appConfig.SpecificConfig = appProvider.SpecificConfig()
	}
	return installer, nil
}

func (r *defaultInstaller) AddToScheme(scheme *runtime.Scheme) error {
	if scheme == nil {
		return errors.New("scheme cannot be nil")
	}

	kindsByGV, err := r.getKindsByGroupVersion()
	if err != nil {
		return fmt.Errorf("failed to get kinds by group version: %w", err)
	}

	internalKinds := map[string]resource.Kind{}
	kindsByGroup := map[string][]resource.Kind{}
	groupVersions := make([]schema.GroupVersion, 0)
	for gv, kinds := range kindsByGV {
		for _, kind := range kinds {
			scheme.AddKnownTypeWithName(kind.Kind.GroupVersionKind(), kind.Kind.ZeroValue())
			scheme.AddKnownTypeWithName(gv.WithKind(kind.Kind.Kind()+"List"), kind.Kind.ZeroListValue())
			metav1.AddToGroupVersion(scheme, kind.Kind.GroupVersionKind().GroupVersion())
			if _, ok := internalKinds[kind.Kind.Kind()]; !ok {
				internalKinds[kind.Kind.Kind()] = kind.Kind
			}
			if _, ok := kindsByGroup[kind.Kind.Group()]; !ok {
				kindsByGroup[kind.Kind.Group()] = []resource.Kind{}
			}
			kindsByGroup[kind.Kind.Group()] = append(kindsByGroup[kind.Kind.Group()], kind.Kind)

			for cpath, pathProps := range kind.ManifestKind.Routes {
				if pathProps.Get != nil {
					if t, exists := r.resolver.CustomRouteQueryGoType(kind.Kind.Kind(), gv.Version, cpath, "GET"); exists {
						scheme.AddKnownTypes(gv, t)
					}
				}
			}
		}
		scheme.AddUnversionedTypes(gv, &ResourceCallOptions{})
		err = scheme.AddGeneratedConversionFunc((*url.Values)(nil), (*ResourceCallOptions)(nil), func(a, b any, scope conversion.Scope) error {
			return CovertURLValuesToResourceCallOptions(a.(*url.Values), b.(*ResourceCallOptions), scope)
		})
		if err != nil {
			return fmt.Errorf("could not add conversion func for ResourceCallOptions: %w", err)
		}
		groupVersions = append(groupVersions, gv)
	}

	internalGv := schema.GroupVersion{Group: r.appConfig.ManifestData.Group, Version: runtime.APIVersionInternal}
	for _, internalKind := range internalKinds {
		scheme.AddKnownTypeWithName(internalGv.WithKind(internalKind.Kind()), internalKind.ZeroValue())
		scheme.AddKnownTypeWithName(internalGv.WithKind(internalKind.Kind()+"List"), internalKind.ZeroListValue())

		for _, kind := range kindsByGroup[internalKind.Group()] {
			if err = scheme.AddConversionFunc(kind.ZeroValue(), internalKind.ZeroValue(), r.conversionHandler); err != nil {
				return fmt.Errorf("could not add conversion func for kind %s: %w", internalKind.Kind(), err)
			}
			if err = scheme.AddConversionFunc(internalKind.ZeroValue(), kind.ZeroValue(), r.conversionHandler); err != nil {
				return fmt.Errorf("could not add conversion func for kind %s: %w", internalKind.Kind(), err)
			}
		}
	}

	sort.Slice(groupVersions, func(i, j int) bool {
		return version.CompareKubeAwareVersionStrings(groupVersions[i].Version, groupVersions[j].Version) < 0
	})
	if err = scheme.SetVersionPriority(groupVersions...); err != nil {
		return fmt.Errorf("failed to set version priority: %w", err)
	}

	// save the scheme for later use
	if r.scheme == nil {
		r.scheme = scheme
		r.codecs = serializer.NewCodecFactory(scheme)
	}

	return nil
}

func (r *defaultInstaller) ManifestData() *app.ManifestData {
	return r.appProvider.Manifest().ManifestData
}

func (r *defaultInstaller) GetOpenAPIDefinitions(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	res := map[string]common.OpenAPIDefinition{}
	hasCustomRoutes := false
	for _, v := range r.appConfig.ManifestData.Versions {
		for _, manifestKind := range v.Kinds {
			kind, ok := r.resolver.KindToGoType(manifestKind.Kind, v.Name)
			if !ok {
				continue
			}
			if r.scheme == nil {
				fmt.Printf("scheme is not set in defaultInstaller.GetOpenAPIDefinitions, skipping %s. This will impact kind availability\n", manifestKind.Kind) //nolint:revive
				continue
			}
			pkgPrefix := ""
			for k, t := range r.scheme.KnownTypes(schema.GroupVersion{Group: r.appConfig.ManifestData.Group, Version: v.Name}) {
				if k == manifestKind.Kind {
					pkgPrefix = t.PkgPath()
				}
			}
			if pkgPrefix == "" {
				fmt.Printf("scheme does not contain kind %s.%s, skipping OpenAPI component\n", v.Name, manifestKind.Kind) //nolint:revive
			}
			oapi, err := manifestKind.Schema.AsKubeOpenAPI(kind.GroupVersionKind(), callback, pkgPrefix)
			if err != nil {
				fmt.Printf("failed to convert kind %s to KubeOpenAPI: %v\n", kind.GroupVersionKind().Kind, err) //nolint:revive
				continue
			}
			maps.Copy(res, oapi)
			if len(manifestKind.Routes) > 0 {
				hasCustomRoutes = true
				// Add the definitions and use the name as the reflect type name from the resolver, if it exists
				maps.Copy(res, r.getManifestCustomRoutesOpenAPI(manifestKind.Kind, v.Name, manifestKind.Routes, "", defaultEtcdPathPrefix, callback))
			}
		}
		if len(v.Routes.Namespaced) > 0 {
			hasCustomRoutes = true
			maps.Copy(res, r.getManifestCustomRoutesOpenAPI("", v.Name, v.Routes.Namespaced, "<namespace>", "", callback))
		}
		if len(v.Routes.Cluster) > 0 {
			hasCustomRoutes = true
			maps.Copy(res, r.getManifestCustomRoutesOpenAPI("", v.Name, v.Routes.Cluster, "", "", callback))
		}
	}
	if hasCustomRoutes {
		maps.Copy(res, GetResourceCallOptionsOpenAPIDefinition())
		res["github.com/grafana/grafana-app-sdk/k8s/apiserver.EmptyObject"] = common.OpenAPIDefinition{
			Schema: spec.Schema{
				SchemaProps: spec.SchemaProps{
					Description: "EmptyObject defines a model for a missing object type",
					Type:        []string{"object"},
				},
			},
		}
	}
	return res
}

func (r *defaultInstaller) InstallAPIs(server *genericapiserver.GenericAPIServer, optsGetter genericregistry.RESTOptionsGetter) error {
	group := r.appConfig.ManifestData.Group
	if r.scheme == nil {
		r.scheme = newScheme()
		r.codecs = serializer.NewCodecFactory(r.scheme)
		if err := r.AddToScheme(r.scheme); err != nil {
			return fmt.Errorf("failed to add to scheme: %w", err)
		}
	}
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(group, r.scheme, runtime.NewParameterCodec(r.scheme), r.codecs)

	kindsByGV, err := r.getKindsByGroupVersion()
	if err != nil {
		return fmt.Errorf("failed to get kinds by group version: %w", err)
	}

	for gv, kinds := range kindsByGV {
		storage := map[string]rest.Storage{}
		for _, kind := range kinds {
			s, err := newGenericStoreForKind(r.scheme, kind.Kind, optsGetter)
			if err != nil {
				return fmt.Errorf("failed to create store for kind %s: %w", kind.Kind.Kind(), err)
			}
			storage[kind.Kind.Plural()] = s
			if _, ok := kind.Kind.ZeroValue().GetSubresource(string(resource.SubresourceStatus)); ok {
				storage[fmt.Sprintf("%s/%s", kind.Kind.Plural(), resource.SubresourceStatus)] = newRegistryStatusStoreForKind(r.scheme, kind.Kind, s)
			}
			for route, props := range kind.ManifestKind.Routes {
				if route == "" {
					continue
				}
				if route[0] == '/' {
					route = route[1:]
				}
				storage[fmt.Sprintf("%s/%s", kind.Kind.Plural(), route)] = &SubresourceConnector{
					Methods: spec3PropsToConnectorMethods(props, kind.Kind.Kind(), gv.Version, route, r.resolver.CustomRouteReturnGoType),
					Route: CustomRoute{
						Path: route,
						Handler: func(ctx context.Context, writer app.CustomRouteResponseWriter, request *app.CustomRouteRequest) error {
							logging.FromContext(ctx).Debug("Calling custom subresource route", "path", route, "namespace", request.ResourceIdentifier.Namespace, "name", request.ResourceIdentifier.Name, "gvk", kind.Kind.GroupVersionKind().String())
							a, err := r.App()
							if err != nil {
								logging.FromContext(ctx).Error("failed to get app for calling custom route", "error", err, "path", route, "namespace", request.ResourceIdentifier.Namespace, "name", request.ResourceIdentifier.Name, "gvk", kind.Kind.GroupVersionKind().String())
								return err
							}
							err = a.CallCustomRoute(ctx, writer, request)
							if errors.Is(err, app.ErrCustomRouteNotFound) {
								writer.WriteHeader(http.StatusNotFound)
								fullError := apierrors.StatusError{
									ErrStatus: metav1.Status{
										Status: metav1.StatusFailure,
										Code:   http.StatusNotFound,
										Reason: metav1.StatusReasonNotFound,
										Details: &metav1.StatusDetails{
											Group: gv.Group,
											Kind:  kind.ManifestKind.Kind,
											Name:  request.ResourceIdentifier.Name,
										},
										Message: fmt.Sprintf("%s.%s/%s subresource '%s' not found", kind.ManifestKind.Plural, gv.Group, gv.Version, route),
									}}
								return json.NewEncoder(writer).Encode(fullError)
							}
							return err
						},
					},
					Kind: kind.Kind,
				}
			}
			apiGroupInfo.VersionedResourcesStorageMap[gv.Version] = storage
		}
	}

	err = server.InstallAPIGroup(&apiGroupInfo)

	// version custom routes
	hasResourceRoutes := false
	for _, v := range r.ManifestData().Versions {
		if len(v.Routes.Namespaced) > 0 || len(v.Routes.Cluster) > 0 {
			hasResourceRoutes = true
			break
		}
	}
	if hasResourceRoutes {
		if server.Handler == nil || server.Handler.GoRestfulContainer == nil {
			return errors.New("could not register custom routes: server.Handler.GoRestfulContainer is nil")
		}
		for _, ws := range server.Handler.GoRestfulContainer.RegisteredWebServices() {
			for _, ver := range r.ManifestData().Versions {
				if ws.RootPath() == fmt.Sprintf("/apis/%s/%s", group, ver.Name) {
					for rpath, route := range ver.Routes.Namespaced {
						r.registerResourceRoute(ws, schema.GroupVersion{Group: group, Version: ver.Name}, rpath, route, resource.NamespacedScope)
					}
					for rpath, route := range ver.Routes.Cluster {
						r.registerResourceRoute(ws, schema.GroupVersion{Group: group, Version: ver.Name}, rpath, route, resource.ClusterScope)
					}
				}
			}
		}
	}

	return err
}

func (r *defaultInstaller) registerResourceRoute(ws *restful.WebService, gv schema.GroupVersion, rpath string, props spec3.PathProps, scope resource.SchemaScope) {
	if props.Get != nil {
		r.registerResourceRouteOperation(ws, gv, rpath, props.Get, scope, "GET")
	}
	if props.Post != nil {
		r.registerResourceRouteOperation(ws, gv, rpath, props.Post, scope, "POST")
	}
	if props.Put != nil {
		r.registerResourceRouteOperation(ws, gv, rpath, props.Put, scope, "PUT")
	}
	if props.Patch != nil {
		r.registerResourceRouteOperation(ws, gv, rpath, props.Patch, scope, "PATCH")
	}
	if props.Delete != nil {
		r.registerResourceRouteOperation(ws, gv, rpath, props.Delete, scope, "DELETE")
	}
	if props.Head != nil {
		r.registerResourceRouteOperation(ws, gv, rpath, props.Head, scope, "HEAD")
	}
	if props.Options != nil {
		r.registerResourceRouteOperation(ws, gv, rpath, props.Options, scope, "OPTIONS")
	}
}

func (r *defaultInstaller) registerResourceRouteOperation(ws *restful.WebService, gv schema.GroupVersion, rpath string, op *spec3.Operation, scope resource.SchemaScope, method string) error {
	lookup := rpath
	if scope == resource.NamespacedScope {
		lookup = path.Join("<namespace>", rpath)
	}
	responseType, ok := r.resolver.CustomRouteReturnGoType("", gv.Version, lookup, method)
	if !ok {
		// TODO: warn here?
		responseType = &EmptyObject{}
	}
	fullpath := rpath
	if scope == resource.NamespacedScope {
		fullpath = path.Join("namespaces", "{namespace}", rpath)
	}
	var builder *restful.RouteBuilder
	switch strings.ToLower(method) {
	case "get":
		builder = ws.GET(fullpath)
	case "post":
		builder = ws.POST(fullpath)
	case "put":
		builder = ws.PUT(fullpath)
	case "patch":
		builder = ws.PATCH(fullpath)
	case "delete":
		builder = ws.DELETE(fullpath)
	case "head":
		builder = ws.HEAD(fullpath)
	case "options":
		builder = ws.OPTIONS(fullpath)
	default:
		return fmt.Errorf("unsupported method %s", method)
	}
	if op.RequestBody != nil {
		if goBody, ok := r.resolver.CustomRouteRequestBodyGoType("", gv.Version, lookup, method); ok {
			builder = builder.Reads(goBody)
		}
	}
	if scope == resource.NamespacedScope {
		builder = builder.Param(restful.PathParameter("namespace", "object name and auth scope, such as for teams and projects"))
	}
	for _, param := range op.Parameters {
		switch param.In {
		case "path":
			builder = builder.Param(restful.PathParameter(param.Name, param.Description))
		case "query":
			builder = builder.Param(restful.QueryParameter(param.Name, param.Description))
		case "header":
			builder = builder.Param(restful.HeaderParameter(param.Name, param.Description))
		}
	}
	ws.Route(builder.Operation(strings.ToLower(method)+op.OperationId).To(func(req *restful.Request, resp *restful.Response) {
		a, err := r.App()
		if err != nil {
			resp.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(resp).Encode(metav1.Status{
				Status:  metav1.StatusFailure,
				Code:    http.StatusInternalServerError,
				Message: err.Error(),
			})
		}
		identifier := resource.FullIdentifier{
			Group:   r.appConfig.ManifestData.Group,
			Version: gv.Version,
		}
		if scope == resource.NamespacedScope {
			identifier.Namespace = req.PathParameters()["namespace"]
		}
		err = a.CallCustomRoute(req.Request.Context(), resp, &app.CustomRouteRequest{
			ResourceIdentifier: identifier,
			Path:               rpath,
			URL:                req.Request.URL,
			Method:             http.MethodGet,
			Headers:            req.Request.Header,
			Body:               req.Request.Body,
		})
		if err != nil {
			resp.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(resp).Encode(metav1.Status{
				Status:  metav1.StatusFailure,
				Code:    http.StatusInternalServerError,
				Message: err.Error(),
			})
		}
	}).Returns(200, "OK", responseType))
	return nil
}

func (r *defaultInstaller) AdmissionPlugin() admission.Factory {
	supportsMutation := false
	supportsValidation := false
	for _, v := range r.appConfig.ManifestData.Versions {
		for _, manifestKind := range v.Kinds {
			if manifestKind.Admission != nil && manifestKind.Admission.SupportsAnyMutation() {
				supportsMutation = true
			}
			if manifestKind.Admission != nil && manifestKind.Admission.SupportsAnyValidation() {
				supportsValidation = true
			}
		}
	}
	if supportsMutation || supportsValidation {
		return func(_ io.Reader) (admission.Interface, error) {
			return newAppAdmission(r.appConfig.ManifestData, func() app.App {
				return r.app
			}), nil
		}
	}

	return nil
}

func (r *defaultInstaller) InitializeApp(cfg clientrest.Config) error {
	r.appMux.Lock()
	defer r.appMux.Unlock()
	if r.app != nil {
		return ErrAppAlreadyInitialized
	}
	initApp, err := r.appProvider.NewApp(app.Config{
		KubeConfig:     cfg,
		SpecificConfig: r.appConfig.SpecificConfig,
		ManifestData:   r.appConfig.ManifestData,
	})
	if err != nil {
		return err
	}
	r.app = initApp
	return nil
}

func (r *defaultInstaller) App() (app.App, error) {
	if r.app == nil {
		return nil, ErrAppNotInitialized
	}
	return r.app, nil
}

func (r *defaultInstaller) GroupVersions() []schema.GroupVersion {
	groupVersions := make([]schema.GroupVersion, 0)
	for _, gv := range r.appConfig.ManifestData.Versions {
		groupVersions = append(groupVersions, schema.GroupVersion{Group: r.appConfig.ManifestData.Group, Version: gv.Name})
	}
	return groupVersions
}

func (r *defaultInstaller) conversionHandler(a, b any, _ conversion.Scope) error {
	if r.app == nil {
		return fmt.Errorf("app is not initialized")
	}
	if r.scheme == nil {
		return fmt.Errorf("scheme is not initialized")
	}
	aResourceObj, ok := a.(resource.Object)
	if !ok {
		return fmt.Errorf("object (%T) is not a resource.Object", a)
	}
	bResourceObj, ok := b.(resource.Object)
	if !ok {
		return fmt.Errorf("object (%T) is not a resource.Object", b)
	}

	rawInput, err := runtime.Encode(r.codecs.LegacyCodec(aResourceObj.GroupVersionKind().GroupVersion()), aResourceObj)
	if err != nil {
		return fmt.Errorf("failed to encode object %s: %w", aResourceObj.GetName(), err)
	}

	req := app.ConversionRequest{
		SourceGVK: aResourceObj.GroupVersionKind(),
		TargetGVK: bResourceObj.GroupVersionKind(),
		Raw: app.RawObject{
			Raw:      rawInput,
			Object:   aResourceObj,
			Encoding: resource.KindEncodingJSON,
		},
	}
	res, err := r.app.Convert(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to convert object %s from %s to %s: %w", aResourceObj.GetName(), req.SourceGVK, req.TargetGVK, err)
	}

	bObj, ok := b.(runtime.Object)
	if !ok {
		return fmt.Errorf("object (%T) is not a runtime.Object", b)
	}

	return runtime.DecodeInto(r.codecs.UniversalDecoder(bResourceObj.GroupVersionKind().GroupVersion()), res.Raw, bObj)
}

func (r *defaultInstaller) getManifestCustomRoutesOpenAPI(kind, ver string, routes map[string]spec3.PathProps, routePathPrefix string, defaultPkgPrefix string, callback common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	defs := make(map[string]common.OpenAPIDefinition)
	for rpath, pathProps := range routes {
		if routePathPrefix != "" {
			rpath = path.Join(routePathPrefix, rpath)
		}
		if pathProps.Get != nil {
			key, val := r.getOperationResponseOpenAPI(kind, ver, rpath, "GET", pathProps.Get, r.resolver.CustomRouteReturnGoType, defaultPkgPrefix, callback)
			defs[key] = val
			if pathProps.Get.RequestBody != nil {
				key, val := r.getOperationRequestBodyOpenAPI(kind, ver, rpath, "GET", pathProps.Get, r.resolver.CustomRouteRequestBodyGoType, defaultPkgPrefix, callback)
				defs[key] = val
			}
		}
		if pathProps.Post != nil {
			key, val := r.getOperationResponseOpenAPI(kind, ver, rpath, "POST", pathProps.Post, r.resolver.CustomRouteReturnGoType, defaultPkgPrefix, callback)
			defs[key] = val
			if pathProps.Post.RequestBody != nil {
				key, val := r.getOperationRequestBodyOpenAPI(kind, ver, rpath, "POST", pathProps.Post, r.resolver.CustomRouteRequestBodyGoType, defaultPkgPrefix, callback)
				defs[key] = val
			}
		}
		if pathProps.Put != nil {
			key, val := r.getOperationResponseOpenAPI(kind, ver, rpath, "PUT", pathProps.Put, r.resolver.CustomRouteReturnGoType, defaultPkgPrefix, callback)
			defs[key] = val
			if pathProps.Put.RequestBody != nil {
				key, val := r.getOperationRequestBodyOpenAPI(kind, ver, rpath, "PUT", pathProps.Put, r.resolver.CustomRouteRequestBodyGoType, defaultPkgPrefix, callback)
				defs[key] = val
			}
		}
		if pathProps.Patch != nil {
			key, val := r.getOperationResponseOpenAPI(kind, ver, rpath, "PATCH", pathProps.Patch, r.resolver.CustomRouteReturnGoType, defaultPkgPrefix, callback)
			defs[key] = val
			if pathProps.Patch.RequestBody != nil {
				key, val := r.getOperationRequestBodyOpenAPI(kind, ver, rpath, "PATCH", pathProps.Patch, r.resolver.CustomRouteRequestBodyGoType, defaultPkgPrefix, callback)
				defs[key] = val
			}
		}
		if pathProps.Delete != nil {
			key, val := r.getOperationResponseOpenAPI(kind, ver, rpath, "DELETE", pathProps.Delete, r.resolver.CustomRouteReturnGoType, defaultPkgPrefix, callback)
			defs[key] = val
			if pathProps.Delete.RequestBody != nil {
				key, val := r.getOperationRequestBodyOpenAPI(kind, ver, rpath, "DELETE", pathProps.Delete, r.resolver.CustomRouteRequestBodyGoType, defaultPkgPrefix, callback)
				defs[key] = val
			}
		}
		if pathProps.Head != nil {
			key, val := r.getOperationResponseOpenAPI(kind, ver, rpath, "HEAD", pathProps.Head, r.resolver.CustomRouteReturnGoType, defaultPkgPrefix, callback)
			defs[key] = val
			if pathProps.Head.RequestBody != nil {
				key, val := r.getOperationRequestBodyOpenAPI(kind, ver, rpath, "HEAD", pathProps.Head, r.resolver.CustomRouteRequestBodyGoType, defaultPkgPrefix, callback)
				defs[key] = val
			}
		}
		if pathProps.Options != nil {
			key, val := r.getOperationResponseOpenAPI(kind, ver, rpath, "OPTIONS", pathProps.Options, r.resolver.CustomRouteReturnGoType, defaultPkgPrefix, callback)
			defs[key] = val
			if pathProps.Options.RequestBody != nil {
				key, val := r.getOperationRequestBodyOpenAPI(kind, ver, rpath, "OPTIONS", pathProps.Options, r.resolver.CustomRouteRequestBodyGoType, defaultPkgPrefix, callback)
				defs[key] = val
			}
		}
	}
	return defs
}

func (*defaultInstaller) getOperationResponseOpenAPI(kind, ver, path, method string, operation *spec3.Operation, resolver CustomRouteResponseResolver, defaultPkgPrefix string, _ common.ReferenceCallback) (string, common.OpenAPIDefinition) {
	typePath := ""
	if resolver == nil {
		resolver = func(_, _, _, _ string) (any, bool) {
			return nil, false
		}
	}
	goType, ok := resolver(kind, ver, path, method)
	if ok {
		typ := reflect.TypeOf(goType)
		typePath = typ.PkgPath() + "." + typ.Name()
	} else {
		// Use a default type name
		var ucFirstMethod string
		if len(method) > 1 {
			ucFirstMethod = strings.ToUpper(method[:1]) + strings.ToLower(method[1:])
		} else {
			ucFirstMethod = strings.ToUpper(method)
		}
		ucFirstPath := regexp.MustCompile("[^A-Za-z0-9]").ReplaceAllString(path, "")
		if len(ucFirstPath) > 1 {
			ucFirstPath = strings.ToUpper(ucFirstPath[:1]) + ucFirstPath[1:]
		} else {
			ucFirstPath = strings.ToUpper(ucFirstPath)
		}
		typePath = fmt.Sprintf("%s.%s%s", defaultPkgPrefix, ucFirstMethod, ucFirstPath)
	}
	var typeSchema spec.Schema
	if operation.Responses != nil && operation.Responses.Default != nil {
		if len(operation.Responses.Default.Content) > 0 {
			for key, val := range operation.Responses.Default.Content {
				if val.Schema != nil {
					typeSchema = *val.Schema
				}
				if key == "application/json" {
					break
				}
			}
		}
	}
	return typePath, common.OpenAPIDefinition{
		Schema: typeSchema,
	}
}

func (*defaultInstaller) getOperationRequestBodyOpenAPI(kind, ver, path, method string, operation *spec3.Operation, resolver CustomRouteResponseResolver, defaultPkgPrefix string, _ common.ReferenceCallback) (string, common.OpenAPIDefinition) {
	typePath := ""
	if resolver == nil {
		resolver = func(_, _, _, _ string) (any, bool) {
			return nil, false
		}
	}
	goType, ok := resolver(kind, ver, path, method)
	if ok {
		typ := reflect.TypeOf(goType)
		typePath = typ.PkgPath() + "." + typ.Name()
	} else {
		// Use a default type name
		var ucFirstMethod string
		if len(method) > 1 {
			ucFirstMethod = strings.ToUpper(method[:1]) + strings.ToLower(method[1:])
		} else {
			ucFirstMethod = strings.ToUpper(method)
		}
		ucFirstPath := regexp.MustCompile("[^A-Za-z0-9]").ReplaceAllString(path, "")
		if len(ucFirstPath) > 1 {
			ucFirstPath = strings.ToUpper(ucFirstPath[:1]) + ucFirstPath[1:]
		} else {
			ucFirstPath = strings.ToUpper(ucFirstPath)
		}
		typePath = fmt.Sprintf("%s.%s%s", defaultPkgPrefix, ucFirstMethod, ucFirstPath)
	}
	var typeSchema spec.Schema
	if operation.RequestBody != nil {
		if len(operation.RequestBody.Content) > 0 {
			for key, val := range operation.RequestBody.Content {
				if val.Schema != nil {
					typeSchema = *val.Schema
				}
				if key == "application/json" {
					break
				}
			}
		}
	}
	return typePath, common.OpenAPIDefinition{
		Schema: typeSchema,
	}
}

type KindAndManifestKind struct {
	Kind         resource.Kind
	ManifestKind app.ManifestVersionKind
}

func (r *defaultInstaller) getKindsByGroupVersion() (map[schema.GroupVersion][]KindAndManifestKind, error) {
	out := make(map[schema.GroupVersion][]KindAndManifestKind)
	group := r.appConfig.ManifestData.Group
	for _, v := range r.appConfig.ManifestData.Versions {
		for _, manifestKind := range v.Kinds {
			gv := schema.GroupVersion{Group: group, Version: v.Name}
			kind, ok := r.resolver.KindToGoType(manifestKind.Kind, v.Name)
			if !ok {
				return nil, fmt.Errorf("failed to resolve kind %s", manifestKind.Kind)
			}
			out[gv] = append(out[gv], KindAndManifestKind{Kind: kind, ManifestKind: manifestKind})
		}
	}
	return out, nil
}

func spec3PropsToConnectorMethods(props spec3.PathProps, kind, ver, path string, resolver CustomRouteResponseResolver) map[string]SubresourceConnectorResponseObject {
	if resolver == nil {
		resolver = func(_, _, _, _ string) (any, bool) {
			return nil, false
		}
	}
	mimeTypes := func(operation *spec3.Operation) []string {
		if operation.Responses == nil {
			return []string{"*/*"}
		}
		if operation.Responses.Default == nil {
			return []string{"*/*"}
		}
		types := make([]string, 0)
		for contentType := range operation.Responses.Default.Content {
			types = append(types, contentType)
		}
		return types
	}
	methods := make(map[string]SubresourceConnectorResponseObject)
	if props.Get != nil {
		resp, _ := resolver(kind, ver, path, "GET")
		methods["GET"] = SubresourceConnectorResponseObject{
			Object:    resp,
			MIMETypes: mimeTypes(props.Get),
		}
	}
	if props.Post != nil {
		resp, _ := resolver(kind, ver, path, "POST")
		methods["POST"] = SubresourceConnectorResponseObject{
			Object:    resp,
			MIMETypes: mimeTypes(props.Get),
		}
	}
	if props.Put != nil {
		resp, _ := resolver(kind, ver, path, "PUT")
		methods["PUT"] = SubresourceConnectorResponseObject{
			Object:    resp,
			MIMETypes: mimeTypes(props.Get),
		}
	}
	if props.Patch != nil {
		resp, _ := resolver(kind, ver, path, "PATCH")
		methods["PATCH"] = SubresourceConnectorResponseObject{
			Object:    resp,
			MIMETypes: mimeTypes(props.Get),
		}
	}
	if props.Delete != nil {
		resp, _ := resolver(kind, ver, path, "DELETE")
		methods["DELETE"] = SubresourceConnectorResponseObject{
			Object:    resp,
			MIMETypes: mimeTypes(props.Get),
		}
	}
	if props.Head != nil {
		resp, _ := resolver(kind, ver, path, "HEAD")
		methods["HEAD"] = SubresourceConnectorResponseObject{
			Object:    resp,
			MIMETypes: mimeTypes(props.Get),
		}
	}
	if props.Options != nil {
		resp, _ := resolver(kind, ver, path, "OPTIONS")
		methods["OPTIONS"] = SubresourceConnectorResponseObject{
			Object:    resp,
			MIMETypes: mimeTypes(props.Get),
		}
	}
	return methods
}

func NewDefaultScheme() *runtime.Scheme {
	return newScheme()
}

func newScheme() *runtime.Scheme {
	unversionedVersion := schema.GroupVersion{Group: "", Version: "v1"}
	unversionedTypes := []runtime.Object{
		&metav1.Status{},
		&metav1.WatchEvent{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
		&metav1.PartialObjectMetadata{},
		&metav1.PartialObjectMetadataList{},
	}

	scheme := runtime.NewScheme()
	// we need to add the options to empty v1
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Group: "", Version: "v1"})
	scheme.AddUnversionedTypes(unversionedVersion, unversionedTypes...)
	return scheme
}

type EmptyObject struct{}
