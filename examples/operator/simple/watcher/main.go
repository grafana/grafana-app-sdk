package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"

	"k8s.io/client-go/tools/clientcmd"
)

// Schema, Kind, and Manifest are typically generated, but can be crafted by hand as seen here.
// For anything more complex than this simple example, it is advised that you use the CLI (grafana-app-sdk generate) to get these values
var (
	schema = resource.NewSimpleSchema("example.grafana.com", "v1", &resource.TypedSpecObject[BasicModel]{}, &resource.TypedList[*resource.TypedSpecObject[BasicModel]]{}, resource.WithKind("BasicCustomResource"))
	kind   = resource.Kind{
		Schema: schema,
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
	manifest = app.NewEmbeddedManifest(app.ManifestData{
		AppName: "example-app",
		Group:   kind.Group(),
		Versions: []app.ManifestVersion{{
			Name: kind.Version(),
			Kinds: []app.ManifestVersionKind{{
				Kind:  kind.Kind(),
				Scope: string(kind.Scope()),
			}},
		}},
	})
)

func main() {
	kubeCfgFile := flag.String("kubecfg", "", "kube config path")
	flag.Parse()
	if kubeCfgFile == nil || *kubeCfgFile == "" {
		_, _ = fmt.Println("--kubecfg must be set to the path of your kubernetes config file")
		os.Exit(1)
	}

	// Kubernetes configuration for all our interactions
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", *kubeCfgFile)
	if err != nil {
		panic(err)
	}
	kubeConfig.APIPath = "/apis" // Don't know why this isn't set correctly by default, but it isn't

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

	// Create an operator runner for our app. This dictates how an app will be run (operator.NewRunner runs as a standalone operator)
	runner, err := operator.NewRunner(operator.RunnerConfig{
		KubeConfig: *kubeConfig,
		MetricsConfig: operator.RunnerMetricsConfig{
			Enabled: true,
			MetricsServerConfig: operator.MetricsServerConfig{
				Port:                9090,
				HealthCheckInterval: 1 * time.Minute,
			},
		},
	})
	if err != nil {
		panic(fmt.Errorf("unable to create runner: %w", err))
	}

	// Set up a signal handler
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	log.Print("\u001B[1;32mStarting Operator\u001B[0m")

	// Run the app as an operator.
	// We provide runner.Run with an AppProvider which instantiates our app using the NewApp function we define at the bottom of this file
	err = runner.Run(ctx, simple.NewAppProvider(manifest, nil, NewApp))
	if err != nil {
		panic(fmt.Errorf("error running operator: %w", err))
	}
}

type BasicModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}

func NewApp(config app.Config) (app.App, error) {
	// Set up the watcher
	watcher := &simple.Watcher{
		AddFunc: func(_ context.Context, object resource.Object) error {
			log.Printf("Added object: %v\n", object)
			return nil
		},
		UpdateFunc: func(_ context.Context, oldObj, newObj resource.Object) error {
			log.Printf("Updated object:\n\told: %v\n\tnew: %v\n", oldObj, newObj)
			return nil
		},
		DeleteFunc: func(_ context.Context, object resource.Object) error {
			log.Printf("Deleted object: %v\n", object)
			return nil
		},
		SyncFunc: func(_ context.Context, object resource.Object) error {
			log.Printf("Synced object: %v\n", object)
			return nil
		},
	}

	// Create the app with the reconciler
	return simple.NewApp(simple.AppConfig{
		Name:       "simple-reconciler-app",
		KubeConfig: config.KubeConfig,
		ManagedKinds: []simple.AppManagedKind{{
			Kind:             kind,
			Watcher:          watcher,
			ReconcileOptions: simple.BasicReconcileOptions{
				// FIXME: Uncomment this line to turn off the opinionated logic
				// UsePlain: true, // UsePlain = true turns off the opinionated logic.
			},
		}},
	})
}
