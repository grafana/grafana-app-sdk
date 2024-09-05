package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"

	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeCfgFile := flag.String("kubecfg", "", "kube config path")
	flag.Parse()
	if kubeCfgFile == nil || *kubeCfgFile == "" {
		fmt.Println("--kubecfg must be set to the path of your kubernetes config file")
		os.Exit(1)
	}

	// Kubernetes' configuration for all our interactions
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", *kubeCfgFile)
	if err != nil {
		panic(err)
	}
	kubeConfig.APIPath = "/apis" // Don't know why this isn't set correctly by default, but it isn't

	// Create a schema to use
	schema := resource.NewSimpleSchema("example.grafana.com", "v1", &resource.TypedSpecObject[BasicModel]{}, &resource.TypedList[*resource.TypedSpecObject[BasicModel]]{}, resource.WithKind("BasicCustomResource"))
	kind := resource.Kind{
		Schema: schema,
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}

	// Register the schema (if it doesn't already exist)
	manager, err := k8s.NewManager(*kubeConfig)
	if err != nil {
		panic(fmt.Errorf("unable to create CRD manager: %w", err))
	}
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Minute)
	defer cancelFunc()
	err = manager.RegisterSchema(ctx, schema, resource.RegisterSchemaOptions{
		NoErrorOnConflict:   true, // Don't error if the schema is already registered
		WaitForAvailability: true, // Wait for the schema to be considered available by k8s, or until the context is canceled
	})
	if err != nil {
		panic(fmt.Errorf("unable to add custom resource definition: %w", err))
	}

	simpleOperator, err := simple.NewOperator(simple.OperatorConfig{
		Name:       "simple-watcher-operator",
		KubeConfig: *kubeConfig,
		Metrics: simple.MetricsConfig{
			Enabled: true,
		},
		ErrorHandler: func(ctx context.Context, err error) {
			log.Printf("\u001B[0;31mERROR: %s\u001B[0m", err.Error())
		},
	})
	if err != nil {
		panic(fmt.Errorf("unable to initialise operator: %w", err))
	}

	// Set up the watcher
	watcher := &simple.Watcher{
		AddFunc: func(ctx context.Context, object resource.Object) error {
			log.Printf("Added object: %v\n", object)
			return nil
		},
		UpdateFunc: func(ctx context.Context, old, new resource.Object) error {
			log.Printf("Updated object:\n\told: %v\n\tnew: %v\n", old, new)
			return nil
		},
		DeleteFunc: func(ctx context.Context, object resource.Object) error {
			log.Printf("Deleted object: %v\n", object)
			return nil
		},
		SyncFunc: func(ctx context.Context, object resource.Object) error {
			log.Printf("Synced object: %v\n", object)
			return nil
		},
	}

	err = simpleOperator.WatchKind(kind, watcher, operator.ListWatchOptions{
		Namespace: "default",
	})
	if err != nil {
		panic(fmt.Errorf("unable to watch kind: %w", err))
	}

	// Create the stop channel
	stopCh := make(chan struct{}, 1)

	// Set up a signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigChan
		close(stopCh)
	}()

	log.Print("\u001B[1;32mStarting Operator\u001B[0m")

	// Run the controller (will block until stopCh receives a message or is closed)
	err = simpleOperator.Run(stopCh)
	if err != nil {
		panic(fmt.Errorf("error running operator: %w", err))
	}
}

type BasicModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}
