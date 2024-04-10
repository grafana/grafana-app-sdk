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

	simpleOperator, err := simple.NewOperator(simple.OperatorConfig{
		Name:       "simple-reconciler-operator",
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

	// Set up the reconciler
	reconciler := &simple.Reconciler{
		ReconcileFunc: func(ctx context.Context, request operator.ReconcileRequest) (operator.ReconcileResult, error) {
			log.Printf(
				"Reconciling object:\n\taction: %s\n\tobject: %v\n",
				operator.ResourceActionFromReconcileAction(request.Action),
				request.Object,
			)

			return operator.ReconcileResult{}, nil
		},
	}

	err = simpleOperator.ReconcileKind(kind, reconciler, simple.ListWatchOptions{
		Namespace: "default",
	})
	if err != nil {
		panic(fmt.Errorf("unable to reconcile kind: %w", err))
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
