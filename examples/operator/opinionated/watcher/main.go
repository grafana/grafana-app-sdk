package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/grafana/grafana-app-sdk/k8s"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// These are for pretty printing in the logs
const (
	nc     = "\033[0m"
	red    = "\033[1;31m"
	green  = "\033[1;32m"
	yellow = "\033[1;33m"
	blue   = "\033[1;34m"
)

func main() {
	log.SetPrefix("\033[0;37m")
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
	schema := resource.NewSimpleSchema("example.grafana.com", "v1", &resource.TypedSpecObject[OpinionatedModel]{}, &resource.TypedList[*resource.TypedSpecObject[OpinionatedModel]]{}, resource.WithKind("OpinionatedCustomResource"))
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
	watcher, err := operator.NewOpinionatedWatcher(schema, client)
	if err != nil {
		panic(fmt.Errorf("unable to create opinionated watcher: %w", err))
	}

	// Set up the watcher handler functions
	// Any functions which aren't explicitly handled will still run with no specific action taken (i.e. the opinionated watcher will still do finalizer logic, etc.)
	// You can also have the opinionated watcher wrap a traditional ResourceWatcher (losing the ability to differentiate Add and Sync events) with watcher.Wrap()
	watcher.AddFunc = func(ctx context.Context, object resource.Object) error {
		log.Printf("%sADD%s: this is a resource which has been added either just now or prior to the start of the operator:\n\t%v\n", green, nc, object)
		return nil
	}
	watcher.SyncFunc = func(ctx context.Context, object resource.Object) error {
		log.Printf("%sSYNC%s: this is a resource which existed prior to the operator's start, but _has_ been handled by the operator before. "+
			"It may or may not have changed since then:\n\t%v\n", blue, nc, object)
		return nil
	}
	watcher.UpdateFunc = func(ctx context.Context, old resource.Object, new resource.Object) error {
		log.Printf("%sUPDATE%s: this is a resource which has been updated while the operator is running. We get both the old and new. "+
			"The opinionated watcher ignores any updates that don't change the contents of the Spec in the resource:\n\tOLD: %v\n\tNEW: %v\n", yellow, nc, old, new)
		return nil
	}
	watcher.DeleteFunc = func(ctx context.Context, object resource.Object) error {
		log.Printf("%sDELETE%s: this is a resource which has been deleted either while the operator was running, or prior to it (but was previously tracked by the operator): \n\t%v\n", red, nc, object)
		return nil
	}

	// Create an informer for the schema to watch all namespaces.
	informer, err := operator.NewKubernetesBasedInformer(kind, client, operator.KubernetesBasedInformerOptions{})
	if err != nil {
		panic(fmt.Errorf("unable to create controller: %w", err))
	}
	// Since we're not running multiple watchers or informers, we can directly add the watcher to the informer,
	// without needing an InformerController to manage it.
	informer.AddEventHandler(watcher)

	// Add a basic error handler to log errors. The function is called if a watcher function returns an error.
	// If no ErrorHandler is defined, the error is swallowed.
	informer.ErrorHandler = func(ctx context.Context, err error) {
		log.Printf("\u001B[0;31mERROR: %s\u001B[0m", err.Error())
	}

	// Create the operator
	op := operator.New()

	// Informers also implement Controller, so we can use them as a controller directly if there's no need for an intermediary.
	op.AddController(informer)

	// Set up a signal handler
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	log.Printf("%sStarting Operator%s", green, nc)

	// Run the controller (will block until stopCh receives a message or is closed)
	op.Run(ctx)
}

type OpinionatedModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}
