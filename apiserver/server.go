package apiserver

import (
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/admission"
	genericapiserver "k8s.io/apiserver/pkg/server"
	clientrest "k8s.io/client-go/rest"
)

type APIGroupInfoOptions struct {
	Scheme         *runtime.Scheme
	Codecs         serializer.CodecFactory
	ParameterCodec runtime.ParameterCodec
}

type StorageProvider interface {
	StandardStorage(kind resource.Kind, scheme *runtime.Scheme) (StandardStorage, error)
}

type Runner interface {
	operator.Controller
}

type OptionsGetter interface {
	// TODO: this interface should describe something allowing post-start runners to get options, flags, etc.
}

type APIGroupProvider interface {
	// AddToScheme registers all kinds provided by the APIGroupProvider with the provided runtime.Scheme
	AddToScheme(scheme *runtime.Scheme) error
	// APIGroupInfo returns a server.APIGroupInfo object for the API Group described by the object
	APIGroupInfo(provider2 StorageProvider, options APIGroupInfoOptions) (*genericapiserver.APIGroupInfo, error)
	// GetPostStartRunners returns a list of Runners to run after the API server has started
	GetPostStartRunners(generator resource.ClientGenerator, getter OptionsGetter) ([]Runner, error)
	// RegisterAdmissionPlugins registers admission plugins for this API Group with the admission plugin manager
	// TODO: should admission plugins be responsible for unique naming of themselves, or should this function require a naming prefix or a namer function?
	RegisterAdmissionPlugins(plugins *admission.Plugins)
}

// APIServer contains state for a Kubernetes cluster master/api server.
type APIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

// Config defines the config for the apiserver
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

// ExtraConfig holds custom apiserver config
type ExtraConfig struct {
	// Place you custom config here.
	Storage        StorageProvider
	ResourceGroups []APIGroupProvider
	// This is all standard operator config
	KubeConfig *clientrest.Config
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
func (c completedConfig) New() (*APIServer, error) {
	genericServer, err := c.GenericConfig.New("test-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &APIServer{
		GenericAPIServer: genericServer,
	}

	for _, g := range c.ExtraConfig.ResourceGroups {
		apiGroupInfo, err := g.APIGroupInfo(c.ExtraConfig.Storage, APIGroupInfoOptions{})
		if err != nil {
			return nil, err
		}
		if err := s.GenericAPIServer.InstallAPIGroup(apiGroupInfo); err != nil {
			return nil, err
		}
	}

	return s, nil
}
