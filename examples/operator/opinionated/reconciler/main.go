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
	schema := resource.NewSimpleSchema("example.grafana.com", "v1", &resource.TypedSpecObject[OpinionatedModel]{}, resource.WithKind("OpinionatedCustomResource"))
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

	reconciler, err := operator.NewOpinionatedReconciler(client, "reconciler-finalizer")
	if err != nil {
		panic(fmt.Errorf("unable to create opinionated reconciler: %w", err))
	}

	// The opinionated reconciler is wrapping another reconciler which prints out the actions it's taking.
	reconciler.Wrap(
		&operator.SimpleReconciler{
			ReconcileFunc: func(ctx context.Context, request operator.ReconcileRequest) (operator.ReconcileResult, error) {
				switch request.Action {
				case operator.ReconcileActionCreated:
					log.Printf("Reconciling an %sAdd action%s:\n\t%v\n", green, nc, request.Object)
				case operator.ReconcileActionUpdated:
					log.Printf("Reconciling an %sUpdate action%s:\n\t%v\n", yellow, nc, request.Object)
				case operator.ReconcileActionDeleted:
					log.Printf("Reconciling a %sDelete action%s:\n\t%v\n", red, nc, request.Object)
				case operator.ReconcileActionResynced:
					log.Printf("Reconciling a %sSync action%s:\n\t%v\n", blue, nc, request.Object)
				}

				return operator.ReconcileResult{}, nil
			},
		},
	)

	// Start an informer controller with default config.
	informerController := operator.NewInformerController(operator.DefaultInformerControllerConfig())

	// Create an informer for the schema to watch all namespaces.
	informer, err := operator.NewKubernetesBasedInformer(kind, client, "")
	if err != nil {
		panic(fmt.Errorf("unable to create controller: %w", err))
	}

	err = informerController.AddInformer(informer, kind.Kind())
	if err != nil {
		panic(fmt.Errorf("unable to add informer to informer controller: %w", err))
	}
	err = informerController.AddReconciler(reconciler, kind.Kind())
	if err != nil {
		panic(fmt.Errorf("unable to add reconciler to informer controller: %w", err))
	}

	// Setting up the error handler for the informer controller.
	informerController.ErrorHandler = func(ctx context.Context, err error) {
		log.Printf("\u001B[0;31mERROR: %s\u001B[0m", err.Error())
	}

	// Create the operator
	op := operator.New()

	// Informers also implement Controller, so we can use them as a controller directly if there's no need for an intermediary.
	op.AddController(informerController)

	// Create the stop channel
	stopCh := make(chan struct{}, 1)

	// Set up a signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigChan
		close(stopCh)
	}()

	log.Printf("%sStarting Operator%s", green, nc)

	// Run the controller (will block until stopCh receives a message or is closed)
	err = op.Run(stopCh)
	if err != nil {
		panic(fmt.Errorf("error running operator: %w", err))
	}
}

type OpinionatedModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}
