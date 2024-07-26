package simple

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"

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
	// Filesystem is an fs.FS that can be used in lieu of the OS filesystem.
	// if empty, it defaults to os.DirFS(".")
	Filesystem fs.FS
}

type OperatorWebhookConfig struct {
	// Port is the port to open the webhook server on
	Port int
	// TLSConfig is the TLS Cert and Key to use for the HTTPS endpoints exposed for webhooks
	TLSConfig k8s.TLSConfig
}

func (s *StandaloneOperator) Run(config SOperatorConfig, stopCh <-chan struct{}) error {
	// Get capabilities from manifest
	capabilities, err := s.getCapabilities(config)
	if err != nil {
		return fmt.Errorf("unable to get app manifest capabilities: %w", err)
	}

	// Create the app
	app, err := s.provider.NewApp(config.AppConfig)
	if err != nil {
		return err
	}

	// Build the operator
	op := operator.New()

	// Admission control
	if capabilities.Validator || capabilities.Mutator || capabilities.Converter {
		validator, ok := app.(resource.ValidatorApp)
		if capabilities.Validator && !ok {
			return fmt.Errorf("manifest has validator capability, but App does not implement ValidatorApp")
		}
		mutator, ok := app.(resource.MutatorApp)
		if capabilities.Mutator && !ok {
			return fmt.Errorf("manifest has mutator capability, but App does not implement MutatorApp")
		}
		converter, ok := app.(resource.ConverterApp)
		if capabilities.Converter && !ok {
			return fmt.Errorf("manifest has converter capability, but App does not implement ConverterApp")
		}
		webhooks, err := k8s.NewWebhookServer(k8s.WebhookServerConfig{
			Port:      config.WebhookConfig.Port,
			TLSConfig: config.WebhookConfig.TLSConfig,
		})
		if err != nil {
			return err
		}
		for _, kind := range app.ManagedKinds() {
			if capabilities.Validator {
				webhooks.AddValidatingAdmissionController(validator, kind)
			}
			if capabilities.Mutator {
				webhooks.AddMutatingAdmissionController(mutator, kind)
			}
			if capabilities.Converter {
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

func (s *StandaloneOperator) getCapabilities(cfg SOperatorConfig) (*resource.AppCapabilities, error) {
	// TODO: get from various places
	manifest := s.provider.Manifest()
	capabilities := resource.AppCapabilities{}
	switch manifest.Location.Type {
	case resource.AppManifestLocationEmbedded:
		if manifest.ManifestData == nil {
			return nil, fmt.Errorf("no ManifestData in AppManifest")
		}
		capabilities = manifest.ManifestData.Capabilities
	case resource.AppManifestLocationFilePath:
		// TODO: more correct version?
		dir := cfg.Filesystem
		if dir == nil {
			dir = os.DirFS(".")
		}
		if contents, err := fs.ReadFile(dir, manifest.Location.Path); err == nil {
			m := resource.AppManifest{}
			if err = json.Unmarshal(contents, &m); err == nil && m.ManifestData != nil {
				capabilities = m.ManifestData.Capabilities
			} else {
				return nil, fmt.Errorf("unable to unmarshal manifest data: %w", err)
			}
		} else {
			return nil, fmt.Errorf("error reading manifest file from disk (path: %s): %w", manifest.Location.Path, err)
		}
	case resource.AppManifestLocationAPIServerResource:
		// TODO: fetch from API server
		return nil, fmt.Errorf("apiserver location not supported yet")
	}
	return &capabilities, nil
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
