package simple

import (
	"context"
	"fmt"
	"strings"

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
	Tracing      TracingConfig
	ErrorHandler func(ctx context.Context, err error)
	// FinalizerGenerator consumes a schema and returns a finalizer name to use for opinionated logic.
	// the finalizer name MUST be 63 chars or fewer, and should be unique to the operator
	FinalizerGenerator func(kind resource.Schema) string
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
		err := me.RegisterCollectors(cg.PrometheusCollectors()...)
		if err != nil {
			return nil, err
		}
		err = me.RegisterCollectors(controller.PrometheusCollectors()...)
		if err != nil {
			return nil, err
		}
	}
	if cfg.Tracing.Enabled {
		err := SetTraceProvider(cfg.Tracing.OpenTelemetryConfig)
		if err != nil {
			return nil, err
		}
	}

	op := &Operator{
		Name:               cfg.Name,
		ErrorHandler:       cfg.ErrorHandler,
		FinalizerGenerator: cfg.FinalizerGenerator,
		clientGen:          cg,
		controller:         controller,
		admission:          ws,
		metricsExporter:    me,
	}
	op.controller.ErrorHandler = op.ErrorHandler
	return op, nil
}

// Operator is a simple operator implementation. Instead of manually registering controllers like with operator.Operator,
// use WatchKind to add a watcher for a specific kind (schema) and configuration (such as namespace, label filters),
// ReconcileKind to add a reconciler for a specific kind (schema) and configuration (such as namespace, label filers),
// and ValidateKind or MutateKind to add admission control for a kind (schema).
type Operator struct {
	Name string
	// ErrorHandler, if non-nil, is called when a recoverable error is encountered in underlying components.
	// This is typically used for logging and/or metrics.
	ErrorHandler func(ctx context.Context, err error)
	// FinalizerGenerator consumes a schema and returns a finalizer name to use for opinionated logic.
	// the finalizer name MUST be 63 chars or fewer, and should be unique to the operator
	FinalizerGenerator func(schema resource.Schema) string
	clientGen          resource.ClientGenerator
	controller         *operator.InformerController
	admission          *k8s.WebhookServer
	metricsExporter    *metrics.Exporter
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
func (o *Operator) WatchKind(kind resource.Kind, watcher SyncWatcher, options operator.ListWatchOptions) error {
	client, err := o.clientGen.ClientFor(kind)
	if err != nil {
		return err
	}
	inf, err := operator.NewKubernetesBasedInformerWithFilters(kind, client, operator.ListWatchOptions{Namespace: options.Namespace, LabelFilters: options.LabelFilters, FieldSelectors: options.FieldSelectors})
	if err != nil {
		return err
	}
	inf.ErrorHandler = o.ErrorHandler
	kindStr := o.label(kind, options)
	err = o.controller.AddInformer(inf, kindStr)
	if err != nil {
		return err
	}
	ow, err := operator.NewOpinionatedWatcherWithFinalizer(kind, client, func(sch resource.Schema) string {
		if o.FinalizerGenerator != nil {
			return o.FinalizerGenerator(sch)
		}
		if o.Name != "" {
			return fmt.Sprintf("%s-%s-finalizer", o.Name, kind.Plural())
		}
		return fmt.Sprintf("%s-finalizer", kind.Plural())
	})
	if err != nil {
		return err
	}
	ow.Wrap(watcher, false)
	ow.SyncFunc = watcher.Sync
	return o.controller.AddWatcher(ow, kindStr)
}

// ReconcileKind will watch the specified kind (schema) with opinionated logic, passing the events on to the provided Reconciler.
// You can configure the query used for watching the kind using ListWatchOptions.
func (o *Operator) ReconcileKind(kind resource.Kind, reconciler operator.Reconciler, options operator.ListWatchOptions) error {
	client, err := o.clientGen.ClientFor(kind)
	if err != nil {
		return err
	}
	inf, err := operator.NewKubernetesBasedInformerWithFilters(kind, client, operator.ListWatchOptions{Namespace: options.Namespace, LabelFilters: options.LabelFilters, FieldSelectors: options.FieldSelectors})
	if err != nil {
		return err
	}
	inf.ErrorHandler = o.ErrorHandler
	kindStr := o.label(kind, options)
	err = o.controller.AddInformer(inf, kindStr)
	if err != nil {
		return err
	}
	finalizer := fmt.Sprintf("%s-finalizer", kind.Plural())
	if o.FinalizerGenerator != nil {
		finalizer = o.FinalizerGenerator(kind)
	} else if o.Name != "" {
		finalizer = fmt.Sprintf("%s-%s-finalizer", o.Name, kind.Plural())
	}
	or, err := operator.NewOpinionatedReconciler(client, finalizer)
	if err != nil {
		return err
	}
	or.Reconciler = reconciler
	return o.controller.AddReconciler(or, kindStr)
}

// ValidateKind provides a validation path for the provided kind (schema) in the validating webhook,
// using the provided ValidatingAdmissionController for the validation logic.
func (o *Operator) ValidateKind(kind resource.Kind, controller resource.ValidatingAdmissionController) error {
	if o.admission == nil {
		return fmt.Errorf("webhooks are not enabled")
	}
	o.admission.AddValidatingAdmissionController(controller, kind)
	return nil
}

// MutateKind provides a mutation path for the provided kind (schema) in the mutating webhook,
// using the provided MutatingAdmissionController for the mutation logic.
func (o *Operator) MutateKind(kind resource.Kind, controller resource.MutatingAdmissionController) error {
	if o.admission == nil {
		return fmt.Errorf("webhooks are not enabled")
	}
	o.admission.AddMutatingAdmissionController(controller, kind)
	return nil
}

// ConvertKind provides a conversion path for the provided GroupKind in the converting webhook,
// using the provided k8s.Converter for the conversion logic.
func (o *Operator) ConvertKind(gk metav1.GroupKind, converter k8s.Converter) error {
	if o.admission == nil {
		return fmt.Errorf("webhooks are not enabled")
	}
	o.admission.AddConverter(converter, gk)
	return nil
}

func (*Operator) label(schema resource.Schema, options operator.ListWatchOptions) string {
	// TODO: hash?
	return fmt.Sprintf("%s-%s-%s-%s-%s-%s", schema.Group(), schema.Kind(), schema.Version(), options.Namespace, strings.Join(options.LabelFilters, ","), strings.Join(options.FieldSelectors, ","))
}
