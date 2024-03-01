package simple

import (
	"net/http"

	"github.com/go-openapi/spec"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	clientrest "k8s.io/client-go/rest"
	"k8s.io/kube-openapi/pkg/spec3"
)

type APIServerResource struct {
	Kind         resource.Schema
	OpenAPISpec  spec.Schema // TODO: add generation of spec.Schema to the codegen for a kind
	Subresources []SubresourceRoute
	Validator    resource.ValidatingAdmissionController
	// Mutators is an optional map of schema => MutatingAdmissionController to use for the schema on admission.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Mutator    resource.MutatingAdmissionController
	Reconciler operator.Reconciler
}

type APIServerGroup struct {
	Name     string
	Resource []APIServerResource
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

type Storage struct {
	rest.StandardStorage
}

type APIServerConfig struct {
}

// ExtraConfig holds custom apiserver config
type ExtraConfig struct {
	// Place you custom config here.
	Storage        Storage
	ResourceGroups []APIServerGroup
	// This is all standard operator config
	KubeConfig *clientrest.Config
}

// Config defines the config for the apiserver
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

// ExampleServer contains state for a Kubernetes cluster master/api server.
type ExampleServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// CompletedConfig embeds a private pointer that cannot be instantiated outside of this package.
type CompletedConfig struct {
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		&cfg.ExtraConfig,
	}

	c.GenericConfig.Version = &version.Info{
		Major: "1",
		Minor: "0",
	}

	return CompletedConfig{&c}
}

// New returns a new instance of ExampleServer from the given config.
func (c completedConfig) New() (*ExampleServer, error) {

	scheme := runtime.NewScheme()
	// Codecs provides methods for retrieving codecs and serializers for specific
	// versions and content types.
	codecs := serializer.NewCodecFactory(scheme)

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

	for _, g := range c.ExtraConfig.ResourceGroups {
		for _, r := range g.Resource {
			gv := schema.GroupVersion{
				Group:   r.Kind.Group(),
				Version: r.Kind.Version(),
			}
			scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()), r.Kind.ZeroValue())
			scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()+"List"), &resource.UntypedList{})
			metav1.AddToGroupVersion(scheme, gv)
		}
	}

	genericServer, err := c.GenericConfig.New("test-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &ExampleServer{
		GenericAPIServer: genericServer,
	}

	parameterCodec := runtime.NewParameterCodec(scheme)

	for _, g := range c.ExtraConfig.ResourceGroups {
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(g.Name, scheme, parameterCodec, codecs)
		for _, r := range g.Resource {
			store := map[string]rest.Storage{}
			store[r.Kind.Plural()] = NewStorage(&r)
			apiGroupInfo.VersionedResourcesStorageMap[r.Kind.Version()] = store
		}
		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func NewStorage(r *APIServerResource) rest.StandardStorage {
	return nil
}
