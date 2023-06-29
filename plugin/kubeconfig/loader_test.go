package kubeconfig_test

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/plugin/kubeconfig"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func TestLoader_Load(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		namespace string
		want      kubeconfig.NamespacedConfig
		wantErr   bool
	}{
		{
			name: "when an invalid config is passed",
			config: `{
				"kind": "Config",
				"contexts": [
					{
						"name": "default",
						"context": {
							"cluster": "testcluster",
							"user": "admin"
						}
					}
				],
				"current-context": "default"
			}`,
			namespace: "default",
			want:      kubeconfig.NamespacedConfig{},
			wantErr:   true,
		},
		{
			name: "when an invalid namespace is passed",
			config: `{
				"kind": "Config",
				"apiVersion": "v1",
				"preferences": {},
				"clusters": [
					{
						"name": "testcluster",
						"cluster": {
							"server": "https://localhost:6443",
							"certificate-authority-data": "ZGF0YQo="
						}
					}
				],
				"users": [
					{
						"name": "admin",
						"user": {
							"client-certificate-data": "ZGF0YQo=",
							"client-key-data": "ZGF0YQo="
						}
					}
				],
				"contexts": [
					{
						"name": "default",
						"context": {
							"cluster": "testcluster",
							"user": "admin"
						}
					}
				],
				"current-context": "default"
			}`,
			namespace: "",
			want:      kubeconfig.NamespacedConfig{},
			wantErr:   true,
		},
		{
			name: "when a valid config is passed",
			config: `{
				"kind": "Config",
				"apiVersion": "v1",
				"preferences": {},
				"clusters": [
					{
						"name": "testcluster",
						"cluster": {
							"server": "https://localhost:6443",
							"certificate-authority-data": "ZGF0YQo="
						}
					}
				],
				"users": [
					{
						"name": "admin",
						"user": {
							"client-certificate-data": "ZGF0YQo=",
							"client-key-data": "ZGF0YQo="
						}
					}
				],
				"contexts": [
					{
						"name": "default",
						"context": {
							"cluster": "testcluster",
							"user": "admin"
						}
					}
				],
				"current-context": "default"
			}`,
			namespace: "default",
			want: kubeconfig.NamespacedConfig{
				CRC32:     3817579672,
				Namespace: "default",
				RestConfig: rest.Config{
					Host:    "https://localhost:6443",
					APIPath: "/apis",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   []byte{100, 97, 116, 97, 10},
						CertData: []byte{100, 97, 116, 97, 10},
						KeyData:  []byte{100, 97, 116, 97, 10},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "when a valid config is passed with a non-default namespace",
			config: `{
				"kind": "Config",
				"apiVersion": "v1",
				"preferences": {},
				"clusters": [
					{
						"name": "testcluster",
						"cluster": {
							"server": "https://localhost:6443",
							"certificate-authority-data": "ZGF0YQo="
						}
					}
				],
				"users": [
					{
						"name": "admin",
						"user": {
							"client-certificate-data": "ZGF0YQo=",
							"client-key-data": "ZGF0YQo="
						}
					}
				],
				"contexts": [
					{
						"name": "default",
						"context": {
							"cluster": "testcluster",
							"user": "admin"
						}
					}
				],
				"current-context": "default"
			}`,
			namespace: "custom",
			want: kubeconfig.NamespacedConfig{
				CRC32:     538988955,
				Namespace: "custom",
				RestConfig: rest.Config{
					Host:    "https://localhost:6443",
					APIPath: "/apis",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   []byte{100, 97, 116, 97, 10},
						CertData: []byte{100, 97, 116, 97, 10},
						KeyData:  []byte{100, 97, 116, 97, 10},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := kubeconfig.NewLoader()

			var res kubeconfig.NamespacedConfig
			err := l.Load(tt.config, tt.namespace, &res)

			assert.Equal(t, tt.want, res)
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestLoader_LoadFromSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings backend.AppInstanceSettings
		want     kubeconfig.NamespacedConfig
		wantErr  bool
	}{
		{
			name: "when valid config keys are passed as raw strings",
			settings: backend.AppInstanceSettings{
				DecryptedSecureJSONData: map[string]string{
					kubeconfig.KeyNamespace: "custom",
					kubeconfig.KeyConfig: `{
						"kind": "Config",
						"apiVersion": "v1",
						"preferences": {},
						"clusters": [
							{
								"name": "testcluster",
								"cluster": {
									"server": "https://localhost:6443",
									"certificate-authority-data": "ZGF0YQo="
								}
							}
						],
						"users": [
							{
								"name": "admin",
								"user": {
									"client-certificate-data": "ZGF0YQo=",
									"client-key-data": "ZGF0YQo="
								}
							}
						],
						"contexts": [
							{
								"name": "default",
								"context": {
									"cluster": "testcluster",
									"user": "admin"
								}
							}
						],
						"current-context": "default"
					}`,
				},
			},
			want: kubeconfig.NamespacedConfig{
				CRC32:     1362417121,
				Namespace: "custom",
				RestConfig: rest.Config{
					Host:    "https://localhost:6443",
					APIPath: "/apis",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   []byte{100, 97, 116, 97, 10},
						CertData: []byte{100, 97, 116, 97, 10},
						KeyData:  []byte{100, 97, 116, 97, 10},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "when valid config keys are passed as raw strings and config is in YAML",
			settings: backend.AppInstanceSettings{
				DecryptedSecureJSONData: map[string]string{
					kubeconfig.KeyNamespace: "custom",
					kubeconfig.KeyConfig: `
kind: Config
apiVersion: v1
preferences: {}
clusters:
  - name: testcluster
    cluster:
      server: https://localhost:6443
      certificate-authority-data: ZGF0YQo=
users:
  - name: admin
    user:
      client-certificate-data: ZGF0YQo=
      client-key-data: ZGF0YQo=
contexts:
  - name: default
    context:
      cluster: testcluster
      user: admin
current-context: default
`,
				},
			},
			want: kubeconfig.NamespacedConfig{
				CRC32:     3859317101,
				Namespace: "custom",
				RestConfig: rest.Config{
					Host:    "https://localhost:6443",
					APIPath: "/apis",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   []byte{100, 97, 116, 97, 10},
						CertData: []byte{100, 97, 116, 97, 10},
						KeyData:  []byte{100, 97, 116, 97, 10},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "when valid config keys are passed as base64-encoded strings",
			settings: backend.AppInstanceSettings{
				DecryptedSecureJSONData: map[string]string{
					kubeconfig.KeyNamespace: "Y3VzdG9t",
					kubeconfig.KeyConfig:    "ewogICJraW5kIjogIkNvbmZpZyIsCiAgImFwaVZlcnNpb24iOiAidjEiLAogICJwcmVmZXJlbmNlcyI6IHt9LAogICJjbHVzdGVycyI6IFsKICAgIHsKICAgICAgIm5hbWUiOiAidGVzdGNsdXN0ZXIiLAogICAgICAiY2x1c3RlciI6IHsKICAgICAgICAic2VydmVyIjogImh0dHBzOi8vbG9jYWxob3N0OjY0NDMiLAogICAgICAgICJjZXJ0aWZpY2F0ZS1hdXRob3JpdHktZGF0YSI6ICJaR0YwWVFvPSIKICAgICAgfQogICAgfQogIF0sCiAgInVzZXJzIjogWwogICAgewogICAgICAibmFtZSI6ICJhZG1pbiIsCiAgICAgICJ1c2VyIjogewogICAgICAgICJjbGllbnQtY2VydGlmaWNhdGUtZGF0YSI6ICJaR0YwWVFvPSIsCiAgICAgICAgImNsaWVudC1rZXktZGF0YSI6ICJaR0YwWVFvPSIKICAgICAgfQogICAgfQogIF0sCiAgImNvbnRleHRzIjogWwogICAgewogICAgICAibmFtZSI6ICJkZWZhdWx0IiwKICAgICAgImNvbnRleHQiOiB7CiAgICAgICAgImNsdXN0ZXIiOiAidGVzdGNsdXN0ZXIiLAogICAgICAgICJ1c2VyIjogImFkbWluIgogICAgICB9CiAgICB9CiAgXSwKICAiY3VycmVudC1jb250ZXh0IjogImRlZmF1bHQiCn0K",
				},
			},
			want: kubeconfig.NamespacedConfig{
				CRC32:     1920780822,
				Namespace: "custom",
				RestConfig: rest.Config{
					Host:    "https://localhost:6443",
					APIPath: "/apis",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   []byte{100, 97, 116, 97, 10},
						CertData: []byte{100, 97, 116, 97, 10},
						KeyData:  []byte{100, 97, 116, 97, 10},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "when valid config keys are passed as base64-encoded strings and config is in YAML",
			settings: backend.AppInstanceSettings{
				DecryptedSecureJSONData: map[string]string{
					kubeconfig.KeyNamespace: "Y3VzdG9t",
					kubeconfig.KeyConfig:    "a2luZDogQ29uZmlnCmFwaVZlcnNpb246IHYxCnByZWZlcmVuY2VzOiB7fQpjbHVzdGVyczoKICAtIG5hbWU6IHRlc3RjbHVzdGVyCiAgICBjbHVzdGVyOgogICAgICBzZXJ2ZXI6IGh0dHBzOi8vbG9jYWxob3N0OjY0NDMKICAgICAgY2VydGlmaWNhdGUtYXV0aG9yaXR5LWRhdGE6IFpHRjBZUW89CnVzZXJzOgogIC0gbmFtZTogYWRtaW4KICAgIHVzZXI6CiAgICAgIGNsaWVudC1jZXJ0aWZpY2F0ZS1kYXRhOiBaR0YwWVFvPQogICAgICBjbGllbnQta2V5LWRhdGE6IFpHRjBZUW89CmNvbnRleHRzOgogIC0gbmFtZTogZGVmYXVsdAogICAgY29udGV4dDoKICAgICAgY2x1c3RlcjogdGVzdGNsdXN0ZXIKICAgICAgdXNlcjogYWRtaW4KY3VycmVudC1jb250ZXh0OiBkZWZhdWx0Cg==",
				},
			},
			want: kubeconfig.NamespacedConfig{
				CRC32:     3872305992,
				Namespace: "custom",
				RestConfig: rest.Config{
					Host:    "https://localhost:6443",
					APIPath: "/apis",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   []byte{100, 97, 116, 97, 10},
						CertData: []byte{100, 97, 116, 97, 10},
						KeyData:  []byte{100, 97, 116, 97, 10},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := kubeconfig.NewLoader()

			var res kubeconfig.NamespacedConfig
			err := l.LoadFromSettings(tt.settings, &res)

			assert.Equal(t, tt.want, res)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCachingLoader_Load(t *testing.T) {
	t.Run("should forward errors", func(t *testing.T) {
		assert.Error(
			t,
			kubeconfig.NewCustomCachingLoader(&FakeLoader{
				crcFn: func(s1, s2 string) (uint32, error) {
					return 0, assert.AnError
				},
			}).Load("config", "namespace", &kubeconfig.NamespacedConfig{}),
		)

		assert.Error(
			t,
			kubeconfig.NewCustomCachingLoader(&FakeLoader{
				loadFn: func(conf, ns string, nc *kubeconfig.NamespacedConfig) error {
					return assert.AnError
				},
			}).Load("config", "namespace", &kubeconfig.NamespacedConfig{}),
		)
	})

	t.Run("should only load once if crc doesn't change", func(t *testing.T) {
		cfg := kubeconfig.NamespacedConfig{
			RestConfig: rest.Config{
				Host: "https://some.url:6443",
			},
			Namespace: "namespace",
		}

		var loadCalls int

		l := kubeconfig.NewCustomCachingLoader(&FakeLoader{
			loadFn: func(conf, ns string, nc *kubeconfig.NamespacedConfig) error {
				assert.Equal(t, conf, "config")
				assert.Equal(t, ns, "namespace")

				loadCalls++
				*nc = cfg
				return nil
			},
			crcFn: func(s1, s2 string) (uint32, error) {
				return 1234, nil
			},
		})

		var res kubeconfig.NamespacedConfig

		_ = l.Load("config", "namespace", &res)
		assert.Equal(t, cfg, res)
		_ = l.Load("config", "namespace", &res)
		assert.Equal(t, cfg, res)
		_ = l.Load("config", "namespace", &res)
		assert.Equal(t, cfg, res)

		assert.Equal(t, 1, loadCalls)
	})

	t.Run("should load multiple times if keeps changing", func(t *testing.T) {
		cfg := []kubeconfig.NamespacedConfig{
			{
				RestConfig: rest.Config{
					Host: "https://some.url:6443",
				},
				Namespace: "namespace1",
			},
			{
				RestConfig: rest.Config{
					Host: "https://some.url:6443",
				},
				Namespace: "namespace2",
			},
			{
				RestConfig: rest.Config{
					Host: "https://some.url:6443",
				},
				Namespace: "namespace3",
			},
		}

		var loadCalls int

		l := kubeconfig.NewCustomCachingLoader(&FakeLoader{
			loadFn: func(conf, ns string, nc *kubeconfig.NamespacedConfig) error {
				assert.Equal(t, conf, "config")
				assert.Equal(t, ns, "namespace")

				*nc = cfg[loadCalls]
				loadCalls++
				return nil
			},
			crcFn: func(s1, s2 string) (uint32, error) {
				return rand.Uint32(), nil
			},
		})

		var res kubeconfig.NamespacedConfig

		_ = l.Load("config", "namespace", &res)
		assert.Equal(t, cfg[0], res)
		_ = l.Load("config", "namespace", &res)
		assert.Equal(t, cfg[1], res)
		_ = l.Load("config", "namespace", &res)
		assert.Equal(t, cfg[2], res)

		assert.Equal(t, 3, loadCalls)
	})
}

type FakeLoader struct {
	crcFn     func(string, string) (uint32, error)
	loadFn    func(string, string, *kubeconfig.NamespacedConfig) error
	loadSetFn func(backend.AppInstanceSettings, *kubeconfig.NamespacedConfig) error
}

func (f *FakeLoader) Load(config, namespace string, dst *kubeconfig.NamespacedConfig) error {
	if f.loadFn != nil {
		return f.loadFn(config, namespace, dst)
	}

	return nil
}

func (f *FakeLoader) LoadFromSettings(set backend.AppInstanceSettings, dst *kubeconfig.NamespacedConfig) error {
	if f.loadFn != nil {
		return f.loadSetFn(set, dst)
	}

	return nil
}

func (f *FakeLoader) CRC32(config, namespace string) (uint32, error) {
	if f.crcFn != nil {
		return f.crcFn(config, namespace)
	}

	return 0, nil
}
