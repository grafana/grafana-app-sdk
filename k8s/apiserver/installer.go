package apiserver

import (
	"context"
	"fmt"
	"io"
	"maps"
	"sort"

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

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
)

type ManagedKindResolver func(kind, ver string) (resource.Kind, bool)

// Installer represents an App which can be installed on a kubernetes API server.
// It provides all the methods needed to configure and install an App onto an API server.
type Installer interface {
	// AddToScheme registers all the kinds provided by the App to the runtime.Scheme
	AddToScheme(scheme *runtime.Scheme) error
	// GetOpenAPIDefinitions gets a map of OpenAPI definitions for use with kubernetes OpenAPI
	GetOpenAPIDefinitions(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition
	// InstallAPIs installs the API endpoints to an API server
	InstallAPIs(server GenericAPIServer, optsGetter genericregistry.RESTOptionsGetter) error
	// AdmissionPlugin returns an admission.Factory to use for the Admission Plugin.
	// If the App does not provide admission control, it should return nil
	AdmissionPlugin() admission.Factory
	// App creates the app.App if it does not yet exist, or returns the existing app.App if already initialized
	App(restConfig clientrest.Config) (app.App, error)
	// GroupVersions returns the list of all GroupVersions supported by this Installer
	GroupVersions() []schema.GroupVersion
	// ManifestData returns the App's ManifestData
	ManifestData() *app.ManifestData
}

type GenericAPIServer interface {
	InstallAPIGroup(apiGroupInfo *genericapiserver.APIGroupInfo) error
}

var _ Installer = (*defaultInstaller)(nil)

type defaultInstaller struct {
	appProvider         app.Provider
	appConfig           app.Config
	managedKindResolver ManagedKindResolver

	app    app.App
	scheme *runtime.Scheme
	codecs serializer.CodecFactory
}

// NewDefaultInstaller creates a new Installer with default behavior for an app.Provider and app.Config.
//
//nolint:revive
func NewDefaultInstaller(appProvider app.Provider, appConfig app.Config, kindResolver ManagedKindResolver) (*defaultInstaller, error) {
	installer := &defaultInstaller{
		appProvider:         appProvider,
		appConfig:           appConfig,
		managedKindResolver: kindResolver,
	}
	return installer, nil
}

func (r *defaultInstaller) AddToScheme(scheme *runtime.Scheme) error {
	kindsByGV, err := r.getKindsByGroupVersion()
	if err != nil {
		return fmt.Errorf("failed to get kinds by group version: %w", err)
	}

	internalKinds := map[string]resource.Kind{}
	kindsByGroup := map[string][]resource.Kind{}
	groupVersions := []schema.GroupVersion{}
	for gv, kinds := range kindsByGV {
		for _, kind := range kinds {
			scheme.AddKnownTypeWithName(kind.GroupVersionKind(), kind.ZeroValue())
			scheme.AddKnownTypeWithName(gv.WithKind(kind.Kind()+"List"), kind.ZeroListValue())
			metav1.AddToGroupVersion(scheme, kind.GroupVersionKind().GroupVersion())
			if _, ok := internalKinds[kind.Kind()]; !ok {
				internalKinds[kind.Kind()] = kind
			}
			if _, ok := kindsByGroup[kind.Group()]; !ok {
				kindsByGroup[kind.Group()] = []resource.Kind{}
			}
			kindsByGroup[kind.Group()] = append(kindsByGroup[kind.Group()], kind)
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
	// Copy in the common definitions
	maps.Copy(res, GetCommonOpenAPIDefinitions(callback))
	for _, v := range r.appConfig.ManifestData.Versions {
		for _, manifestKind := range v.Kinds {
			kind, ok := r.managedKindResolver(manifestKind.Kind, v.Name)
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
		}
	}
	return res
}

func (r *defaultInstaller) InstallAPIs(server GenericAPIServer, optsGetter genericregistry.RESTOptionsGetter) error {
	group := r.appConfig.ManifestData.Group
	if r.scheme == nil {
		r.scheme = newScheme()
		r.codecs = serializer.NewCodecFactory(r.scheme)
		if err := r.AddToScheme(r.scheme); err != nil {
			return fmt.Errorf("failed to add to scheme: %w", err)
		}
	}
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(group, r.scheme, metav1.ParameterCodec, r.codecs)

	kindsByGV, err := r.getKindsByGroupVersion()
	if err != nil {
		return fmt.Errorf("failed to get kinds by group version: %w", err)
	}

	for gv, kinds := range kindsByGV {
		for _, kind := range kinds {
			storage := map[string]rest.Storage{}
			s, err := newGenericStoreForKind(r.scheme, kind, optsGetter)
			if err != nil {
				return fmt.Errorf("failed to create store for kind %s: %w", kind.Kind(), err)
			}
			storage[kind.Plural()] = s
			apiGroupInfo.VersionedResourcesStorageMap[gv.Version] = storage
		}
	}

	return server.InstallAPIGroup(&apiGroupInfo)
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
			return &appAdmission{
				appGetter: func() app.App {
					return r.app
				},
				manifestData: r.appConfig.ManifestData,
			}, nil
		}
	}

	return nil
}

func (r *defaultInstaller) App(restConfig clientrest.Config) (app.App, error) {
	if r.app != nil {
		return r.app, nil
	}
	r.appConfig.KubeConfig = restConfig
	a, err := r.appProvider.NewApp(r.appConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create app: %w", err)
	}
	r.app = a
	return a, nil
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

func (r *defaultInstaller) GroupVersions() []schema.GroupVersion {
	groupVersions := []schema.GroupVersion{}
	for _, gv := range r.appConfig.ManifestData.Versions {
		groupVersions = append(groupVersions, schema.GroupVersion{Group: r.appConfig.ManifestData.Group, Version: gv.Name})
	}
	return groupVersions
}

func (r *defaultInstaller) getKindsByGroupVersion() (map[schema.GroupVersion][]resource.Kind, error) {
	out := map[schema.GroupVersion][]resource.Kind{}
	group := r.appConfig.ManifestData.Group
	for _, v := range r.appConfig.ManifestData.Versions {
		for _, manifestKind := range v.Kinds {
			gv := schema.GroupVersion{Group: group, Version: v.Name}
			kind, ok := r.managedKindResolver(manifestKind.Kind, v.Name)
			if !ok {
				return nil, fmt.Errorf("failed to resolve kind %s", manifestKind.Kind)
			}
			out[gv] = append(out[gv], kind)
		}
	}
	return out, nil
}

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
	return scheme
}
