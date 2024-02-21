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
	"k8s.io/client-go/tools/clientcmd"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

func main() {
	kubeCfgFile := flag.String("kubecfg", "", "kube config path")
	flag.Parse()
	if kubeCfgFile == nil || *kubeCfgFile == "" {
		fmt.Println("--kubecfg must be set to the path of your kubernetes config file")
		os.Exit(1)
	}

	// Kubernetes configuration for all our interactions
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", *kubeCfgFile)
	if err != nil {
		panic(err)
	}
	kubeConfig.APIPath = "/apis" // Don't know why this isn't set correctly by default, but it isn't

	// Create a schema to use
	schema := resource.NewSimpleSchema("example.grafana.com", "v1", &resource.TypedSpecObject[BasicModel]{}, resource.WithKind("BasicCustomResource"))
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

	// Get a client for our schema
	clientGenerator := k8s.NewClientRegistry(*kubeConfig, k8s.ClientConfig{})
	client, err := clientGenerator.ClientFor(kind)
	if err != nil {
		panic(fmt.Errorf("error creating client for schema: %w", err))
	}

	// Set up the watcher
	watcher := &BasicWatcher{}

	// Create the Informer
	informer, err := operator.NewKubernetesBasedInformer(kind, client, "default")
	if err != nil {
		panic(fmt.Errorf("unable to create informer: %w", err))
	}

	// Since we're not using multiple watchers, we can add our watcher directly to the informer,
	// and not use an InformerController, instead adding our Informer directly to the Operator
	informer.AddEventHandler(watcher)

	// Add a basic error handler to log errors. The function is called if a watcher function returns an error.
	// If no ErrorHandler is defined, the error is swallowed.
	informer.ErrorHandler = func(ctx context.Context, err error) {
		log.Printf("\u001B[0;31mERROR: %s\u001B[0m", err.Error())
	}

	// Create the operator
	op := operator.New()

	// Add the controller
	op.AddController(informer)

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
	op.Run(stopCh)
}

type BasicModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}

// BasicWatcher is a basic user-defined watcher to use in our operator. It has functions for handling adds, updates, and deletes for our object
type BasicWatcher struct {
}

func (b *BasicWatcher) Add(ctx context.Context, object resource.Object) error {
	log.Printf("Added object: %v\n", object)
	return nil
}

func (b *BasicWatcher) Update(ctx context.Context, old, new resource.Object) error {
	log.Printf("Updated object:\n\told: %v\n\tnew: %v\n", old, new)
	return nil
}

func (b *BasicWatcher) Delete(ctx context.Context, object resource.Object) error {
	log.Printf("Deleted object: %v\n", object)
	return nil
}
