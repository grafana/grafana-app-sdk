package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	backendapp "github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/health"
	"github.com/grafana/grafana-app-sdk/resource"
)

// fakeApp is a minimal app.App which records the Config it was created with and
// exposes a Runnable whose lifecycle the test can observe.
type fakeApp struct {
	cfg    app.Config
	runner app.Runnable
}

func (*fakeApp) PrometheusCollectors() []prometheus.Collector          { return nil }
func (*fakeApp) HealthChecks() []health.Check                          { return nil }
func (*fakeApp) Validate(context.Context, *app.AdmissionRequest) error { return nil }
func (*fakeApp) Mutate(context.Context, *app.AdmissionRequest) (*app.MutatingResponse, error) {
	return nil, app.ErrNotImplemented
}
func (*fakeApp) Convert(context.Context, app.ConversionRequest) (*app.RawObject, error) {
	return nil, app.ErrNotImplemented
}
func (*fakeApp) CallCustomRoute(context.Context, app.CustomRouteResponseWriter, *app.CustomRouteRequest) error {
	return app.ErrCustomRouteNotFound
}
func (*fakeApp) ManagedKinds() []resource.Kind { return nil }
func (a *fakeApp) Runner() app.Runnable        { return a.runner }

var _ app.App = (*fakeApp)(nil)

// fakeRunner records that it was run, and blocks until its context is cancelled.
type fakeRunner struct {
	started chan struct{}
	done    chan struct{}
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{started: make(chan struct{}), done: make(chan struct{})}
}

func (r *fakeRunner) Run(ctx context.Context) error {
	close(r.started)
	defer close(r.done)
	<-ctx.Done()
	return ctx.Err()
}

// fakeProvider is a minimal app.Provider.
type fakeProvider struct {
	manifest       app.Manifest
	specificConfig app.SpecificConfig
	app            *fakeApp
	newAppErr      error
}

func (p *fakeProvider) Manifest() app.Manifest             { return p.manifest }
func (p *fakeProvider) SpecificConfig() app.SpecificConfig { return p.specificConfig }
func (p *fakeProvider) NewApp(cfg app.Config) (app.App, error) {
	if p.newAppErr != nil {
		return nil, p.newAppErr
	}
	p.app.cfg = cfg
	return p.app, nil
}

var _ app.Provider = (*fakeProvider)(nil)

func newFakeProvider(appName string) *fakeProvider {
	return &fakeProvider{
		manifest: app.Manifest{
			ManifestData: &app.ManifestData{AppName: appName},
			Location:     app.ManifestLocation{Type: app.ManifestLocationEmbedded},
		},
		app: &fakeApp{runner: newFakeRunner()},
	}
}

// stubManage replaces the package-level manage for the duration of the test,
// recording the arguments Run passed to it and returning err.
type manageCall struct {
	pluginID string
	appFunc  backendapp.InstanceFactoryFunc
	opts     backendapp.ManageOpts
}

func stubManage(t *testing.T, err error) *manageCall {
	t.Helper()
	call := &manageCall{}
	orig := manage
	manage = func(pluginID string, appFunc backendapp.InstanceFactoryFunc, opts backendapp.ManageOpts) error {
		call.pluginID = pluginID
		call.appFunc = appFunc
		call.opts = opts
		return err
	}
	t.Cleanup(func() { manage = orig })
	return call
}

func TestRun(t *testing.T) {
	t.Run("manages the plugin using the manifest app name and a stub instance", func(t *testing.T) {
		call := stubManage(t, nil)
		p := newFakeProvider("my-app")

		if err := Run(p); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}

		if call.pluginID != "my-app" {
			t.Errorf("expected pluginID %q, got %q", "my-app", call.pluginID)
		}
		if call.appFunc == nil {
			t.Fatal("expected an InstanceFactoryFunc to be passed to Manage")
		}
		// The default instance factory should produce the health-check-only stub.
		inst, err := call.appFunc(context.Background(), backend.AppInstanceSettings{})
		if err != nil {
			t.Fatalf("appFunc returned error: %v", err)
		}
		if _, ok := inst.(stubInstance); !ok {
			t.Errorf("expected stubInstance, got %T", inst)
		}
		// appadapter registers the plugin v3 services as extra plugins.
		if len(call.opts.ExtraPlugins) == 0 {
			t.Error("expected ExtraPlugins to be set by Run")
		}
	})

	t.Run("passes manifest data, kube config and specific config to NewApp", func(t *testing.T) {
		stubManage(t, nil)
		p := newFakeProvider("my-app")
		p.specificConfig = "specific"
		kubeConfig := rest.Config{Host: "https://example.com"}

		if err := Run(p, WithKubeConfig(kubeConfig)); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}

		got := p.app.cfg
		if got.KubeConfig.Host != kubeConfig.Host {
			t.Errorf("expected kube config host %q, got %q", kubeConfig.Host, got.KubeConfig.Host)
		}
		if got.ManifestData.AppName != "my-app" {
			t.Errorf("expected manifest data to be passed through, got %+v", got.ManifestData)
		}
		if got.SpecificConfig != "specific" {
			t.Errorf("expected specific config to be passed through, got %v", got.SpecificConfig)
		}
	})

	t.Run("runs the app runner and stops it when Manage returns", func(t *testing.T) {
		stubManage(t, nil)
		p := newFakeProvider("my-app")
		runner := p.app.runner.(*fakeRunner)

		if err := Run(p); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}

		select {
		case <-runner.started:
		default:
			t.Error("expected the app runner to have been started")
		}
		select {
		case <-runner.done:
		default:
			t.Error("expected the app runner to have been stopped before Run returned")
		}
	})

	t.Run("honours WithPluginID and WithAppFunc overrides", func(t *testing.T) {
		call := stubManage(t, nil)
		p := newFakeProvider("my-app")
		var appFuncCalled bool
		appFunc := func(context.Context, backend.AppInstanceSettings) (instancemgmt.Instance, error) {
			appFuncCalled = true
			return stubInstance{}, nil
		}

		err := Run(p, WithPluginID("override-id"), WithAppFunc(appFunc))
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}

		if call.pluginID != "override-id" {
			t.Errorf("expected pluginID %q, got %q", "override-id", call.pluginID)
		}
		if _, err := call.appFunc(context.Background(), backend.AppInstanceSettings{}); err != nil {
			t.Fatalf("appFunc returned error: %v", err)
		}
		if !appFuncCalled {
			t.Error("expected the provided InstanceFactoryFunc to be used")
		}
	})

	t.Run("errors", func(t *testing.T) {
		newAppErr := errors.New("new app failed")
		manageErr := errors.New("manage failed")

		for _, tt := range []struct {
			name      string
			provider  app.Provider
			opts      []RunOption
			manageErr error
			wantErr   error
			wantMsg   string
		}{
			{
				name:    "nil provider",
				wantMsg: "provider cannot be nil",
			},
			{
				name:     "manifest without embedded data",
				provider: &fakeProvider{manifest: app.Manifest{Location: app.ManifestLocation{Type: app.ManifestLocationFilePath, Path: "manifest.json"}}},
				wantMsg:  "embedded manifest required",
			},
			{
				name:     "NewApp fails",
				provider: &fakeProvider{manifest: newFakeProvider("my-app").manifest, newAppErr: newAppErr},
				wantErr:  newAppErr,
			},
			{
				name:     "ExtraPlugins already set",
				provider: newFakeProvider("my-app"),
				opts: []RunOption{WithManageOpts(backendapp.ManageOpts{
					ExtraPlugins: goplugin.PluginSet{"already-set": nil},
				})},
				wantMsg: "ExtraPlugins cannot be overridden",
			},
			{
				name:      "Manage fails",
				provider:  newFakeProvider("my-app"),
				manageErr: manageErr,
				wantErr:   manageErr,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				stubManage(t, tt.manageErr)

				err := Run(tt.provider, tt.opts...)
				if err == nil {
					t.Fatal("expected an error")
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				if tt.wantMsg != "" && err.Error() != tt.wantMsg {
					t.Errorf("expected error %q, got %q", tt.wantMsg, err.Error())
				}
			})
		}
	})
}
