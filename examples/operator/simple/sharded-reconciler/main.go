package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	kitlog "github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"
)

var (
	schemaDef = resource.NewSimpleSchema("example.grafana.app", "v1", &resource.TypedSpecObject[BasicModel]{}, &resource.TypedList[*resource.TypedSpecObject[BasicModel]]{}, resource.WithKind("BasicCustomResource"))
	kindDef   = resource.Kind{
		Schema: schemaDef,
		Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: resource.NewJSONCodec()},
	}
	manifest = app.NewEmbeddedManifest(app.ManifestData{
		AppName: "hash-ring-sharding-example",
		Group:   kindDef.Group(),
		Versions: []app.ManifestVersion{{
			Name: kindDef.Version(),
			Kinds: []app.ManifestVersionKind{{
				Kind:  kindDef.Kind(),
				Scope: string(kindDef.Scope()),
			}},
		}},
	})
)

type BasicModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}

type ExampleConfig struct {
	MemcachedAddr     string
	ShardFilterConfig HashRingShardFilterConfig
}

func main() {
	kubeCfgFile := flag.String("kubecfg", "", "kube config path")
	memcachedAddr := flag.String("memcached-addr", "localhost:21211", "memcached address (host:port)")
	instanceID := flag.String("instance-id", "", "stable identifier for this replica")
	instanceAddr := flag.String("instance-addr", "", "instance address stored in the ring; defaults to advertise addr")
	numTokens := flag.Int("num-tokens", 128, "number of tokens to claim in the ring")
	heartbeatPeriod := flag.Duration("heartbeat-period", 5*time.Second, "memberlist heartbeat period")
	heartbeatTimeout := flag.Duration("heartbeat-timeout", time.Minute, "memberlist heartbeat timeout")
	rejoinInterval := flag.Duration("rejoin-interval", 30*time.Second, "memberlist rejoin interval")
	memberlistBindAddr := flag.String("memberlist-bind-addr", "", "memberlist bind address")
	memberlistBindPort := flag.Int("memberlist-bind-port", 7946, "memberlist bind port")
	memberlistAdvertiseAddr := flag.String("memberlist-advertise-addr", "", "memberlist advertise address")
	memberlistAdvertisePort := flag.Int("memberlist-advertise-port", 0, "memberlist advertise port; defaults to bind port")
	memberlistJoinMembers := flag.String("memberlist-join", "", "comma-separated memberlist peers to join")
	abortIfJoinFails := flag.Bool("memberlist-abort-if-join-fails", false, "stop startup if the initial join fails")
	memberlistClusterLabel := flag.String("memberlist-cluster-label", shardRingName, "memberlist cluster label")
	memberlistClusterLabelVerificationDisabled := flag.Bool("memberlist-cluster-label-verification-disabled", false, "disable memberlist cluster label verification")
	metricsPort := flag.Int("metrics-port", 9090, "metrics server port")
	flag.Parse()

	if kubeCfgFile == nil || *kubeCfgFile == "" {
		_, _ = fmt.Println("--kubecfg must be set to the path of your kubernetes config file")
		os.Exit(1)
	}

	kubeConfig, err := clientcmd.BuildConfigFromFlags("", *kubeCfgFile)
	if err != nil {
		panic(err)
	}
	kubeConfig.APIPath = "/apis"

	manager, err := k8s.NewManager(*kubeConfig)
	if err != nil {
		panic(fmt.Errorf("unable to create CRD manager: %w", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	err = manager.RegisterSchema(ctx, schemaDef, resource.RegisterSchemaOptions{
		NoErrorOnConflict:   true,
		WaitForAvailability: true,
	})
	if err != nil {
		panic(fmt.Errorf("unable to add custom resource definition: %w", err))
	}

	runner, err := operator.NewRunner(operator.RunnerConfig{
		KubeConfig: *kubeConfig,
		MetricsConfig: operator.RunnerMetricsConfig{
			Enabled: true,
			MetricsServerConfig: operator.MetricsServerConfig{
				Port:                *metricsPort,
				HealthCheckInterval: time.Minute,
			},
		},
	})
	if err != nil {
		panic(fmt.Errorf("unable to create runner: %w", err))
	}

	specificConfig := ExampleConfig{
		MemcachedAddr: *memcachedAddr,
		ShardFilterConfig: HashRingShardFilterConfig{
			InstanceID:              *instanceID,
			InstanceAddr:            *instanceAddr,
			NumTokens:               *numTokens,
			HeartbeatPeriod:         *heartbeatPeriod,
			HeartbeatTimeout:        *heartbeatTimeout,
			RejoinInterval:          *rejoinInterval,
			MemberlistBindAddr:      *memberlistBindAddr,
			MemberlistBindPort:      *memberlistBindPort,
			MemberlistAdvertiseAddr: *memberlistAdvertiseAddr,
			MemberlistAdvertisePort: *memberlistAdvertisePort,
			MemberlistJoinMembers:   splitCSV(*memberlistJoinMembers),
			AbortIfJoinFails:        *abortIfJoinFails,
			MemberlistClusterLabel:  *memberlistClusterLabel,
			MemberlistClusterLabelVerificationDisabled: *memberlistClusterLabelVerificationDisabled,
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("starting hash-ring sharding example instance=%q", specificConfig.ShardFilterConfig.instanceID())

	err = runner.Run(ctx, simple.NewAppProvider(manifest, specificConfig, NewApp))
	if err != nil {
		panic(fmt.Errorf("error running operator: %w", err))
	}
}

func NewApp(config app.Config) (app.App, error) {
	specific, ok := config.SpecificConfig.(ExampleConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected specific config type %T", config.SpecificConfig)
	}

	filter, err := NewHashRingShardFilter(
		specific.ShardFilterConfig,
		kitlog.NewLogfmtLogger(kitlog.NewSyncWriter(os.Stdout)),
		prometheus.DefaultRegisterer,
	)
	if err != nil {
		return nil, err
	}

	reconciler := &simple.Reconciler{
		ReconcileFunc: func(_ context.Context, request operator.ReconcileRequest) (operator.ReconcileResult, error) {
			log.Printf(
				"instance=%s action=%s object=%s/%s",
				specific.ShardFilterConfig.instanceID(),
				operator.ResourceActionFromReconcileAction(request.Action),
				request.Object.GetNamespace(),
				request.Object.GetName(),
			)
			return operator.ReconcileResult{}, nil
		},
	}

	exampleApp, err := simple.NewApp(simple.AppConfig{
		Name:       "hash-ring-sharding-example",
		KubeConfig: config.KubeConfig,
		InformerConfig: simple.AppInformerConfig{
			InformerSupplier: func(kind resource.Kind, clients resource.ClientGenerator, _ operator.InformerOptions) (operator.Informer, error) {
				client, err := clients.ClientFor(kind)
				if err != nil {
					return nil, err
				}
				return operator.NewMemcachedInformer(kind, client, operator.MemcachedInformerOptions{
					ServerAddrs: []string{specific.MemcachedAddr},
				})
			},
		},
		ManagedKinds: []simple.AppManagedKind{{
			Kind:       kindDef,
			Reconciler: reconciler,
			ReconcileOptions: simple.BasicReconcileOptions{
				ShardFilter: filter,
				UsePlain:    true,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	exampleApp.AddRunnable(filter)

	return exampleApp, nil
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}

	raw := strings.Split(v, ",")
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		values = append(values, item)
	}

	return values
}
