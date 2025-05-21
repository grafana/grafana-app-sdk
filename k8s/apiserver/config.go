package apiserver

import (
	"maps"

	"github.com/grafana/grafana-app-sdk/app"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/util/compatibility"
	"k8s.io/kube-openapi/pkg/common"
)

type Config struct {
	Generic    *genericapiserver.RecommendedConfig
	scheme     *runtime.Scheme
	codecs     serializer.CodecFactory
	installers []apiServerInstaller
}

func NewConfig(installers []apiServerInstaller) (*Config, error) {
	scheme := newScheme()
	codecs := serializer.NewCodecFactory(scheme)
	c := &Config{
		scheme:     scheme,
		codecs:     codecs,
		installers: installers,
	}
	if err := c.AddToScheme(); err != nil {
		return nil, err
	}
	c.Generic = genericapiserver.NewRecommendedConfig(codecs)
	return c, nil
}

func (c *Config) AddToScheme() error {
	for _, installer := range c.installers {
		if err := installer.AddToScheme(c.scheme); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) UpdateOpenAPIConfig() {
	defGetter := func(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition {
		res := make(map[string]common.OpenAPIDefinition)
		for _, installer := range c.installers {
			maps.Copy(res, installer.GetOpenAPIDefinitions(callback))
		}
		return res
	}

	c.Generic.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(defGetter, openapi.NewDefinitionNamer(c.scheme))
	c.Generic.OpenAPIConfig.Info.Title = "Core"
	c.Generic.OpenAPIConfig.Info.Version = "1.0"

	c.Generic.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(defGetter, openapi.NewDefinitionNamer(c.scheme))
	c.Generic.OpenAPIV3Config.Info.Title = "Core"
	c.Generic.OpenAPIV3Config.Info.Version = "1.0"
}

func (c *Config) NewServer(delegate genericapiserver.DelegationTarget) (*genericapiserver.GenericAPIServer, error) {
	manifests := make([]app.ManifestData, 0)
	for _, installer := range c.installers {
		manifests = append(manifests, installer.appConfig.ManifestData)
		_, err := installer.App(*c.Generic.LoopbackClientConfig)
		if err != nil {
			return nil, err
		}
	}
	c.Generic.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(c.openAPIDefinitionGetterFunc(manifests), openapi.NewDefinitionNamer(c.scheme))
	c.Generic.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(c.openAPIDefinitionGetterFunc(manifests), openapi.NewDefinitionNamer(c.scheme))
	c.Generic.EffectiveVersion = compatibility.DefaultBuildEffectiveVersion()
	completedConfig := c.Generic.Complete()
	return completedConfig.New("grafana-app-sdk", delegate)
}

func (c *Config) openAPIDefinitionGetterFunc(manifests []app.ManifestData) func(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return func(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition {
		res := make(map[string]common.OpenAPIDefinition)
		for _, m := range manifests {
			for _, version := range m.Versions {
				for _, kind := range version.Kinds {
					oapi, err := kind.Schema.AsKubeOpenAPI(schema.GroupVersionKind{
						Group:   m.Group,
						Version: version.Name,
						Kind:    kind.Kind,
					}, callback)
					if err != nil {
						continue
					}
					maps.Copy(res, oapi)
				}
			}
		}
		return res
	}
}
