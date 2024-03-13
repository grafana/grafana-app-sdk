package simple

import (
	"maps"
	"net/http"

	"github.com/grafana/grafana-app-sdk/apiserver"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana/pkg/apimachinery/apis/common/v0alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	clientrest "k8s.io/client-go/rest"
	"k8s.io/kube-openapi/pkg/common"
)

type APIServerResource struct {
	Kind                  resource.Kind
	GetOpenAPIDefinitions common.GetOpenAPIDefinitions
	Subresources          []SubresourceRoute
	Validator             resource.ValidatingAdmissionController
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
	Name     string
	Resource []APIServerResource
	// Converters is an optional map of GroupKind => Converter to use for CRD version conversion requests.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Converters map[metav1.GroupKind]k8s.Converter
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
	for _, r := range g.Resource {
		r.AddToScheme(scheme)
		metav1.AddToGroupVersion(scheme, schema.GroupVersion{
			Group:   r.Kind.Group(),
			Version: r.Kind.Version(),
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

// This should eventually live in grafana-app-sdk
func GetOpenAPIDefinitions(getters []common.GetOpenAPIDefinitions) common.GetOpenAPIDefinitions {
	return func(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
		defs := v0alpha1.GetOpenAPIDefinitions(ref) // common grafana apis
		for _, fn := range getters {
			out := fn(ref)
			maps.Copy(defs, out)
		}
		return defs
	}
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

	openapiGetters := []common.GetOpenAPIDefinitions{}
	for _, g := range c.ExtraConfig.ResourceGroups {
		for _, r := range g.Resource {
			gv := schema.GroupVersion{
				Group:   r.Kind.Group(),
				Version: r.Kind.Version(),
			}
			scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()), r.Kind.ZeroValue())
			scheme.AddKnownTypeWithName(gv.WithKind(r.Kind.Kind()+"List"), &resource.UntypedList{})
			metav1.AddToGroupVersion(scheme, gv)
			openapiGetters = append(openapiGetters, r.GetOpenAPIDefinitions)
		}
	}

	c.GenericConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(GetOpenAPIDefinitions(openapiGetters), openapi.NewDefinitionNamer(scheme))
	c.GenericConfig.OpenAPIConfig.Info.Title = "Core"
	c.GenericConfig.OpenAPIConfig.Info.Version = "1.0"

	c.GenericConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(GetOpenAPIDefinitions(openapiGetters), openapi.NewDefinitionNamer(scheme))
	c.GenericConfig.OpenAPIV3Config.Info.Title = "Core"
	c.GenericConfig.OpenAPIV3Config.Info.Version = "1.0"

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
			s, err := apiserver.NewRESTStorage(scheme, r.Kind, c.GenericConfig.RESTOptionsGetter)
			if err != nil {
				return nil, err
			}
			store := map[string]rest.Storage{}
			store[r.Kind.Plural()] = s
			apiGroupInfo.VersionedResourcesStorageMap[r.Kind.Version()] = store
		}
		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}

	return s, nil
}
