package simple

import (
	"context"
	"fmt"
	"strings"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// OperatorConfig is used to configure an Operator on creation
type OperatorConfig struct {
	Name         string
	KubeConfig   rest.Config
	Webhooks     WebhookConfig
	Metrics      MetricsConfig
	Tracing      TracingConfig
	ErrorHandler func(ctx context.Context, err error)
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
	Validators map[resource.Schema]resource.ValidatingAdmissionController
	// Mutators is an optional map of schema => MutatingAdmissionController to use for the schema on admission.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Mutators map[resource.Schema]resource.MutatingAdmissionController
	// Converters is an optional map of GroupKind => Converter to use for CRD version conversion requests.
	// This can be empty or nil and specific MutatingAdmissionControllers can be set later with Operator.MutateKind
	Converters map[metav1.GroupKind]k8s.Converter
}

// MetricsConfig contains configuration information for exposing prometheus metrics
type MetricsConfig struct {
	metrics.ExporterConfig
	Enabled   bool
	Namespace string
}

// TracingConfig contains configuration information for OpenTelemetry tracing
type TracingConfig struct {
	Enabled bool
	OpenTelemetryConfig
}

// NewOperator creates a new Operator
func NewOperator(cfg OperatorConfig) (*Operator, error) {
	cg := k8s.NewClientRegistry(cfg.KubeConfig, k8s.ClientConfig{})
	var ws *k8s.WebhookServer
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

	informerControllerConfig := operator.DefaultInformerControllerConfig()
	informerControllerConfig.MetricsConfig.Namespace = cfg.Metrics.Namespace
	// TODO: other factors?
	controller := operator.NewInformerController(informerControllerConfig)

	// Telemetry (metrics, traces)
	var me *metrics.Exporter
	if cfg.Metrics.Enabled {
		me = metrics.NewExporter(cfg.Metrics.ExporterConfig)
		me.RegisterCollectors(cg.PrometheusCollectors()...)
		me.RegisterCollectors(controller.PrometheusCollectors()...)
	}
	if cfg.Tracing.Enabled {
		SetTraceProvider(cfg.Tracing.OpenTelemetryConfig)
	}

	return &Operator{
		Name:            cfg.Name,
		ErrorHandler:    cfg.ErrorHandler,
		clientGen:       cg,
		controller:      controller,
		admission:       ws,
		metricsExporter: me,
	}, nil
}

// Operator is a simple operator implementation. Instead of manually registering controllers like with operator.Operator,
// use WatchKind to add a watcher for a specific kind (schema) and configuration (such as namespace, label filters),
// ReconcileKind to add a reconciler for a specific kind (schema) and configuration (such as namespace, label filers),
// and ValidateKind or MutateKind to add admission control for a kind (schema).
type Operator struct {
	Name string
	// ErrorHandler, if non-nil, is called when a recoverable error is encountered in underlying components.
	// This is typically used for logging and/or metrics.
	ErrorHandler    func(ctx context.Context, err error)
	clientGen       resource.ClientGenerator
	controller      *operator.InformerController
	admission       *k8s.WebhookServer
	metricsExporter *metrics.Exporter
}

type ListWatchOptions struct {
	Namespace    string
	LabelFilters []string
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
func (o *Operator) Run(stopCh <-chan struct{}) error {
	op := operator.New()
	op.AddController(o.controller)
	if o.admission != nil {
		op.AddController(o.admission)
	}
	if o.metricsExporter != nil {
		op.AddController(o.metricsExporter)
	}
	return op.Run(stopCh)
}

// RegisterMetricsCollectors registers Prometheus collectors with the exporter used by the operator,
// and will expose those metrics via the metrics endpoint configured in the operator config on Operator.Run
func (o *Operator) RegisterMetricsCollectors(collectors ...prometheus.Collector) error {
	return o.metricsExporter.RegisterCollectors(collectors...)
}

// WatchKind will watch the specified kind (schema) with opinionated logic, passing the relevant events on to the SyncWatcher.
// You can configure the query used for watching the kind using ListWatchOptions.
func (o *Operator) WatchKind(schema resource.Schema, watcher SyncWatcher, options ListWatchOptions) error {
	client, err := o.clientGen.ClientFor(schema)
	if err != nil {
		return err
	}
	inf, err := operator.NewKubernetesBasedInformerWithFilters(schema, client, options.Namespace, options.LabelFilters)
	if err != nil {
		return err
	}
	kindStr := o.label(schema, options)
	err = o.controller.AddInformer(inf, kindStr)
	if err != nil {
		return err
	}
	ow, err := operator.NewOpinionatedWatcherWithFinalizer(schema, client, func(sch resource.Schema) string {
		if o.Name != "" {
			return fmt.Sprintf("%s-%s-finalizer", o.Name, schema.Plural())
		}
		return fmt.Sprintf("%s-finalizer", schema.Plural())
	})
	if err != nil {
		return err
	}
	ow.Wrap(watcher, false)
	ow.SyncFunc = watcher.Sync
	return o.controller.AddWatcher(ow, kindStr)
}

func (o *Operator) ReconcileKind(schema resource.Schema, reconciler operator.Reconciler, options ListWatchOptions) error {
	client, err := o.clientGen.ClientFor(schema)
	if err != nil {
		return err
	}
	inf, err := operator.NewKubernetesBasedInformerWithFilters(schema, client, options.Namespace, options.LabelFilters)
	if err != nil {
		return err
	}
	kindStr := o.label(schema, options)
	err = o.controller.AddInformer(inf, kindStr)
	if err != nil {
		return err
	}
	finalizer := fmt.Sprintf("%s-finalizer", schema.Plural())
	if o.Name != "" {
		finalizer = fmt.Sprintf("%s-%s-finalizer", o.Name, schema.Plural())
	}
	or, err := operator.NewOpinionatedReconciler(client, finalizer)
	or.Reconciler = reconciler
	if err != nil {
		return err
	}
	return o.controller.AddReconciler(or, kindStr)
}

func (o *Operator) ValidateKind(schema resource.Schema, controller resource.ValidatingAdmissionController) error {
	if o.admission == nil {
		return fmt.Errorf("webhooks are not enabled")
	}
	o.admission.AddValidatingAdmissionController(controller, schema)
	return nil
}

func (o *Operator) MutateKind(schema resource.Schema, controller resource.MutatingAdmissionController) error {
	if o.admission == nil {
		return fmt.Errorf("webhooks are not enabled")
	}
	o.admission.AddMutatingAdmissionController(controller, schema)
	return nil
}

func (o *Operator) ConvertKind(schema resource.Schema, converter k8s.Converter) error {
	if o.admission == nil {
		return fmt.Errorf("webhooks are not enabled")
	}
	o.admission.AddConverter(converter, metav1.GroupKind{
		Group: schema.Group(),
		Kind:  schema.Kind(),
	})
	return nil
}

func (*Operator) label(schema resource.Schema, options ListWatchOptions) string {
	// TODO: hash
	return fmt.Sprintf("%s-%s-%s-%s-%s", schema.Group(), schema.Kind(), schema.Version(), options.Namespace, strings.Join(options.LabelFilters, ","))
}
