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

type ManagedKindResolver func(kind, version string) (resource.Kind, error)

type APIServerInstaller struct {
	appProvider         app.Provider
	appConfig           app.Config
	managedKindResolver ManagedKindResolver

	app    app.App
	scheme *runtime.Scheme
	codecs serializer.CodecFactory
}

func NewApIServerInstaller(appProvider app.Provider, appConfig app.Config, kindResolver ManagedKindResolver) (*APIServerInstaller, error) {
	installer := &APIServerInstaller{
		appProvider:         appProvider,
		appConfig:           appConfig,
		managedKindResolver: kindResolver,
	}
	return installer, nil
}

func (r *APIServerInstaller) AddToScheme(scheme *runtime.Scheme) error {
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
			scheme.AddConversionFunc(kind.ZeroValue(), internalKind.ZeroValue(), r.conversionHandler)
			scheme.AddConversionFunc(internalKind.ZeroValue(), kind.ZeroValue(), r.conversionHandler)
		}
	}

	sort.Slice(groupVersions, func(i, j int) bool {
		return version.CompareKubeAwareVersionStrings(groupVersions[i].Version, groupVersions[j].Version) < 0
	})
	scheme.SetVersionPriority(groupVersions...)

	// save the scheme for later use
	if r.scheme == nil {
		r.scheme = scheme
		r.codecs = serializer.NewCodecFactory(scheme)
	}

	return nil
}

func (r *APIServerInstaller) GetOpenAPIDefinitions(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	res := map[string]common.OpenAPIDefinition{}
	for _, v := range r.appConfig.ManifestData.Versions {
		for _, manifestKind := range v.Kinds {
			kind, err := r.managedKindResolver(manifestKind.Kind, v.Name)
			if err != nil {
				continue
			}
			oapi, err := manifestKind.Schema.AsKubeOpenAPI(kind.GroupVersionKind(), callback)
			if err != nil {
				continue
			}
			maps.Copy(res, oapi)
		}
	}
	return res
}

func (r *APIServerInstaller) InstallAPIs(server *genericapiserver.GenericAPIServer, optsGetter genericregistry.RESTOptionsGetter) error {
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
			storage[kind.Kind()] = s
			apiGroupInfo.VersionedResourcesStorageMap[gv.String()] = storage
		}
	}

	server.InstallAPIGroup(&apiGroupInfo)

	return nil
}

func (r *APIServerInstaller) AdmissionPlugin() (string, admission.Factory) {

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
		pluginName := r.appProvider.Manifest().ManifestData.AppName + " admission"
		return pluginName, func(_ io.Reader) (admission.Interface, error) {
			if r.app == nil {
				return nil, fmt.Errorf("app is not initialized")
			}
			return &appAdmission{
				app: r.app,
			}, nil
		}
	}

	return "", nil
}

func (r *APIServerInstaller) App(restConfig clientrest.Config) (app.App, error) {
	if r.app != nil {
		return r.app, nil
	}
	r.appConfig.KubeConfig = restConfig
	app, err := r.appProvider.NewApp(r.appConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create app: %w", err)
	}
	r.app = app
	return app, nil
}

func (r *APIServerInstaller) admissionHandler(supportsMutation, supportsValidation bool) func(a admission.Attributes, o admission.ObjectInterfaces) error {
	return func(a admission.Attributes, o admission.ObjectInterfaces) error {

		return nil
	}
}

func (r *APIServerInstaller) conversionHandler(a, b interface{}, scope conversion.Scope) error {
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

func (r *APIServerInstaller) getKindsByGroupVersion() (map[schema.GroupVersion][]resource.Kind, error) {
	out := map[schema.GroupVersion][]resource.Kind{}
	group := r.appConfig.ManifestData.Group
	for _, v := range r.appConfig.ManifestData.Versions {
		for _, manifestKind := range v.Kinds {
			gv := schema.GroupVersion{Group: group, Version: v.Name}
			kind, err := r.managedKindResolver(manifestKind.Kind, v.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve kind %s: %w", manifestKind.Kind, err)
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
