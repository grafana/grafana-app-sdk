/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/grafana/grafana-app-sdk/apiserver"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"
	filestorage "github.com/grafana/grafana/pkg/apiserver/storage/file"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	netutils "k8s.io/utils/net"
)

const defaultEtcdPathPrefix = "/registry/grafana.app"

type APIServerOptions struct {
	RecommendedOptions *genericoptions.RecommendedOptions

	StdOut io.Writer
	StdErr io.Writer

	config *simple.APIServerConfig

	groups []apiserver.ResourceGroup
}

func NewAPIServerOptions(groups []apiserver.ResourceGroup, out, errOut io.Writer) *APIServerOptions {
	serverConfig := simple.NewAPIServerConfig(groups)

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
	return o
}

// NewCommandStartAPIServer provides a CLI handler for starting the API server.
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
func (o *APIServerOptions) Config() (*simple.APIServerConfig, error) {
	serverConfig := o.config
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", []string{}, []net.IP{netutils.ParseIPSloppy("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	if err := o.RecommendedOptions.SecureServing.ApplyTo(&serverConfig.GenericConfig.SecureServing, &serverConfig.GenericConfig.LoopbackClientConfig); err != nil {
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

	return serverConfig, nil
}

func (o *APIServerOptions) Run(stopCh <-chan struct{}) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New()
	if err != nil {
		return err
	}

	server.GenericAPIServer.AddPostStartHook("start-resource-informers", func(ctx genericapiserver.PostStartHookContext) error {
		if ctx.LoopbackClientConfig == nil {
			return fmt.Errorf("missing LoopbackClientConfig from PostStartHookContext")
		}
		ctx.LoopbackClientConfig.Host = strings.Replace(ctx.LoopbackClientConfig.Host, "[::1]", "127.0.0.1", 1) // This is [::1] by default?
		ctx.LoopbackClientConfig.APIPath = "/apis"                                                              // empty by default?
		clientRegistry := k8s.NewClientRegistry(*ctx.LoopbackClientConfig, k8s.DefaultClientConfig())
		controller := operator.NewInformerController(operator.DefaultInformerControllerConfig())
		for _, g := range o.groups {
			for _, r := range g.Resources {
				if r.Reconciler != nil {
					kindStr := fmt.Sprintf("%s.%s/%s", r.Kind.Plural(), r.Kind.Group(), r.Kind.Version())
					controller.AddReconciler(r.Reconciler, kindStr)
					client, err := clientRegistry.ClientFor(r.Kind)
					if err != nil {
						return err
					}
					ret, err := client.List(context.Background(), resource.NamespaceAll, resource.ListOptions{})
					fmt.Println(ret)
					fmt.Println(err)
					if c, ok := err.(resource.APIServerResponseError); ok {
						fmt.Println(c.StatusCode())
					}
					informer, err := operator.NewKubernetesBasedInformer(r.Kind, client, resource.NamespaceAll)
					if err != nil {
						return err
					}
					controller.AddInformer(informer, kindStr)
				}
			}
		}
		go controller.Run(ctx.StopCh)
		return nil
	})

	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}
