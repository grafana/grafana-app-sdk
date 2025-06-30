package app

import (
	"context"
	"fmt"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/simple"

	generated "github.com/grafana/issue-tracker-project/pkg/generated"
	issuev1 "github.com/grafana/issue-tracker-project/pkg/generated/issue/v1"
	"github.com/grafana/issue-tracker-project/pkg/watchers"
)

func Provider(appCfg app.SpecificConfig) app.Provider {
	return simple.NewAppProvider(generated.LocalManifest(), appCfg, New)
}

func New(cfg app.Config) (app.App, error) {
	issueWatcher, err := watchers.NewIssueWatcher()
	if err != nil {
		return nil, fmt.Errorf("unable to create IssueWatcher: %w", err)
	}

	config := simple.AppConfig{
		Name:       "issue-tracker-project",
		KubeConfig: cfg.KubeConfig,
		InformerConfig: simple.AppInformerConfig{
			ErrorHandler: func(ctx context.Context, err error) {
				// FIXME: add your own error handling here
				logging.FromContext(ctx).With("error", err).Error("Informer processing error")
			},
		},
		ManagedKinds: []simple.AppManagedKind{
			{
				Kind:    issuev1.Kind(),
				Watcher: issueWatcher,
			},
		},
	}

	// Create the App
	a, err := simple.NewApp(config)
	if err != nil {
		return nil, err
	}

	// Validate the capabilities against the provided manifest to make sure there isn't a mismatch
	err = a.ValidateManifest(cfg.ManifestData)
	return a, err
}
