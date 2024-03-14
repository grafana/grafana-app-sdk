package main

import (
	"fmt"

	corev1 "github.com/grafana/grafana-app-sdk/examples/apiserver/apis/core/v1"
	"github.com/grafana/grafana-app-sdk/simple"
	filestorage "github.com/grafana/grafana/pkg/apiserver/storage/file"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
)

func main() {
	r := simple.APIServerResource{
		Kind:                  corev1.Kind(),
		GetOpenAPIDefinitions: corev1.GetOpenAPIDefinitions,
	}
	g := simple.APIServerGroup{
		Name:     r.Kind.Group(),
		Resource: []simple.APIServerResource{r},
	}

	s := runtime.NewScheme()
	g.AddToScheme(s)
	codecs := serializer.NewCodecFactory(s)

	o := genericoptions.NewRecommendedOptions(
		"/registry/grafana",
		codecs.LegacyCodec(schema.GroupVersion{Group: r.Kind.Group(), Version: r.Kind.Version()}),
	)

	o.SecureServing.BindPort = 6443
	serverConfig := genericapiserver.NewRecommendedConfig(codecs)
	if err := o.SecureServing.ApplyTo(&serverConfig.SecureServing, &serverConfig.LoopbackClientConfig); err != nil {
		panic(err)
	}
	if err := o.Etcd.ApplyTo(&serverConfig.Config); err != nil {
		panic(err)
	}

	restStorage, err := filestorage.NewRESTOptionsGetter("./.data", o.Etcd.StorageConfig)
	if err != nil {
		panic(err)
	}
	serverConfig.RESTOptionsGetter = restStorage

	cfg := simple.Config{
		GenericConfig: serverConfig,
		ExtraConfig: simple.ExtraConfig{
			ResourceGroups: []simple.APIServerGroup{g},
		},
	}

	completed := cfg.Complete()

	server, err := completed.New()
	if err != nil {
		panic(err)
	}

	prepared := server.GenericAPIServer.PrepareRun()

	ch := make(chan struct{})
	fmt.Printf("Starting server\n")
	if err := prepared.Run(ch); err != nil {
		panic(err)
	}
}
