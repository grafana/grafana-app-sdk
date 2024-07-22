package simple

import (
	"context"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type StandaloneOperator struct {
	provider resource.AppProvider
}

func NewStandaloneOperator(provider resource.AppProvider) *StandaloneOperator {
	return &StandaloneOperator{
		provider: provider,
	}
}

type SOperatorConfig struct {
	// WebhookConfig contains configuration information for exposing k8s webhooks.
	// This can be empty if your App does not implement ValidatorApp, MutatorApp, or ConversionApp
	WebhookConfig OperatorWebhookConfig
	// MetricsConfig contains the configuration for exposing prometheus metrics, if desired
	MetricsConfig MetricsConfig
	// AppConfig contains the configuration needed for creating and running the underlying App
	AppConfig resource.AppConfig
}

type OperatorWebhookConfig struct {
	// Port is the port to open the webhook server on
	Port int
	// TLSConfig is the TLS Cert and Key to use for the HTTPS endpoints exposed for webhooks
	TLSConfig k8s.TLSConfig
}

func (s *StandaloneOperator) Run(config SOperatorConfig, stopCh <-chan struct{}) error {
	// Create the app
	app, err := s.provider.NewApp(config.AppConfig)
	if err != nil {
		return err
	}

	// Build the operator
	op := operator.New()

	// Admission control
	validator, vOK := app.(resource.ValidatorApp)
	mutator, mOK := app.(resource.MutatorApp)
	converter, cOK := app.(resource.ConverterApp)
	if vOK || mOK || cOK {
		webhooks, err := k8s.NewWebhookServer(k8s.WebhookServerConfig{
			Port:      config.WebhookConfig.Port,
			TLSConfig: config.WebhookConfig.TLSConfig,
		})
		if err != nil {
			return err
		}
		for _, kind := range app.ManagedKinds() {
			if vOK {
				webhooks.AddValidatingAdmissionController(validator, kind)
			}
			if mOK {
				webhooks.AddMutatingAdmissionController(mutator, kind)
			}
			if cOK {
				webhooks.AddConverter(toWebhookConverter(converter), metav1.GroupKind{
					Group: kind.Group(),
					Kind:  kind.Kind(),
				})
			}
		}
		op.AddController(webhooks)
	}

	// Main loop
	runner := app.Runner()
	if runner != nil {
		op.AddController(runner)
	}

	// Metrics
	if config.MetricsConfig.Enabled {
		exporter := metrics.NewExporter(config.MetricsConfig.ExporterConfig)
		exporter.RegisterCollectors(op.PrometheusCollectors()...)
		op.AddController(exporter)
	}

	return op.Run(stopCh)
}

func toWebhookConverter(app resource.ConverterApp) k8s.Converter {
	return &simpleK8sConverter{
		convertFunc: func(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
			converted, err := app.Convert(context.Background(), resource.ConversionRequest{
				SourceGVK: schema.FromAPIVersionAndKind(obj.APIVersion, obj.Kind),
				TargetGVK: schema.FromAPIVersionAndKind(targetAPIVersion, obj.Kind),
				Raw: resource.RawObject{
					Raw:      obj.Raw,
					Encoding: resource.KindEncodingJSON,
				},
			})
			if err != nil {
				return nil, err
			}
			return converted.Raw, nil
		},
	}
}

type simpleK8sConverter struct {
	convertFunc func(obj k8s.RawKind, targetAPIVersion string) ([]byte, error)
}

func (s *simpleK8sConverter) Convert(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
	return s.convertFunc(obj, targetAPIVersion)
}
