package apiserver

import (
	"context"
	"maps"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	genericregistry "k8s.io/apiserver/pkg/registry/generic"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/routes"
	"k8s.io/kube-openapi/pkg/common"
)

type RunnerConfig struct {
	// TODO: full apiserver config
	RESTOptionsGetter genericregistry.RESTOptionsGetter
}

// Runner allows running one or more Apps as an apiserver
type Runner struct {
	cfg        RunnerConfig
	scheme     *runtime.Scheme
	server     *genericapiserver.GenericAPIServer
	installed  []APIServerInstaller
	running    bool
	runningCtx context.Context
	runningMux sync.Mutex
}

func NewRunner(config RunnerConfig) (*Runner, error) {
	return &Runner{
		cfg:       config,
		installed: make([]APIServerInstaller, 0),
	}, nil
}

// Run starts a new apiserver or adds the installers to an existing running apiserver
// TODO: if updating an existing server becomes too complicated, abandon that behavior
func (r *Runner) Run(ctx context.Context, installers ...APIServerInstaller) error {
	r.runningMux.Lock()
	if r.scheme == nil {
		r.scheme = runtime.NewScheme()
		// Add standard metadata and unversioned stuff
		metav1.AddToGroupVersion(r.scheme, schema.GroupVersion{Version: "v1"})

		unversioned := schema.GroupVersion{Group: "", Version: "v1"}
		r.scheme.AddUnversionedTypes(unversioned,
			&metav1.Status{},
			&metav1.APIVersions{},
			&metav1.APIGroupList{},
			&metav1.APIGroup{},
			&metav1.APIResourceList{},
		)
	}

	// Install the installer to the scheme
	for _, installer := range installers {
		err := installer.AddToScheme(r.scheme)
		if err != nil {
			return err
		}
	}

	if r.server == nil || !r.running {
		// Build the apiserver
		codecs := serializer.NewCodecFactory(r.scheme)
		cfg := genericapiserver.NewRecommendedConfig(codecs)
		cfg.RESTOptionsGetter = r.cfg.RESTOptionsGetter
		// TODO: other config values (from RunnerConfig?)

		completedConfig := cfg.Complete()

		completedConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(r.openAPIDefinitionGetterFunc(append(r.installed, installers...)), openapi.NewDefinitionNamer(r.scheme))
		completedConfig.OpenAPIConfig.Info.Title = "Core"
		completedConfig.OpenAPIConfig.Info.Version = "1.0"

		completedConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(r.openAPIDefinitionGetterFunc(append(r.installed, installers...)), openapi.NewDefinitionNamer(r.scheme))
		completedConfig.OpenAPIV3Config.Info.Title = "Core"
		completedConfig.OpenAPIV3Config.Info.Version = "1.0"

		var err error
		r.server, err = completedConfig.New("apiserver", genericapiserver.NewEmptyDelegate())
		if err != nil {
			return err
		}
	}

	// Install the installer to the API server
	for _, installer := range installers {
		err := installer.InstallAPIs(r.server, r.cfg.RESTOptionsGetter)
		if err != nil {
			return err
		}
	}

	// TODO: post-start-hook for app runners
	// should that be in APIServerInstaller, since it has app awareness?

	r.installed = append(r.installed, installers...)

	// Only run the server if it isn't already running
	if !r.running {
		r.running = true
		r.runningCtx = ctx
		r.runningMux.Unlock()
		err := r.server.PrepareRun().RunWithContext(ctx)
		r.running = false
		return err
	} else {
		// Update the OpenAPI in the running apiserver
		r.server.OpenAPIVersionedService, r.server.StaticOpenAPISpec = routes.OpenAPI{
			Config: genericapiserver.DefaultOpenAPIConfig(r.openAPIDefinitionGetterFunc(installers), openapi.NewDefinitionNamer(r.scheme)),
		}.InstallV2(r.server.Handler.GoRestfulContainer, r.server.Handler.NonGoRestfulMux)

		r.server.OpenAPIV3VersionedService = routes.OpenAPI{
			V3Config: genericapiserver.DefaultOpenAPIV3Config(r.openAPIDefinitionGetterFunc(installers), openapi.NewDefinitionNamer(r.scheme)),
		}.InstallV3(r.server.Handler.GoRestfulContainer, r.server.Handler.NonGoRestfulMux)

		// Wait for the running apiserver to exit
		r.runningMux.Unlock()
		<-r.runningCtx.Done()
	}

	return nil
}

func (r *Runner) openAPIDefinitionGetterFunc(installed []APIServerInstaller) func(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return func(callback common.ReferenceCallback) map[string]common.OpenAPIDefinition {
		res := make(map[string]common.OpenAPIDefinition)
		for _, installer := range installed {
			if installer.AppProvider.Manifest().ManifestData == nil {
				continue
			}
			for _, kind := range installer.AppProvider.Manifest().ManifestData.Kinds {
				for _, version := range kind.Versions {
					oapi, err := version.Schema.AsKubeOpenAPI(schema.GroupVersionKind{
						Group:   installer.AppProvider.Manifest().ManifestData.Group,
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
