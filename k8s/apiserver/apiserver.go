package apiserver

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	genericregistry "k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
)

type APIServerInstaller struct {
	AppProvider         app.Provider
	AppConfig           app.Config
	ManagedKindResolver func(kind, version string) (resource.Kind, error)

	app    app.App
	scheme *runtime.Scheme
	codecs serializer.CodecFactory
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

	internalGv := schema.GroupVersion{Group: r.AppConfig.ManifestData.Group, Version: runtime.APIVersionInternal}
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

func (r *APIServerInstaller) InstallAPIs(server *genericapiserver.GenericAPIServer, optsGetter genericregistry.RESTOptionsGetter) error {
	group := r.AppConfig.ManifestData.Group
	if r.scheme == nil {
		scheme := runtime.NewScheme()
		if err := r.AddToScheme(scheme); err != nil {
			return fmt.Errorf("failed to add to scheme: %w", err)
		}
	}
	codecs := serializer.NewCodecFactory(r.scheme)

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(group, r.scheme, metav1.ParameterCodec, codecs)

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

	req := app.ConversionRequest{
		SourceGVK: aResourceObj.GroupVersionKind(),
		TargetGVK: bResourceObj.GroupVersionKind(),
		Raw: app.RawObject{
			// TODO set raw bytes and encoding
			Object: aResourceObj,
		},
	}
	res, err := r.app.Convert(context.TODO(), req)
	if err != nil {
		return fmt.Errorf("failed to convert object %s from %s to %s: %w", aResourceObj.GetName(), req.SourceGVK, req.TargetGVK, err)
	}

	dest, err := conversion.EnforcePtr(b)
	if err != nil {
		return fmt.Errorf("failed to enforce ptr: %w", err)
	}

	source, err := conversion.EnforcePtr(res.Object)
	if err != nil {
		return fmt.Errorf("failed to enforce ptr: %w", err)
	}

	dest.Set(source)

	return nil
}

func (r *APIServerInstaller) getKindsByGroupVersion() (map[schema.GroupVersion][]resource.Kind, error) {
	out := map[schema.GroupVersion][]resource.Kind{}
	group := r.AppConfig.ManifestData.Group
	for _, manifestKind := range r.AppConfig.ManifestData.Kinds {
		for _, v := range manifestKind.Versions {
			gv := schema.GroupVersion{Group: group, Version: v.Name}
			kind, err := r.ManagedKindResolver(manifestKind.Kind, v.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve kind %s: %w", manifestKind.Kind, err)
			}
			out[gv] = append(out[gv], kind)
		}
	}
	return out, nil
}
