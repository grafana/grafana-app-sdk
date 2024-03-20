package simple

import (
	"net/http"

	"github.com/grafana/grafana-app-sdk/apiserver"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/kube-openapi/pkg/common"
)

type APIServerGroup struct {
	Name     string
	Resource []apiserver.Resource
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

// ExtraConfig holds custom apiserver config
type ExtraConfig struct {
	ResourceGroups []apiserver.ResourceGroup
	Scheme         *runtime.Scheme
	Codecs         serializer.CodecFactory
}

// APIServerConfig defines the config for the apiserver
type APIServerConfig struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

func NewAPIServerConfig(groups []apiserver.ResourceGroup) *APIServerConfig {
	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)

	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})

	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)

	// Add each ResourceGroup to the scheme
	for _, g := range groups {
		g.AddToScheme(scheme)
	}

	return &APIServerConfig{
		GenericConfig: genericapiserver.NewRecommendedConfig(codecs),
		ExtraConfig: ExtraConfig{
			ResourceGroups: groups,
			Scheme:         scheme,
			Codecs:         codecs,
		},
	}
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// CompletedAPIServerConfig embeds a private pointer that cannot be instantiated outside of this package.
type CompletedAPIServerConfig struct {
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *APIServerConfig) Complete() CompletedAPIServerConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		&cfg.ExtraConfig,
	}

	c.GenericConfig.Version = &version.Info{
		Major: "1",
		Minor: "0",
	}

	return CompletedAPIServerConfig{&c}
}

type Server struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

func (c completedConfig) New() (*Server, error) {
	//scheme := c.ExtraConfig.Scheme
	//codecs := c.ExtraConfig.Codecs
	openapiGetters := []common.GetOpenAPIDefinitions{}

	for _, g := range c.ExtraConfig.ResourceGroups {
		for _, r := range g.Resources {
			openapiGetters = append(openapiGetters, r.GetOpenAPIDefinitions)
		}
	}

	c.GenericConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(apiserver.GetOpenAPIDefinitions(openapiGetters), openapi.NewDefinitionNamer(c.ExtraConfig.Scheme))
	c.GenericConfig.OpenAPIConfig.Info.Title = "Core"
	c.GenericConfig.OpenAPIConfig.Info.Version = "1.0"

	c.GenericConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(apiserver.GetOpenAPIDefinitions(openapiGetters), openapi.NewDefinitionNamer(c.ExtraConfig.Scheme))
	c.GenericConfig.OpenAPIV3Config.Info.Title = "Core"
	c.GenericConfig.OpenAPIV3Config.Info.Version = "1.0"

	genericServer, err := c.GenericConfig.New("apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &Server{
		GenericAPIServer: genericServer,
	}

	//parameterCodec := runtime.NewParameterCodec(scheme)
	provider := apiserver.NewRESTStorageProvider(c.GenericConfig.RESTOptionsGetter)
	for _, g := range c.ExtraConfig.ResourceGroups {
		/*apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(g.Name, scheme, parameterCodec, codecs)
		for _, r := range g.Resources {
			plural := strings.ToLower(r.Kind.Plural())
			s, err := apiserver.NewRESTStorage(scheme, r.Kind, c.GenericConfig.RESTOptionsGetter)
			if err != nil {
				return nil, err
			}
			store := map[string]rest.Storage{}
			store[plural] = s
			// Custom subresource routes
			resourceCaller := &apiserver.SubresourceConnector{
				Routes: r.Subresources,
			}
			for _, subRoute := range r.Subresources {
				store[fmt.Sprintf("%s/%s", plural, subRoute.Path)] = resourceCaller
			}
			apiGroupInfo.VersionedResourcesStorageMap[r.Kind.Version()] = store
		}*/
		apiGroupInfo, err := g.APIGroupInfo(provider)
		if err != nil {
			return nil, err
		}
		if err := s.GenericAPIServer.InstallAPIGroup(apiGroupInfo); err != nil {
			return nil, err
		}
	}

	return s, nil
}
