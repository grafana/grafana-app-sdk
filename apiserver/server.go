package apiserver

import (
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	clientrest "k8s.io/client-go/rest"
)

// APIServer contains state for a Kubernetes cluster master/api server.
type APIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type StorageProvider interface {
	StandardStorageFor(kind resource.Kind) rest.StandardStorage
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
	ResourceGroups []APIServerGroup
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
		apiGroupInfo := g.APIGroupInfo(c.ExtraConfig.Storage.StandardStorageFor)
		if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
			return nil, err
		}
	}

	return s, nil
}
