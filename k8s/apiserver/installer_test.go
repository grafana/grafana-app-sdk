package apiserver

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientrest "k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"
)

func TestDefaultInstaller_AddToScheme(t *testing.T) {
	t.Run("nil scheme", func(t *testing.T) {
		installer := defaultInstaller{}
		err := installer.AddToScheme(nil)
		assert.Equal(t, errors.New("scheme cannot be nil"), err)
	})

	t.Run("unresolvable kind", func(t *testing.T) {
		installer, err := NewDefaultAppInstaller(&simple.AppProvider{
			AppManifest: app.NewEmbeddedManifest(app.ManifestData{
				Group: "test.ext.grafana.com",
				Versions: []app.ManifestVersion{{
					Name: "v1",
					Kinds: []app.ManifestVersionKind{{
						Kind: "Foo",
					}},
				}},
			}),
		}, app.Config{}, func(kind, ver string) (resource.Kind, bool) {
			return resource.Kind{}, false
		})
		require.Nil(t, err)
		scheme := newScheme()
		err = installer.AddToScheme(scheme)
		assert.Equal(t, fmt.Errorf("failed to get kinds by group version: %w", errors.New("failed to resolve kind Foo")), err)
	})

	t.Run("success", func(t *testing.T) {
		installer, err := NewDefaultAppInstaller(&simple.AppProvider{
			AppManifest: app.NewEmbeddedManifest(app.ManifestData{
				Group: TestKind.Group(),
				Versions: []app.ManifestVersion{{
					Name: TestKind.Version(),
					Kinds: []app.ManifestVersionKind{{
						Kind:   TestKind.Kind(),
						Plural: TestKind.Plural(),
					}},
				}},
			}),
		}, app.Config{}, func(kind, ver string) (resource.Kind, bool) {
			return TestKind, true
		})
		require.Nil(t, err)
		scheme := newScheme()
		err = installer.AddToScheme(scheme)
		assert.Nil(t, err)
		known := scheme.KnownTypes(schema.GroupVersion{Group: TestKind.Group(), Version: TestKind.Version()})
		// 9 => Object, List, CreateOptions, GetOptions, UpdateOptions, DeleteOptions, ListOptions, PatchOptions, WatchEvent
		assert.Equal(t, 9, len(known))
		testKindVal, ok := known[TestKind.Kind()]
		require.True(t, ok)
		assert.Equal(t, reflect.TypeOf(resource.UntypedObject{}), testKindVal)
		testKindListVal, ok := known[TestKind.Kind()+"List"]
		require.True(t, ok)
		assert.Equal(t, reflect.TypeOf(resource.UntypedList{}), testKindListVal)
	})
}

func TestDefaultInstaller_GetOpenAPIDefinitions(t *testing.T) {
	// TODO
}

func TestDefaultInstaller_InstallAPIs(t *testing.T) {
	// TODO
}

func TestDefaultInstaller_AdmissionPlugin(t *testing.T) {
	// TODO
}

func TestDefaultInstaller_InitializeApp(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(app.ManifestData{}), nil, func(cfg app.Config) (app.App, error) {
			return nil, errors.New("I AM ERROR")
		}), app.Config{}, nil)
		require.Nil(t, err)
		err = installer.InitializeApp(clientrest.Config{})
		assert.Equal(t, errors.New("I AM ERROR"), err)
	})

	t.Run("already initialized", func(t *testing.T) {
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(app.ManifestData{}), nil, func(cfg app.Config) (app.App, error) {
			return nil, errors.New("I AM ERROR")
		}), app.Config{}, nil)
		require.Nil(t, err)
		installer.app = &MockApp{}
		err = installer.InitializeApp(clientrest.Config{})
		assert.Equal(t, ErrAppAlreadyInitialized, err)
	})

	t.Run("success", func(t *testing.T) {
		md := app.ManifestData{
			AppName: "test-app",
		}
		rcfg := clientrest.Config{
			Host: "foo",
		}
		initCalled := false
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(md), "foo", func(cfg app.Config) (app.App, error) {
			assert.Equal(t, "foo", cfg.SpecificConfig)
			assert.Equal(t, md, cfg.ManifestData)
			assert.Equal(t, rcfg, cfg.KubeConfig)
			initCalled = true
			return &MockApp{}, nil
		}), app.Config{}, nil)
		require.Nil(t, err)
		err = installer.InitializeApp(rcfg)
		require.Nil(t, err)
		assert.True(t, initCalled)
	})
}

func TestDefaultInstaller_App(t *testing.T) {
	t.Run("uninitialized", func(t *testing.T) {
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(app.ManifestData{}), nil, nil), app.Config{}, nil)
		require.Nil(t, err)
		app, err := installer.App()
		assert.Nil(t, app)
		assert.Equal(t, ErrAppNotInitialized, err)
	})

	t.Run("initialized", func(t *testing.T) {
		mockApp := &MockApp{}
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(app.ManifestData{}), nil, func(cfg app.Config) (app.App, error) {
			return mockApp, nil
		}), app.Config{}, nil)
		require.Nil(t, err)
		err = installer.InitializeApp(clientrest.Config{})
		require.Nil(t, err)
		app, err := installer.App()
		assert.Nil(t, err)
		assert.Equal(t, mockApp, app)
	})
}

func TestDefaultInstaller_GroupVersions(t *testing.T) {
	tests := []struct {
		manifest app.ManifestData
		expected []schema.GroupVersion
	}{{
		manifest: app.ManifestData{},
		expected: []schema.GroupVersion{},
	}, {
		manifest: app.ManifestData{
			Group: "test.ext.grafana.com",
			Versions: []app.ManifestVersion{{
				Name: "v1",
				Kinds: []app.ManifestVersionKind{{
					Kind: "Foo",
				}},
			}},
		},
		expected: []schema.GroupVersion{{Group: "test.ext.grafana.com", Version: "v1"}},
	}, {
		manifest: app.ManifestData{
			Group: "test.ext.grafana.com",
			Versions: []app.ManifestVersion{{
				Name: "v1",
				Kinds: []app.ManifestVersionKind{{
					Kind: "Foo",
				}},
			}, {
				Name: "v2alpha1",
				Kinds: []app.ManifestVersionKind{{
					Kind: "Foo",
				}},
			}, {
				Name: "v2alpha2",
				Kinds: []app.ManifestVersionKind{{
					Kind: "Foo",
				}},
			}, {
				Name: "v2beta1",
				Kinds: []app.ManifestVersionKind{{
					Kind: "Foo",
				}},
			}},
		},
		expected: []schema.GroupVersion{
			{Group: "test.ext.grafana.com", Version: "v1"},
			{Group: "test.ext.grafana.com", Version: "v2alpha1"},
			{Group: "test.ext.grafana.com", Version: "v2alpha2"},
			{Group: "test.ext.grafana.com", Version: "v2beta1"},
		},
	}}

	for idx, test := range tests {
		t.Run(fmt.Sprintf("%d", idx), func(t *testing.T) {
			installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(test.manifest), nil, nil), app.Config{}, nil)
			require.Nil(t, err)
			assert.Equal(t, test.expected, installer.GroupVersions())
		})
	}
}

func TestDefaultInstaller_ManifestData(t *testing.T) {
	data := app.ManifestData{
		Group: "test.ext.grafana.com",
		Versions: []app.ManifestVersion{{
			Name: "v1",
			Kinds: []app.ManifestVersionKind{{
				Kind: "Foo",
			}},
		}},
	}
	installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(data), nil, nil), app.Config{}, nil)
	require.Nil(t, err)
	assert.Equal(t, &data, installer.ManifestData())
}
