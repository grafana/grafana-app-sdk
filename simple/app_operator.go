package simple

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/metrics"
	"github.com/grafana/grafana-app-sdk/operator"
	"github.com/grafana/grafana-app-sdk/resource"
)

// StandaloneOperator runs an app as a standalone operator, capable of exposing admission (validation, mutation)
// and conversion as webhooks, and running a main control loop with reconcilers and watchers.
// It relies on the Kinds managed by the app.App already existing in the API server it talks to, either as CRD's
// or another type. It does not support certain advanced app.App functionality which is not natively supported by
// CRDs, such as arbitrary subresources (app.App.CallSubresource). It should be instantiated with NewStandaloneOperator.
type StandaloneOperator struct {
	provider app.Provider
}

// NewStandaloneOperator creates a new, properly-initialized instance of a StandaloneOperator,
// which will use the given app.Provider to instantiate a new underlying app.
func NewStandaloneOperator(provider app.Provider) (*StandaloneOperator, error) {
	if provider == nil {
		return nil, errors.New("provider cannot be nil")
	}
	return &StandaloneOperator{
		provider: provider,
	}, nil
}

type OperatorAppConfig struct {
	// WebhookConfig contains configuration information for exposing k8s webhooks.
	// This can be empty if your App does not implement ValidatorApp, MutatorApp, or ConversionApp
	WebhookConfig OperatorWebhookConfig
	// MetricsConfig contains the configuration for exposing prometheus metrics, if desired
	MetricsConfig MetricsConfig
	// TracingConfig contains the configuration for sending traces, if desired
	TracingConfig TracingConfig
	// AppConfig contains the configuration needed for creating and running the underlying App
	AppConfig app.Config
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

type capabilities struct {
	conversion bool
	mutation   bool
	validation bool
}

// Run runs the StandaloneOperator using the provided config, until stopCh is closed or receives a message, or an unrecoverable error occurs
func (s *StandaloneOperator) Run(config OperatorAppConfig, stopCh <-chan struct{}) error {
	if s.provider == nil {
		return errors.New("provider cannot be nil, instantiate StandaloneOperator with NewStandaloneOperator")
	}

	// Get capabilities from manifest
	manifestData, err := s.getManifestData(config)
	if err != nil {
		return fmt.Errorf("unable to get app manifest capabilities: %w", err)
	}
	config.AppConfig.ManifestData = *manifestData

	// Create the app
	a, err := s.provider.NewApp(config.AppConfig)
	if err != nil {
		return err
	}

	// Set up tracing, if enabled
	if config.TracingConfig.Enabled {
		err := SetTraceProvider(config.TracingConfig.OpenTelemetryConfig)
		if err != nil {
			return err
		}
	}

	// Build the operator
	op := operator.New()

	// Admission control
	anyWebhooks := false
	vkCapabilities := make(map[string]capabilities)
	for _, kind := range manifestData.Kinds {
		for _, version := range kind.Versions {
			if version.Admission == nil {
				continue
			}
			vkCapabilities[fmt.Sprintf("%s/%s", kind.Kind, version.Name)] = capabilities{
				conversion: kind.Conversion,
				mutation:   version.Admission.SupportsAnyMutation(),
				validation: version.Admission.SupportsAnyValidation(),
			}
			if kind.Conversion || version.Admission.SupportsAnyMutation() || version.Admission.SupportsAnyValidation() {
				anyWebhooks = true
			}
		}
	}
	if anyWebhooks {
		webhooks, err := k8s.NewWebhookServer(k8s.WebhookServerConfig{
			Port:      config.WebhookConfig.Port,
			TLSConfig: config.WebhookConfig.TLSConfig,
		})
		if err != nil {
			return err
		}
		for _, kind := range a.ManagedKinds() {
			c, ok := vkCapabilities[fmt.Sprintf("%s/%s", kind.Kind(), kind.Version())]
			if !ok {
				continue
			}
			if c.validation {
				webhooks.AddValidatingAdmissionController(a, kind)
			}
			if c.mutation {
				webhooks.AddMutatingAdmissionController(a, kind)
			}
			if c.conversion {
				webhooks.AddConverter(toWebhookConverter(a), metav1.GroupKind{
					Group: kind.Group(),
					Kind:  kind.Kind(),
				})
			}
		}
		op.AddController(webhooks)
	}

	// Main loop
	runner := a.Runner()
	if runner != nil {
		op.AddController(runner)
	}

	// Metrics
	if config.MetricsConfig.Enabled {
		exporter := metrics.NewExporter(config.MetricsConfig.ExporterConfig)
		err = exporter.RegisterCollectors(op.PrometheusCollectors()...)
		if err != nil {
			return err
		}
		op.AddController(exporter)
	}

	return op.Run(stopCh)
}

func (s *StandaloneOperator) getManifestData(cfg OperatorAppConfig) (*app.ManifestData, error) {
	manifest := s.provider.Manifest()
	data := app.ManifestData{}
	switch manifest.Location.Type {
	case app.ManifestLocationEmbedded:
		if manifest.ManifestData == nil {
			return nil, fmt.Errorf("no ManifestData in Manifest")
		}
		data = *manifest.ManifestData
	case app.ManifestLocationFilePath:
		// TODO: more correct version?
		dir := cfg.Filesystem
		if dir == nil {
			dir = os.DirFS(".")
		}
		contents, err := fs.ReadFile(dir, manifest.Location.Path)
		if err != nil {
			return nil, fmt.Errorf("error reading manifest file from disk (path: %s): %w", manifest.Location.Path, err)
		}
		m := app.Manifest{}
		if err = json.Unmarshal(contents, &m); err == nil && m.ManifestData != nil {
			data = *m.ManifestData
		} else {
			return nil, fmt.Errorf("unable to unmarshal manifest data: %w", err)
		}
	case app.ManifestLocationAPIServerResource:
		// TODO: fetch from API server
		return nil, fmt.Errorf("apiserver location not supported yet")
	}
	return &data, nil
}

func toWebhookConverter(a app.App) k8s.Converter {
	return &simpleK8sConverter{
		convertFunc: func(obj k8s.RawKind, targetAPIVersion string) ([]byte, error) {
			converted, err := a.Convert(context.Background(), app.ConversionRequest{
				SourceGVK: schema.FromAPIVersionAndKind(obj.APIVersion, obj.Kind),
				TargetGVK: schema.FromAPIVersionAndKind(targetAPIVersion, obj.Kind),
				Raw: app.RawObject{
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
