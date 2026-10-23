package plugin

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/grafana/authlib/authn"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	backendapp "github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/build/buildinfo"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/health"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
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
			ManifestData: &app.ManifestData{AppName: appName, Group: appName + ".grafana.app"},
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

		if err := Run(p, WithInsecureSkipAuthentication()); err != nil {
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

		if err := Run(p, WithKubeConfig(kubeConfig), WithInsecureSkipAuthentication()); err != nil {
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

		if err := Run(p, WithKubeConfig(rest.Config{Host: "https://example.com"}), WithInsecureSkipAuthentication()); err != nil {
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

	t.Run("does not start the app runner without a kube config", func(t *testing.T) {
		stubManage(t, nil)
		// BuildKubeConfig needs a router URL.
		t.Setenv("API_ACCESS_ROUTER_URL", "")
		p := newFakeProvider("my-app")
		runner := p.app.runner.(*fakeRunner)

		if err := Run(p, WithInsecureSkipAuthentication()); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}

		select {
		case <-runner.started:
			t.Error("expected the app runner not to be started")
		default:
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

		err := Run(p, WithPluginID("override-id"), WithAppFunc(appFunc), WithInsecureSkipAuthentication())
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

	t.Run("serves plugin v3 with the authenticator", func(t *testing.T) {
		call := stubManage(t, nil)
		authenticator := authn.NewAccessTokenAuthenticator(authn.NewUnsafeAccessTokenVerifier(authn.VerifierConfig{}))

		if err := Run(newFakeProvider("my-app"), WithAuthenticator(authenticator)); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if len(call.opts.ExtraPlugins) == 0 {
			t.Error("expected ExtraPlugins to be set by Run")
		}
	})

	t.Run("builds the authenticator from the environment", func(t *testing.T) {
		call := stubManage(t, nil)
		t.Setenv(EnvVarGrafanaJWKSURL, "https://auth.example.com/jwks")

		if err := Run(newFakeProvider("my-app")); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if len(call.opts.ExtraPlugins) == 0 {
			t.Error("expected ExtraPlugins to be set by Run")
		}
	})

	t.Run("authenticates with the manifest-derived plugin ID as an audience", func(t *testing.T) {
		call := stubManage(t, nil)
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvVarGrafanaJWKS, testJWKS(t, jose.JSONWebKey{Key: &key.PublicKey, KeyID: "test", Algorithm: string(jose.ES256), Use: "sig"}))

		if err := Run(newFakeProvider("my-app")); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		admission := serveAdmission(t, call.opts.ExtraPlugins)

		for _, tt := range []struct {
			audience string
			want     codes.Code
		}{
			// The API group is accepted for requests to that group.
			{audience: "my-app.grafana.app", want: codes.OK},
			// The plugin ID covers every API group the plugin serves.
			{audience: "my-app", want: codes.OK},
			{audience: "other-app", want: codes.Unauthenticated},
		} {
			ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-access-token", signTestToken(t, key, "test", tt.audience)))
			req := pluginv3.AdmissionReviewRequest_builder{
				Kind:        pluginv3.GroupVersionKind_builder{Group: new("my-app.grafana.app")}.Build(),
				ObjectBytes: []byte(`{"metadata":{"namespace":"stacks-1"}}`),
			}.Build()
			_, err := admission.AdmissionReview(ctx, req)
			if got := status.Code(err); got != tt.want {
				t.Errorf("audience %q: expected %v, got %v (%v)", tt.audience, tt.want, got, err)
			}
		}
	})

	t.Run("rejects an invalid signing keys URL even when skipping authentication", func(t *testing.T) {
		stubManage(t, nil)
		t.Setenv(EnvVarGrafanaJWKSURL, "ftp://auth.example.com/jwks")

		err := Run(newFakeProvider("my-app"), WithInsecureSkipAuthentication())
		if err == nil || !strings.Contains(err.Error(), "unsupported scheme") {
			t.Fatalf("expected an unsupported scheme error, got %v", err)
		}
	})

	t.Run("skips authentication when the host asks for local development", func(t *testing.T) {
		call := stubManage(t, nil)
		t.Setenv(EnvVarInsecureSkipAuthentication, "true")

		if err := Run(newFakeProvider("my-app")); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if len(call.opts.ExtraPlugins) == 0 {
			t.Error("expected ExtraPlugins to be set by Run")
		}
	})

	t.Run("starts without an authenticator, rejecting plugin v3 requests", func(t *testing.T) {
		for _, tt := range []struct {
			name      string
			skipValue string
		}{
			{name: "nothing configured"},
			// Only "true" skips authentication.
			{name: "skip not \"true\"", skipValue: "1"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				call := stubManage(t, nil)
				t.Setenv(EnvVarInsecureSkipAuthentication, tt.skipValue)

				if err := Run(newFakeProvider("my-app")); err != nil {
					t.Fatalf("Run returned error: %v", err)
				}
				admission := serveAdmission(t, call.opts.ExtraPlugins)

				for _, md := range []metadata.MD{nil, metadata.Pairs("x-access-token", "token")} {
					ctx := metadata.NewOutgoingContext(context.Background(), md)
					req := pluginv3.AdmissionReviewRequest_builder{
						Kind: pluginv3.GroupVersionKind_builder{Group: new("my-app.grafana.app")}.Build(),
					}.Build()
					_, err := admission.AdmissionReview(ctx, req)
					if got := status.Code(err); got != codes.FailedPrecondition {
						t.Errorf("metadata %v: expected %v, got %v (%v)", md, codes.FailedPrecondition, got, err)
					}
				}
			})
		}
	})

	t.Run("defaults the plugin ID to the plugin.json ID from the build", func(t *testing.T) {
		call := stubManage(t, nil)
		orig := buildinfo.GetBuildInfo
		buildinfo.GetBuildInfo = func() (buildinfo.Info, error) { return buildinfo.Info{PluginID: "my-plugin-json-id"}, nil }
		t.Cleanup(func() { buildinfo.GetBuildInfo = orig })

		if err := Run(newFakeProvider("my-app"), WithInsecureSkipAuthentication()); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if call.pluginID != "my-plugin-json-id" {
			t.Errorf("expected pluginID %q, got %q", "my-plugin-json-id", call.pluginID)
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

				err := Run(tt.provider, append(tt.opts, WithInsecureSkipAuthentication())...)
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

// serveAdmission serves the v3 plugins over an in-memory gRPC connection and
// returns an admission client for them.
func serveAdmission(t *testing.T, plugins goplugin.PluginSet) pluginv3.AdmissionServiceClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	for name, p := range plugins {
		if err := p.(goplugin.GRPCPlugin).GRPCServer(nil, server); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pluginv3.NewAdmissionServiceClient(conn)
}
