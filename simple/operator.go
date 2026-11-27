package simple

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// OperatorConfig is used to configure an Operator on creation
type OperatorConfig struct {
	Name         string
	KubeConfig   rest.Config
	Webhooks     WebhookConfig
	Metrics      MetricsConfig
	Port         int
	Tracing      TracingConfig
	ErrorHandler func(ctx context.Context, err error)
	// FinalizerGenerator consumes a schema and returns a finalizer name to use for opinionated logic.
	// the finalizer name MUST be 63 chars or fewer, and should be unique to the operator
	FinalizerGenerator          func(kind resource.Schema) string
	InformerCacheResyncInterval time.Duration
	// InformerSupplier builds the informers for the kinds this operator watches. If left unset, a plain
	// KubernetesBasedInformer is used, which calls handlers one at a time; a supplier set here decides its own
	// concurrency, as DefaultInformerSupplier does by wrapping in a ConcurrentInformer. It is given each kind's
	// needs as InformerOptions.WatchNeeds, so operator.NewSelectingInformerSupplier can be used to select a mechanism per
	// kind, as an App does.
	//
	// Experimental: this API may change in a minor release.
	InformerSupplier InformerSupplier
	// DiscoveryRefreshInterval is the interval at which the API discovery cache should be refreshed.
	// This is primarily used by the DynamicPatcher in the OpinionatedWatcher/OpinionatedReconciler
	// for sending finalizer add/remove patches to the latest version of the kind.
	// This defaults to 10 minutes.
	DiscoveryRefreshInterval time.Duration
}

// WebhookConfig is a configuration for exposed kubernetes webhooks for an Operator
type WebhookConfig struct {
	Enabled bool
	// Port is the port to open the webhook server on
	Port int
	// TLSConfig is the TLS Cert and Key to use for the HTTPS endpoints exposed for webhooks
	TLSConfig k8s.TLSConfig
	// DefaultValidator is an optional Default ValidatingAdmissionController to use if a specific one for the incoming
	// kind cannot be found
	DefaultValidator resource.ValidatingAdmissionController
	// DefaultMutator is an optional Default MutatingAdmissionController to use if a specific one for the incoming
	// kind cannot be found
	DefaultMutator resource.MutatingAdmissionController
	// Validators is an optional map of schema => ValidatingAdmissionController to use for the schema on admission.
	// This can be empty or nil and specific ValidatingAdmissionControllers can be set later with Operator.ValidateKind
	Validators map[*resource.Kind]resource.ValidatingAdmissionController
	// Mutators is an optional map of schema => MutatingAdmissionController to use for the schema on admission.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Mutators map[*resource.Kind]resource.MutatingAdmissionController
	// Converters is an optional map of GroupKind => Converter to use for CRD version conversion requests.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Converters map[metav1.GroupKind]k8s.Converter
}

// MetricsConfig contains configuration information for exposing prometheus metrics
type MetricsConfig struct {
	metrics.ExporterConfig
	operator.MetricsServerConfig
	Enabled   bool
	Namespace string
}

// TracingConfig contains configuration information for OpenTelemetry tracing
type TracingConfig struct {
	Enabled bool
	OpenTelemetryConfig
}

// NewOperator creates a new Operator
//
// Deprecated: please use simple.NewApp and operator.NewRunner to create a simple operator app.
func NewOperator(cfg OperatorConfig) (*Operator, error) {
	cg := k8s.NewClientRegistry(cfg.KubeConfig, k8s.ClientConfig{})
	var ws *k8s.WebhookServer

	if cfg.Port <= 0 {
		cfg.Port = 9090
		cfg.Metrics.Port = 9090
	}

	if cfg.Webhooks.Enabled {
		var err error
		ws, err = k8s.NewWebhookServer(k8s.WebhookServerConfig{
			Port:                        cfg.Webhooks.Port,
			TLSConfig:                   cfg.Webhooks.TLSConfig,
			DefaultValidatingController: cfg.Webhooks.DefaultValidator,
			DefaultMutatingController:   cfg.Webhooks.DefaultMutator,
			ValidatingControllers:       cfg.Webhooks.Validators,
			MutatingControllers:         cfg.Webhooks.Mutators,
			KindConverters:              cfg.Webhooks.Converters,
		})
		if err != nil {
			return nil, err
		}
	}
	discoveryRefresh := cfg.DiscoveryRefreshInterval
	if discoveryRefresh == 0 {
		discoveryRefresh = time.Minute * 10
	}

	patcher, err := k8s.NewDynamicPatcher(cg, discoveryRefresh)
	if err != nil {
		return nil, err
	}

	informerControllerConfig := operator.DefaultInformerControllerConfig()
	informerControllerConfig.MetricsConfig.Namespace = cfg.Metrics.Namespace
	// TODO: other factors?
	controller := operator.NewInformerController(informerControllerConfig)

	// this deprecated operator doesn't have any actual health checks, use the new operator runner
	// in order to get a true read on the readiness of the operator
	metricsServer := operator.NewMetricsServer(cfg.Metrics.MetricsServerConfig)

	// Telemetry (metrics, traces)
	var me *metrics.Exporter
	if cfg.Metrics.Enabled {
		me = metrics.NewExporter(cfg.Metrics.ExporterConfig)
		err := me.RegisterCollectors(cg.PrometheusCollectors()...)
		if err != nil {
			return nil, err
		}
		err = me.RegisterCollectors(controller.PrometheusCollectors()...)
		if err != nil {
			return nil, err
		}

		metricsServer.RegisterHealthChecks(controller.HealthChecks()...)

		metricsServer.RegisterMetricsHandler(me.HTTPHandler())
	}
	if cfg.Tracing.Enabled {
		err := SetTraceProvider(cfg.Tracing.OpenTelemetryConfig)
		if err != nil {
			return nil, err
		}
	}

	op := &Operator{
		Name:                cfg.Name,
		ErrorHandler:        cfg.ErrorHandler,
		FinalizerGenerator:  cfg.FinalizerGenerator,
		clientGen:           cg,
		controller:          controller,
		admission:           ws,
		metricsExporter:     me,
		cacheResyncInterval: cfg.InformerCacheResyncInterval,
		informerSupplier:    cfg.InformerSupplier,
		patcher:             patcher,
		metricsServer:       metricsServer,
	}

	op.controller.ErrorHandler = op.ErrorHandler
	return op, nil
}

// Operator is a simple operator implementation. Instead of manually registering controllers like with operator.Operator,
// use WatchKind to add a watcher for a specific kind (schema) and configuration (such as namespace, label filters),
// ReconcileKind to add a reconciler for a specific kind (schema) and configuration (such as namespace, label filers),
// and ValidateKind or MutateKind to add admission control for a kind (schema).
//
// Deprecated: use simple.App in conjunction with operator.Runner instead.
type Operator struct {
	Name string
	// ErrorHandler, if non-nil, is called when a recoverable error is encountered in underlying components.
	// This is typically used for logging and/or metrics.
	ErrorHandler func(ctx context.Context, err error)
	// FinalizerGenerator consumes a schema and returns a finalizer name to use for opinionated logic.
	// the finalizer name MUST be 63 chars or fewer, and should be unique to the operator
	FinalizerGenerator  func(schema resource.Schema) string
	clientGen           resource.ClientGenerator
	controller          *operator.InformerController
	admission           *k8s.WebhookServer
	metricsServer       *operator.MetricsServer
	metricsExporter     *metrics.Exporter
	cacheResyncInterval time.Duration
	informerSupplier    InformerSupplier
	patcher             *k8s.DynamicPatcher
	watches             sharedWatches
}

// SyncWatcher extends operator.ResourceWatcher with a Sync method which can be called by the operator.OpinionatedWatcher
type SyncWatcher interface {
	operator.ResourceWatcher
	// Sync is called for resources which _may_ have experienced updates
	Sync(context.Context, resource.Object) error
}

// ClientGenerator returns the ClientGenerator used by the Operator for getting clients for a particular schema
func (o *Operator) ClientGenerator() resource.ClientGenerator {
	return o.clientGen
}

// Run will start the operator and run until stopCh is closed or receives message.
// While running, the operator will:
//
// * Watch/Reconcile all configured resources
//
// * Expose all configured webhooks as an HTTPS server
//
// * Expose a prometheus metrics endpoint if configured
func (o *Operator) Run(ctx context.Context) error {
	// WatchKind and ReconcileKind build their informers after NewOperator gathered what the controller had, so
	// anything they brought with them is registered here. The exporter skips what it already holds.
	if o.metricsExporter != nil {
		if err := o.metricsExporter.RegisterCollectors(o.controller.PrometheusCollectors()...); err != nil {
			return err
		}
	}
	op := operator.New()
	op.AddController(o.controller)
	if o.admission != nil {
		op.AddController(&k8sRunnable{runner: o.admission})
	}

	// todo: is this the correct place to add these? op is only a local variable?
	// in the Run method
	o.metricsServer.RegisterHealthChecks(op.HealthChecks()...)
	op.AddController(o.metricsServer)
	return op.Run(ctx)
}

// RegisterMetricsCollectors registers Prometheus collectors with the exporter used by the operator,
// and will expose those metrics via the metrics endpoint configured in the operator config on Operator.Run
func (o *Operator) RegisterMetricsCollectors(collectors ...prometheus.Collector) error {
	return o.metricsExporter.RegisterCollectors(collectors...)
}

// kubernetesInformerSupplier builds the informer an Operator used before it took a supplier: a plain watch, whose
// handlers are called one at a time rather than through a ConcurrentInformer's workers.
func kubernetesInformerSupplier(
	kind resource.Kind, clients resource.ClientGenerator, options operator.InformerOptions,
) (operator.Informer, error) {
	client, err := clients.ClientFor(kind)
	if err != nil {
		return nil, err
	}
	if options.ResourceKind == "" {
		options.ResourceKind = kind.GroupVersionKind().String()
	}
	return operator.NewKubernetesBasedInformer(kind, client, options)
}

// KindWatchOption configures how one kind is watched, beyond the query ListWatchOptions describes.
//
// Experimental: this API may change in a minor release.
type KindWatchOption func(*kindWatchSettings)

type kindWatchSettings struct {
	watchNeeds operator.WatchOptions
}

// WithWatchNeeds states what a kind's handlers need of the changes they are watched by, which the operator passes
// to its InformerSupplier so that a selecting one can pick the mechanism which satisfies them, and whichever
// mechanism is built can configure itself from them. See operator.WatchOptions.
//
// Experimental: this API may change in a minor release.
func WithWatchNeeds(needs operator.WatchOptions) KindWatchOption {
	return func(s *kindWatchSettings) {
		s.watchNeeds = needs
	}
}

// addInformerForKind builds the informer for a kind and adds it to the controller under its key. forWatcher states
// that the kind's handler is an OpinionatedWatcher rather than an OpinionatedReconciler.
func (o *Operator) addInformerForKind(
	kind resource.Kind, options operator.ListWatchOptions, opts []KindWatchOption, forWatcher bool,
) error {
	var settings kindWatchSettings
	for _, opt := range opts {
		if opt != nil {
			opt(&settings)
		}
	}
	supplier := o.informerSupplier
	if supplier == nil {
		supplier = kubernetesInformerSupplier
	}
	return o.watches.register(o.label(kind, options), operator.InformerRequest{
		Kind:     kind,
		Clients:  o.clientGen,
		Supplier: supplier,
		Options: operator.InformerOptions{
			CacheResyncInterval: o.cacheResyncInterval,
			ErrorHandler:        o.ErrorHandler,
			ListWatchOptions:    options,
			WatchNeeds:          settings.watchNeeds,
		},
		// WatchKind and ReconcileKind both wrap their handler in its Opinionated variant.
		Opinionated:     true,
		DiscardsResyncs: forWatcher,
	}, o.controller.AddInformer)
}

// WatchKind will watch the specified kind (schema) with opinionated logic, passing the relevant events on to the SyncWatcher.
// You can configure the query used for watching the kind using ListWatchOptions, and what the watcher needs of the
// changes it is watched by with WithWatchNeeds.
func (o *Operator) WatchKind(
	kind resource.Kind, watcher SyncWatcher, options operator.ListWatchOptions, opts ...KindWatchOption,
) error {
	kindStr := o.label(kind, options)
	ow, err := operator.NewOpinionatedWatcher(kind, newWatcherPatcher(o.patcher, o.clientGen, kind), operator.OpinionatedWatcherConfig{
		Finalizer: func(sch resource.Schema) string {
			if o.FinalizerGenerator != nil {
				return o.FinalizerGenerator(sch)
			}
			if o.Name != "" {
				return fmt.Sprintf("%s-%s-finalizer", o.Name, kind.Plural())
			}
			return fmt.Sprintf("%s-finalizer", kind.Plural())
		},
		InProgressFinalizer: func(_ resource.Schema) string {
			if o.Name != "" {
				return fmt.Sprintf("%s-%s-wip", o.Name, kind.Plural())
			}
			return fmt.Sprintf("%s-wip", kind.Plural())
		},
	})
	if err != nil {
		return err
	}
	ow.Wrap(watcher, false)
	ow.SyncFunc = watcher.Sync
	if err := o.addInformerForKind(kind, options, opts, true); err != nil {
		return err
	}
	return o.controller.AddWatcher(ow, kindStr)
}

// ReconcileKind will watch the specified kind (schema) with opinionated logic, passing the events on to the provided Reconciler.
// You can configure the query used for watching the kind using ListWatchOptions, and what the reconciler needs of
// the changes it is watched by with WithWatchNeeds.
func (o *Operator) ReconcileKind(
	kind resource.Kind, reconciler operator.Reconciler, options operator.ListWatchOptions, opts ...KindWatchOption,
) error {
	kindStr := o.label(kind, options)
	finalizer := fmt.Sprintf("%s-finalizer", kind.Plural())
	if o.FinalizerGenerator != nil {
		finalizer = o.FinalizerGenerator(kind)
	} else if o.Name != "" {
		finalizer = fmt.Sprintf("%s-%s-finalizer", o.Name, kind.Plural())
	}
	or, err := operator.NewOpinionatedReconciler(&watchPatcher{o.patcher.ForKind(kind.GroupVersionKind().GroupKind())}, finalizer)
	if err != nil {
		return err
	}
	or.Reconciler = reconciler
	if err := o.addInformerForKind(kind, options, opts, false); err != nil {
		return err
	}
	return o.controller.AddReconciler(or, kindStr)
}

// ValidateKind provides a validation path for the provided kind (schema) in the validating webhook,
// using the provided ValidatingAdmissionController for the validation logic.
func (o *Operator) ValidateKind(kind resource.Kind, controller resource.ValidatingAdmissionController) error {
	if o.admission == nil {
		return errors.New("webhooks are not enabled")
	}
	o.admission.AddValidatingAdmissionController(controller, kind)
	return nil
}

// MutateKind provides a mutation path for the provided kind (schema) in the mutating webhook,
// using the provided MutatingAdmissionController for the mutation logic.
func (o *Operator) MutateKind(kind resource.Kind, controller resource.MutatingAdmissionController) error {
	if o.admission == nil {
		return errors.New("webhooks are not enabled")
	}
	o.admission.AddMutatingAdmissionController(controller, kind)
	return nil
}

// ConvertKind provides a conversion path for the provided GroupKind in the converting webhook,
// using the provided k8s.Converter for the conversion logic.
func (o *Operator) ConvertKind(gk metav1.GroupKind, converter k8s.Converter) error {
	if o.admission == nil {
		return errors.New("webhooks are not enabled")
	}
	o.admission.AddConverter(converter, gk)
	return nil
}

func (*Operator) label(schema resource.Schema, options operator.ListWatchOptions) string {
	// TODO: hash?
	return fmt.Sprintf("%s-%s-%s-%s-%s-%s", schema.Group(), schema.Kind(), schema.Version(), options.Namespace, strings.Join(options.LabelFilters, ","), strings.Join(options.FieldSelectors, ","))
}
