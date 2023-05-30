package kubeconfig_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/grafana/grafana-app-sdk/plugin/kubeconfig"
)

func TestLoadingMiddleware(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]string
		configErr error
		want      kubeconfig.NamespacedConfig
	}{
		{
			name:      "when keys are missing from secureJsonData",
			data:      map[string]string{},
			configErr: nil,
			want:      kubeconfig.NamespacedConfig{},
		},
		{
			name: "when config is missing from secureJsonData",
			data: map[string]string{
				kubeconfig.KeyNamespace: "namespace",
			},
			configErr: nil,
			want:      kubeconfig.NamespacedConfig{},
		},
		{
			name: "when namespace key is missing from secureJsonData",
			data: map[string]string{
				kubeconfig.KeyConfig: "config",
			},
			configErr: nil,
			want:      kubeconfig.NamespacedConfig{},
		},
		{
			name: "when config loader fails",
			data: map[string]string{
				kubeconfig.KeyConfig:    "config",
				kubeconfig.KeyNamespace: "namespace",
			},
			configErr: assert.AnError,
			want:      kubeconfig.NamespacedConfig{},
		},
		{
			name: "when config is added successfully",
			data: map[string]string{
				kubeconfig.KeyConfig:    "config",
				kubeconfig.KeyNamespace: "namespace",
			},
			configErr: nil,
			want: kubeconfig.NamespacedConfig{
				Namespace: "namespace",
				RestConfig: rest.Config{
					Host: "https://some.url:6443",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := kubeconfig.LoadingMiddlewareWithLoader(&FakeConfigLoader{
				loadFn: func(s backend.AppInstanceSettings, c *kubeconfig.NamespacedConfig) error {
					if tt.configErr != nil {
						return tt.configErr
					}

					cf, ns, err := kubeconfig.LoadRawConfig(s.DecryptedSecureJSONData)
					if err != nil {
						return err
					}

					c.Namespace = ns
					if cf != "" {
						c.RestConfig = rest.Config{
							Host: "https://some.url:6443",
						}
					}

					return nil
				},
			})

			hl := mw(func(ctx context.Context, crr *backend.CallResourceRequest, crrs backend.CallResourceResponseSender) {
				cfg, err := kubeconfig.FromContext(ctx)

				assert.Equal(t, tt.want, cfg)
				assert.NoError(t, err)

				crrs.Send(&backend.CallResourceResponse{
					Status: http.StatusOK,
				})
			})

			hl(
				context.Background(),
				&backend.CallResourceRequest{
					PluginContext: backend.PluginContext{
						AppInstanceSettings: &backend.AppInstanceSettings{
							DecryptedSecureJSONData: tt.data,
						},
					},
				},
				&FakeResponseSender{
					sendFn: func(crr *backend.CallResourceResponse) error {
						assert.Equal(t, http.StatusOK, crr.Status)
						return nil
					},
				},
			)
		})
	}
}

func TestMustLoadMiddleware(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]string
		config    kubeconfig.NamespacedConfig
		configErr error
		want      kubeconfig.NamespacedConfig
		wantRes   *backend.CallResourceResponse
		wantErr   bool
	}{
		{
			name: "when keys are missing from secureJsonData",
			data: map[string]string{},
			config: kubeconfig.NamespacedConfig{
				RestConfig: rest.Config{
					Host: "https://some.url:6443",
				},
			},
			configErr: nil,
			want:      kubeconfig.NamespacedConfig{},
			wantRes: &backend.CallResourceResponse{
				Status: http.StatusInternalServerError,
				Body:   plugin.MarshalError(kubeconfig.ErrConfigMissing),
			},
			wantErr: true,
		},
		{
			name: "when config is missing from secureJsonData",
			data: map[string]string{
				kubeconfig.KeyNamespace: "namespace",
			},
			config: kubeconfig.NamespacedConfig{
				RestConfig: rest.Config{
					Host: "https://some.url:6443",
				},
			},
			configErr: nil,
			want:      kubeconfig.NamespacedConfig{},
			wantRes: &backend.CallResourceResponse{
				Status: http.StatusInternalServerError,
				Body:   plugin.MarshalError(kubeconfig.ErrConfigMissing),
			},
			wantErr: true,
		},
		{
			name: "when namespace key is missing from secureJsonData",
			data: map[string]string{
				kubeconfig.KeyConfig: "config",
			},
			config: kubeconfig.NamespacedConfig{
				RestConfig: rest.Config{
					Host: "https://some.url:6443",
				},
			},
			configErr: nil,
			want:      kubeconfig.NamespacedConfig{},
			wantRes: &backend.CallResourceResponse{
				Status: http.StatusInternalServerError,
				Body:   plugin.MarshalError(kubeconfig.ErrConfigMissing),
			},
			wantErr: true,
		},
		{
			name: "when config loader fails",
			data: map[string]string{
				kubeconfig.KeyConfig:    "config",
				kubeconfig.KeyNamespace: "namespace",
			},
			config:    kubeconfig.NamespacedConfig{},
			configErr: assert.AnError,
			want:      kubeconfig.NamespacedConfig{},
			wantRes: &backend.CallResourceResponse{
				Status: http.StatusInternalServerError,
				Body:   plugin.MarshalError(assert.AnError),
			},
			wantErr: true,
		},
		{
			name: "when config is added successfully",
			data: map[string]string{
				kubeconfig.KeyConfig:    "config",
				kubeconfig.KeyNamespace: "namespace",
			},
			config: kubeconfig.NamespacedConfig{
				RestConfig: rest.Config{
					Host: "https://some.url:6443",
				},
			},
			configErr: nil,
			want: kubeconfig.NamespacedConfig{
				Namespace: "namespace",
				RestConfig: rest.Config{
					Host: "https://some.url:6443",
				},
			},
			wantRes: &backend.CallResourceResponse{
				Status: http.StatusOK,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := kubeconfig.MustLoadMiddlewareWithLoader(&FakeConfigLoader{
				loadFn: func(s backend.AppInstanceSettings, c *kubeconfig.NamespacedConfig) error {
					if tt.configErr != nil {
						return tt.configErr
					}

					cf, ns, err := kubeconfig.LoadRawConfig(s.DecryptedSecureJSONData)
					if err != nil {
						return err
					}

					c.Namespace = ns
					if cf != "" {
						c.RestConfig = rest.Config{
							Host: "https://some.url:6443",
						}
					}

					return nil
				},
			})

			hl := mw(func(ctx context.Context, crr *backend.CallResourceRequest, crrs backend.CallResourceResponseSender) {
				cfg, err := kubeconfig.FromContext(ctx)

				assert.Equal(t, tt.want, cfg)
				assert.Equal(t, tt.wantErr, err != nil)

				crrs.Send(&backend.CallResourceResponse{
					Status: http.StatusOK,
				})
			})

			hl(
				context.Background(),
				&backend.CallResourceRequest{
					PluginContext: backend.PluginContext{
						AppInstanceSettings: &backend.AppInstanceSettings{
							DecryptedSecureJSONData: tt.data,
						},
					},
				},
				&FakeResponseSender{
					sendFn: func(crr *backend.CallResourceResponse) error {
						assert.Equal(t, tt.wantRes, crr)
						return nil
					},
				},
			)
		})
	}
}

type FakeConfigLoader struct {
	loadFn func(backend.AppInstanceSettings, *kubeconfig.NamespacedConfig) error
}

func (f *FakeConfigLoader) Load(config, namespace string, dst *kubeconfig.NamespacedConfig) error {
	return nil
}

func (f *FakeConfigLoader) LoadFromSettings(set backend.AppInstanceSettings, dst *kubeconfig.NamespacedConfig) error {
	if f.loadFn != nil {
		return f.loadFn(set, dst)
	}

	return nil
}

type FakeResponseSender struct {
	sendFn func(*backend.CallResourceResponse) error
}

func (f *FakeResponseSender) Send(res *backend.CallResourceResponse) error {
	if f.sendFn != nil {
		return f.sendFn(res)
	}

	return nil
}
