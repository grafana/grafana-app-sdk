package apiserver

import (
	"bytes"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana/pkg/apimachinery/apis/common/v0alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/server"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/kube-openapi/pkg/common"
)

type Resource struct {
	Kind                  resource.Kind
	GetOpenAPIDefinitions common.GetOpenAPIDefinitions
	Subresources          []SubresourceRoute
	Validator             resource.ValidatingAdmissionController
	Mutator               resource.MutatingAdmissionController
	Reconciler            operator.Reconciler // TODO: do we want this here, or only here for the simple package version?
}

func (r *Resource) AddToScheme(scheme *runtime.Scheme) {
	gv := schema.GroupVersion{
		Group:   r.Kind.Group(),
		Version: r.Kind.Version(),
	}
	scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()), r.Kind.ZeroValue())
	scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()+"List"), r.Kind.ZeroListValue())
	// If there are subresource routes, we need to add the ResourceCallOptions to the scheme for the Connector to work
	if len(r.Subresources) > 0 {
		scheme.AddKnownTypes(gv, &ResourceCallOptions{})
		scheme.AddGeneratedConversionFunc((*url.Values)(nil), (*ResourceCallOptions)(nil), func(a, b interface{}, scope conversion.Scope) error {
			return CovertURLValuesToResourceCallOptions(a.(*url.Values), b.(*ResourceCallOptions), scope)
		})
	}
}

type SubresourceRoute struct {
	// Path is the path _past_ the resource identifier
	// {schema.group}/{schema.version}/{schema.plural}[/ns/{ns}]/{path}
	Path        string
	OpenAPISpec common.GetOpenAPIDefinitions
	Handler     AdditionalRouteHandler
}

type AdditionalRouteHandler func(w http.ResponseWriter, r *http.Request, identifier resource.Identifier)

type ResourceGroup struct {
	Name      string
	Resources []Resource
	// Converters is an optional map of GroupKind => Converter to use for CRD version conversion requests.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Converters map[metav1.GroupKind]k8s.Converter
}

func (g *ResourceGroup) Scheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	g.AddToScheme(scheme)
	return scheme
}

func (g *ResourceGroup) AddToScheme(scheme *runtime.Scheme) {
	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
	for _, r := range g.Resources {
		r.AddToScheme(scheme)
		metav1.AddToGroupVersion(scheme, schema.GroupVersion{
			Group:   r.Kind.Group(),
			Version: r.Kind.Version(),
		})
	}

	// Register conversion functions
	// TODO: necessary?
	for _, r1 := range g.Resources {
		converter, ok := g.Converters[metav1.GroupKind{
			Group: r1.Kind.Group(),
			Kind:  r1.Kind.Kind(),
		}]
		if !ok {
			continue
		}
		for _, r2 := range g.Resources {
			if r1.Kind.Kind() != r2.Kind.Kind() {
				continue
			}

			scheme.AddConversionFunc(r1.Kind.ZeroValue(), r2.Kind.ZeroValue(), schemeConversionFunc(r1.Kind, r2.Kind, converter))
			scheme.AddConversionFunc(r2.Kind.ZeroValue(), r1.Kind.ZeroValue(), schemeConversionFunc(r2.Kind, r1.Kind, converter))
		}
	}
}

type StorageProviderFunc func(resource.Kind, *runtime.Scheme, generic.RESTOptionsGetter) (rest.Storage, error)

type StandardStorage interface {
	rest.StandardStorage
	GetSubresources() map[string]SubresourceStorage
}

type SubresourceStorage interface {
	rest.Storage
	rest.Patcher
}

type StorageProvider2 interface {
	StandardStorage(kind resource.Kind, scheme *runtime.Scheme) (StandardStorage, error)
}

func (g *ResourceGroup) APIGroupInfo(storageProvider StorageProvider2) (*server.APIGroupInfo, error) {
	scheme := g.Scheme()                                // TODO: have this be an argument?
	parameterCodec := runtime.NewParameterCodec(scheme) // TODO: have this be an argument?
	codecs := serializer.NewCodecFactory(scheme)        // TODO: codec based on kinds?
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(g.Name, scheme, parameterCodec, codecs)
	for _, r := range g.Resources {
		plural := strings.ToLower(r.Kind.Plural())
		s, err := storageProvider.StandardStorage(r.Kind, scheme)
		if err != nil {
			return nil, err
		}
		store := map[string]rest.Storage{}
		// Resource storage
		store[plural] = s
		// Subresource storage
		for k, subRoute := range s.GetSubresources() {
			store[fmt.Sprintf("%s/%s", plural, k)] = subRoute
		}

		// Custom subresource routes
		resourceCaller := &SubresourceConnector{
			Routes: r.Subresources,
		}
		for _, subRoute := range r.Subresources {
			store[fmt.Sprintf("%s/%s", plural, subRoute.Path)] = resourceCaller
		}
		apiGroupInfo.VersionedResourcesStorageMap[r.Kind.Version()] = store
	}
	return &apiGroupInfo, nil
}

func schemeConversionFunc(r1, r2 resource.Kind, converter k8s.Converter) func(any, any, conversion.Scope) error {
	// TODO: This has extra allocations, do we want converters to be object -> object rather than bytes -> bytes?
	return func(a, b interface{}, scope conversion.Scope) error {
		fromObj, ok := a.(resource.Object)
		if !ok {
			return fmt.Errorf("from type is not a valid resource.Object")
		}
		fromBytes := &bytes.Buffer{}
		err := r1.Write(fromObj, fromBytes, resource.KindEncodingJSON)
		if err != nil {
			return err
		}
		toObj, ok := b.(resource.Object)
		if !ok {
			return fmt.Errorf("to type is not a valid resource.Object")
		}
		converted, err := converter.Convert(k8s.RawKind{
			Kind:       r1.Kind(),
			APIVersion: fmt.Sprintf("%s/%s", r1.Group(), r1.Version()),
			Group:      r1.Group(),
			Version:    r1.Version(),
			Raw:        fromBytes.Bytes(),
		}, fmt.Sprintf("%s/%s", r1.Group(), r1.Version()))
		if err != nil {
			return err
		}
		return r2.Codec(resource.KindEncodingJSON).Read(bytes.NewReader(converted), toObj)
	}
}

// GetOpenAPIDefinitions combines the provided list of getters and standard grafana and kubernetes OpenAPIDefinitions
// into a single GetOpenAPIDefinitions function which can be used with a kubernetes API Server.
func GetOpenAPIDefinitions(getters []common.GetOpenAPIDefinitions) common.GetOpenAPIDefinitions {
	return func(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
		// TODO: extract v0alpha1 openAPI into app-sdk, or leave in grafana?
		defs := v0alpha1.GetOpenAPIDefinitions(ref) // common grafana apis
		for _, fn := range getters {
			out := fn(ref)
			maps.Copy(defs, out)
		}
		maps.Copy(defs, GetResourceCallOptionsOpenAPIDefinition())
		return defs
	}
}
