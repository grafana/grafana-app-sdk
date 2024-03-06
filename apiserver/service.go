package apiserver

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/go-openapi/spec"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/kube-openapi/pkg/spec3"
)

type APIServerResource struct {
	Kind         resource.Kind
	OpenAPISpec  spec.Schema // TODO: add generation of spec.Schema to the codegen for a kind
	Subresources []SubresourceRoute
	Validator    resource.ValidatingAdmissionController
	// Mutators is an optional map of schema => MutatingAdmissionController to use for the schema on admission.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Mutator    resource.MutatingAdmissionController
	Reconciler operator.Reconciler
}

func (r *APIServerResource) AddToScheme(scheme *runtime.Scheme) {
	gv := schema.GroupVersion{
		Group:   r.Kind.Group(),
		Version: r.Kind.Version(),
	}
	scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()), r.Kind.ZeroValue())
	scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()+"List"), &resource.UntypedList{})
}

type APIServerGroup struct {
	Name      string
	Resources []APIServerResource
	// Converters is an optional map of GroupKind => Converter to use for CRD version conversion requests.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Converters map[metav1.GroupKind]k8s.Converter
}

type SubresourceRoute struct {
	// Path is the path _past_ the resource identifier
	// {schema.group}/{schema.version}/{schema.plural}[/ns/{ns}]/{path}
	Path        string
	OpenAPISpec *spec3.PathProps // Exposed in the open api service discovery
	Handler     AdditionalRouteHandler
}

type AdditionalRouteHandler func(w http.ResponseWriter, r *http.Request, identifier resource.Identifier)

// An APIService is a set of one or more GroupVersions with the same Group, which can be run as a part of
// an APIServer.
type APIService interface {
	Group() string
	APIGroups() []server.APIGroupInfo
}

func (g *APIServerGroup) Scheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	g.AddToScheme(scheme)
	return scheme
}

func (g *APIServerGroup) AddToScheme(scheme *runtime.Scheme) {
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

			scheme.AddConversionFunc(r1.Kind.ZeroValue(), r2.Kind.ZeroValue(), g.schemeConversionFunc(r1.Kind, r2.Kind, converter))
			scheme.AddConversionFunc(r2.Kind.ZeroValue(), r1.Kind.ZeroValue(), g.schemeConversionFunc(r2.Kind, r1.Kind, converter))
		}
	}
}

func (g *APIServerGroup) GetStorage(objectStorageFunc func(resource.Kind) rest.StandardStorage) map[string]map[string]rest.Storage {
	storage := make(map[string]map[string]rest.Storage)
	for _, r := range g.Resources {
		store := map[string]rest.Storage{}
		store[r.Kind.Plural()] = objectStorageFunc(r.Kind)
		// Extra routes
		if len(r.Subresources) > 0 {
			// TODO: caller per-subresourceRoute?
			resourceCaller := &SubresourceConnector{
				Routes: r.Subresources,
			}
			for _, subRoute := range r.Subresources {
				store[fmt.Sprintf("%s/%s", r.Kind.Plural(), subRoute.Path)] = resourceCaller
			}
		}
		storage[r.Kind.Version()] = store
	}
	return storage
}

func (g *APIServerGroup) APIGroupInfo(storageProvider func(kind resource.Kind) rest.StandardStorage) server.APIGroupInfo {
	scheme := g.Scheme()
	// Codecs provides methods for retrieving codecs and serializers for specific
	// versions and content types.
	// TODO: custom codec factory using k8s.KindNegotiatedSerializer
	codecs := serializer.NewCodecFactory(scheme)
	// TODO: custom ParameterCodec?
	parameterCodec := runtime.NewParameterCodec(scheme)

	return server.APIGroupInfo{
		PrioritizedVersions:          scheme.PrioritizedVersionsForGroup(g.Name),
		VersionedResourcesStorageMap: g.GetStorage(storageProvider),
		// TODO unhardcode this.  It was hardcoded before, but we need to re-evaluate
		OptionsExternalVersion: &schema.GroupVersion{Version: "v1"},
		Scheme:                 scheme,
		ParameterCodec:         parameterCodec,
		NegotiatedSerializer:   codecs,
	}
}

func (*APIServerGroup) schemeConversionFunc(r1, r2 resource.Kind, converter k8s.Converter) func(any, any, conversion.Scope) error {
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
