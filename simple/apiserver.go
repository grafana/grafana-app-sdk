package simple

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/grafana/grafana-app-sdk/apiserver"
	filestorage "github.com/grafana/grafana/pkg/apiserver/storage/file"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kube-openapi/pkg/common"
	netutils "k8s.io/utils/net"
)

const defaultEtcdPathPrefix = "/registry/grafana.app"

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

func (c completedConfig) NewServer() (*Server, error) {
	scheme := c.ExtraConfig.Scheme
	codecs := c.ExtraConfig.Codecs
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

	parameterCodec := runtime.NewParameterCodec(scheme)
	provider := apiserver.NewRESTStorageProvider(c.GenericConfig.RESTOptionsGetter)
	for _, g := range c.ExtraConfig.ResourceGroups {
		apiGroupInfo, err := g.APIGroupInfo(scheme, codecs, parameterCodec, provider)
		if err != nil {
			return nil, err
		}
		if err := s.GenericAPIServer.InstallAPIGroup(apiGroupInfo); err != nil {
			return nil, err
		}
	}

	return s, nil
}

type APIServerOptions struct {
	RecommendedOptions *genericoptions.RecommendedOptions

	StdOut io.Writer
	StdErr io.Writer

	config *APIServerConfig

	groups []apiserver.ResourceGroup
}

func NewAPIServerOptions(groups []apiserver.ResourceGroup, out, errOut io.Writer) *APIServerOptions {
	serverConfig := NewAPIServerConfig(groups)

	gvs := []schema.GroupVersion{}
	for _, g := range groups {
		for _, r := range g.Resources {
			gv := schema.GroupVersion{
				Group:   r.Kind.Group(),
				Version: r.Kind.Version(),
			}
			gvs = append(gvs, gv)
		}
	}

	o := &APIServerOptions{
		RecommendedOptions: genericoptions.NewRecommendedOptions(
			defaultEtcdPathPrefix,
			serverConfig.ExtraConfig.Codecs.LegacyCodec(gvs...),
		),

		StdOut: out,
		StdErr: errOut,

		config: serverConfig,
		groups: groups,
	}

	o.RecommendedOptions.Admission.Plugins = admission.NewPlugins()
	for gid, g := range groups {
		for rid, _ := range g.Resources {
			r := &groups[gid].Resources[rid]
			r.RegisterAdmissionPlugin(o.RecommendedOptions.Admission.Plugins)
		}
	}

	o.RecommendedOptions.Admission.RecommendedPluginOrder = o.RecommendedOptions.Admission.Plugins.Registered()
	o.RecommendedOptions.Admission.EnablePlugins = o.RecommendedOptions.Admission.Plugins.Registered()
	o.RecommendedOptions.Admission.DisablePlugins = []string{}
	o.RecommendedOptions.Admission.DefaultOffPlugins = sets.NewString()
	return o
}

// NewCommandStartAPIServer provides a CLI handler for starting a simple API server.
func NewCommandStartAPIServer(o *APIServerOptions, stopCh <-chan struct{}) *cobra.Command {
	cmd := &cobra.Command{
		Short: "Launch an API server",
		Long:  "Launch an API server",
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Validate(args); err != nil {
				return err
			}
			return o.Run(stopCh)
		},
	}

	flags := cmd.Flags()
	o.RecommendedOptions.AddFlags(flags)
	utilfeature.DefaultMutableFeatureGate.AddFlag(flags)

	return cmd
}

// Validate validates APIServerOptions
func (o *APIServerOptions) Validate(args []string) error {
	errors := []error{}
	errors = append(errors, o.RecommendedOptions.SecureServing.Validate()...)
	return utilerrors.NewAggregate(errors)
}

// Config returns config for the api server given APIServerOptions
func (o *APIServerOptions) Config() (*APIServerConfig, error) {
	// TODO: this all needs to change to not be local/self-signed in the future
	serverConfig := o.config
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", []string{}, []net.IP{netutils.ParseIPSloppy("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	if err := o.RecommendedOptions.SecureServing.ApplyTo(&serverConfig.GenericConfig.SecureServing, &serverConfig.GenericConfig.LoopbackClientConfig); err != nil {
		return nil, err
	}

	fakev1Informers := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 10*time.Minute)
	if err := o.RecommendedOptions.Admission.ApplyTo(&serverConfig.GenericConfig.Config, fakev1Informers, fake.NewSimpleClientset(), nil, nil); err != nil {
		return nil, err
	}

	o.RecommendedOptions.Etcd.EnableWatchCache = false
	//o.RecommendedOptions.Etcd.StorageConfig.Transport.ServerList = []string{"127.0.0.1:2379"}

	if err := o.RecommendedOptions.Etcd.ApplyTo(&serverConfig.GenericConfig.Config); err != nil {
		return nil, err
	}
	// override the default storage with file storage
	restStorage, err := filestorage.NewRESTOptionsGetter("./.data", o.RecommendedOptions.Etcd.StorageConfig)
	if err != nil {
		panic(err)
	}
	serverConfig.GenericConfig.RESTOptionsGetter = restStorage
	serverConfig.GenericConfig.AddPostStartHook("start-resource-informers", apiserver.ReconcilersPostStartHook(o.groups...))

	return serverConfig, nil
}

func (o *APIServerOptions) Run(stopCh <-chan struct{}) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().NewServer()
	if err != nil {
		return err
	}

	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}
