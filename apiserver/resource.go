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

// TODO: should this be different from the k8s.Converter? Using the k8s.Converter means we need extra allocations when working with the runtime.Object apimachinery supplies
type Converter interface {
	k8s.Converter
}

type GenericConverter struct{}

func (GenericConverter) Convert(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
	codec := resource.NewJSONCodec()
	into := &resource.UntypedObject{}
	err := codec.Read(bytes.NewReader(obj.Raw), into)
	if err != nil {
		return nil, err
	}
	into.SetGroupVersionKind(schema.FromAPIVersionAndKind(targetAPIVersion, obj.Kind))
	buf := bytes.Buffer{}
	err = codec.Write(&buf, into)
	return buf.Bytes(), err
}

type ResourceGroup struct {
	Name      string
	Resources []Resource
	// Converters is an optional map of GroupKind => Converter to use for CRD version conversion requests.
	// This SHOULD be supplied if multiple versions of the same GroupKind exist in the ResourceGroup.
	// If not supplied, a GenericConverter will be used for all conversions.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Converters map[metav1.GroupKind]Converter
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

	// TODO: this assumes items in the Resources slice are ordered by version
	versions := make(map[metav1.GroupKind][]Resource)
	for _, r := range g.Resources {
		gk := metav1.GroupKind{Group: r.Kind.Group(), Kind: r.Kind.Kind()}
		list, ok := versions[gk]
		if !ok {
			list = make([]Resource, 0)
		}
		list = append(list, r)
		versions[gk] = list
	}

	for gk, vers := range versions {
		// Create an internal version which is set as the latest version in the list for each distinct GroupKind
		latest := vers[len(vers)-1]
		gv := schema.GroupVersion{
			Group:   latest.Kind.Group(),
			Version: runtime.APIVersionInternal,
		}
		scheme.AddKnownTypeWithName(gv.WithKind(gk.Kind), latest.Kind.ZeroValue())
		scheme.AddKnownTypeWithName(gv.WithKind(gk.Kind+"List"), latest.Kind.ZeroListValue())

		// Get the converter for this GroupKind, or use a Generic one if none was supplied
		var converter Converter = GenericConverter{}
		if g.Converters != nil {
			ok := false
			converter, ok = g.Converters[gk]
			if !ok {
				converter = GenericConverter{}
			}
		}

		// Register each added version with the scheme
		priorities := make([]schema.GroupVersion, len(vers))
		for i, v := range vers {
			groupVersion := schema.GroupVersion{
				Group:   v.Kind.Group(),
				Version: v.Kind.Version(),
			}
			v.AddToScheme(scheme)
			metav1.AddToGroupVersion(scheme, groupVersion)
			priorities[len(priorities)-1-i] = groupVersion

			// Also register converters
			for _, v2 := range vers {
				if v.Kind.Version() == v2.Kind.Version() {
					continue
				}
				scheme.AddConversionFunc(v.Kind.ZeroValue(), v2.Kind.ZeroValue(), schemeConversionFunc(v.Kind, v2.Kind, converter))
				scheme.AddConversionFunc(v2.Kind.ZeroValue(), v.Kind.ZeroValue(), schemeConversionFunc(v2.Kind, v.Kind, converter))
			}
		}

		// Set version priorities based on the reverse-order list we built
		scheme.SetVersionPriority(priorities...)
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
		}, fmt.Sprintf("%s/%s", r2.Group(), r2.Version()))
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
