package simple

import (
	"context"
	"fmt"
	"strings"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/client-go/rest"
)

type WebhookConfig struct {
	Enabled   bool
	Port      int
	TLSConfig k8s.TLSConfig
}

// NewOperator creates a new Operator
func NewOperator(kubeConfig rest.Config, webhookConfig WebhookConfig) (*Operator, error) {
	cg := k8s.NewClientRegistry(kubeConfig, k8s.ClientConfig{})
	var ws *k8s.WebhookServer
	if webhookConfig.Enabled {
		var err error
		ws, err = k8s.NewWebhookServer(k8s.WebhookServerConfig{
			Port:      0,
			TLSConfig: k8s.TLSConfig{},
		})
		if err != nil {
			return nil, err
		}
	}
	return &Operator{
		clientGen:  cg,
		controller: operator.NewInformerController(),
		admission:  ws,
	}, nil
}

// Operator is a simple operator implementation. Instead of manually registering controllers like with operator.Operator,
// use WatchKind to add a watcher for a specific kind (schema) and configuration (such as namespace, label filters),
// ReconcileKind to add a reconciler for a specific kind (schema) and configuration (such as namespace, label filers),
// and ValidateKind or MutateKind to add admission control for a kind (schema).
type Operator struct {
	Name       string
	clientGen  resource.ClientGenerator
	controller *operator.InformerController
	admission  *k8s.WebhookServer
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

func (o *Operator) ClientGenerator() resource.ClientGenerator {
	return o.clientGen
}

func (o *Operator) Run(stopCh <-chan struct{}) error {
	op := operator.New()
	op.AddController(o.controller)
	if o.admission != nil {
		op.AddController(o.admission)
	}
	return op.Run(stopCh)
}

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

func (*Operator) label(schema resource.Schema, options ListWatchOptions) string {
	// TODO: hash
	return fmt.Sprintf("%s-%s-%s-%s-%s", schema.Group(), schema.Kind(), schema.Version(), options.Namespace, strings.Join(options.LabelFilters, ","))
}
