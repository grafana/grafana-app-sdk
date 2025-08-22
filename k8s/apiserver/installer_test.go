package apiserver

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapiserver "k8s.io/apiserver/pkg/server"
	clientrest "k8s.io/client-go/rest"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

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
		}, app.Config{}, &mockGoTypeResolver{
			KindToGoTypeFunc: func(kind, ver string) (resource.Kind, bool) {
				return resource.Kind{}, false
			},
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
		}, app.Config{}, &mockGoTypeResolver{
			KindToGoTypeFunc: func(kind, ver string) (resource.Kind, bool) {
				return TestKind, true
			},
		})
		require.Nil(t, err)
		scheme := newScheme()
		err = installer.AddToScheme(scheme)
		assert.Nil(t, err)
		known := scheme.KnownTypes(schema.GroupVersion{Group: TestKind.Group(), Version: TestKind.Version()})
		// 10 => Object, List, CreateOptions, GetOptions, UpdateOptions, DeleteOptions, ListOptions, PatchOptions, WatchEvent, ResourceCallOptions
		assert.Equal(t, 10, len(known))
		testKindVal, ok := known[TestKind.Kind()]
		require.True(t, ok)
		assert.Equal(t, reflect.TypeOf(resource.UntypedObject{}), testKindVal)
		testKindListVal, ok := known[TestKind.Kind()+"List"]
		require.True(t, ok)
		assert.Equal(t, reflect.TypeOf(resource.UntypedList{}), testKindListVal)
	})
}

func TestDefaultInstaller_GetOpenAPIDefinitions(t *testing.T) {
	sch1, err := app.VersionSchemaFromMap(map[string]any{
		"spec": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"foo": map[string]any{
					"type": "string",
				},
			},
		},
	})
	fooSch := spec.Schema{
		SchemaProps: spec.SchemaProps{
			ID: "foo",
		},
	}
	kind := TestKind
	require.Nil(t, err)
	md := app.ManifestData{
		Group: kind.Group(),
		Versions: []app.ManifestVersion{{
			Name: kind.Version(),
			Kinds: []app.ManifestVersionKind{{
				Kind:   kind.Kind(),
				Schema: sch1,
				Routes: map[string]spec3.PathProps{
					"/foo": {
						Get: &spec3.Operation{
							OperationProps: spec3.OperationProps{
								Responses: &spec3.Responses{
									ResponsesProps: spec3.ResponsesProps{
										Default: &spec3.Response{
											ResponseProps: spec3.ResponseProps{
												Content: map[string]*spec3.MediaType{
													"application/json": {
														MediaTypeProps: spec3.MediaTypeProps{
															Schema: &fooSch,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}},
		}},
	}
	refCallback := func(path string) spec.Ref {
		ref, _ := spec.NewRef(path)
		return ref
	}
	expected := make(map[string]common.OpenAPIDefinition)
	oapi1, err := md.Versions[0].Kinds[0].Schema.AsKubeOpenAPI(kind.GroupVersionKind(), refCallback, "github.com/grafana/grafana-app-sdk/resource")
	require.Nil(t, err)
	maps.Copy(expected, oapi1)
	maps.Copy(expected, GetResourceCallOptionsOpenAPIDefinition())
	expected["/registry/grafana.app.GetFoo"] = common.OpenAPIDefinition{
		Schema: fooSch,
	}

	installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(md), nil, nil), app.Config{}, &mockGoTypeResolver{
		KindToGoTypeFunc: func(k, v string) (resource.Kind, bool) {
			return kind, true
		},
	})
	require.Nil(t, err)
	scheme := newScheme()
	require.Nil(t, installer.AddToScheme(scheme))
	res := installer.GetOpenAPIDefinitions(refCallback)
	require.Equal(t, len(expected), len(res))
	assert.Equal(t, expected, res)
}

func TestDefaultInstaller_InstallAPIs(t *testing.T) {
	md := app.ManifestData{
		Versions: []app.ManifestVersion{{
			Name:   TestKind.Version(),
			Served: true,
			Kinds: []app.ManifestVersionKind{{
				Kind:   TestKind.Kind(),
				Plural: TestKind.Plural(),
				Admission: &app.AdmissionCapabilities{
					Validation: &app.ValidationCapability{
						Operations: []app.AdmissionOperation{app.AdmissionOperationAny},
					},
				},
				Routes: map[string]spec3.PathProps{
					"/foo": {
						Get: &spec3.Operation{},
					},
				},
			}},
		}},
	}
	t.Run("error adding to scheme", func(t *testing.T) {
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(md), nil, nil), app.Config{}, &mockGoTypeResolver{
			KindToGoTypeFunc: func(kind, ver string) (resource.Kind, bool) {
				return TestKind, false
			},
		})
		require.Nil(t, err)
		err = installer.InstallAPIs(&MockGenericAPIServer{
			InstallAPIGroupFunc: func(_ *genericapiserver.APIGroupInfo) error {
				assert.Fail(t, "should not be called")
				return errors.New("I AM ERROR")
			},
		}, nil)
		assert.NotNil(t, err)
		assert.EqualError(t, err, "failed to add to scheme: failed to get kinds by group version: failed to resolve kind Test")
	})

	t.Run("error getting groupversions", func(t *testing.T) {
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(md), nil, nil), app.Config{}, &mockGoTypeResolver{
			KindToGoTypeFunc: func(kind, ver string) (resource.Kind, bool) {
				return TestKind, false
			},
		})
		require.Nil(t, err)
		installer.scheme = newScheme()
		err = installer.InstallAPIs(&MockGenericAPIServer{
			InstallAPIGroupFunc: func(_ *genericapiserver.APIGroupInfo) error {
				assert.Fail(t, "should not be called")
				return errors.New("I AM ERROR")
			},
		}, nil)
		assert.NotNil(t, err)
		assert.EqualError(t, err, "failed to get kinds by group version: failed to resolve kind Test")
	})

	t.Run("error creating store", func(t *testing.T) {
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(md), nil, nil), app.Config{}, &mockGoTypeResolver{
			KindToGoTypeFunc: func(kind, ver string) (resource.Kind, bool) {
				return TestKind, true
			},
		})
		require.Nil(t, err)
		err = installer.InstallAPIs(&MockGenericAPIServer{
			InstallAPIGroupFunc: func(_ *genericapiserver.APIGroupInfo) error {
				assert.Fail(t, "should not be called")
				return errors.New("I AM ERROR")
			},
		}, nil)
		assert.NotNil(t, err)
		assert.EqualError(t, err, "failed to create store for kind Test: failed completing storage options for Test: options for tests.test.ext.grafana.app must have RESTOptions set")
	})
}

func TestDefaultInstaller_AdmissionPlugin(t *testing.T) {
	t.Run("no admission control", func(t *testing.T) {
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(app.ManifestData{}), nil, nil), app.Config{}, nil)
		require.Nil(t, err)
		plugin := installer.AdmissionPlugin()
		assert.Nil(t, plugin)
	})

	t.Run("validation", func(t *testing.T) {
		md := app.ManifestData{
			Versions: []app.ManifestVersion{{
				Name:   TestKind.Version(),
				Served: true,
				Kinds: []app.ManifestVersionKind{{
					Kind:   TestKind.Kind(),
					Plural: TestKind.Plural(),
					Admission: &app.AdmissionCapabilities{
						Validation: &app.ValidationCapability{
							Operations: []app.AdmissionOperation{app.AdmissionOperationAny},
						},
					},
				}},
			}},
		}
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(md), nil, nil), app.Config{}, nil)
		require.Nil(t, err)
		plugin := installer.AdmissionPlugin()
		assert.NotNil(t, plugin)
		adm, err := plugin(nil)
		require.Nil(t, err)
		appAdm, ok := adm.(*appAdmission)
		require.True(t, ok)
		assert.Equal(t, md, appAdm.manifestData)
	})

	t.Run("mutation", func(t *testing.T) {
		md := app.ManifestData{
			Versions: []app.ManifestVersion{{
				Name:   TestKind.Version(),
				Served: true,
				Kinds: []app.ManifestVersionKind{{
					Kind:   TestKind.Kind(),
					Plural: TestKind.Plural(),
					Admission: &app.AdmissionCapabilities{
						Mutation: &app.MutationCapability{
							Operations: []app.AdmissionOperation{app.AdmissionOperationAny},
						},
					},
				}},
			}},
		}
		installer, err := NewDefaultAppInstaller(simple.NewAppProvider(app.NewEmbeddedManifest(md), nil, nil), app.Config{}, nil)
		require.Nil(t, err)
		plugin := installer.AdmissionPlugin()
		assert.NotNil(t, plugin)
		adm, err := plugin(nil)
		require.Nil(t, err)
		appAdm, ok := adm.(*appAdmission)
		require.True(t, ok)
		assert.Equal(t, md, appAdm.manifestData)
	})
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

type MockGenericAPIServer struct {
	InstallAPIGroupFunc func(apiGroupInfo *genericapiserver.APIGroupInfo) error
}

func (m *MockGenericAPIServer) InstallAPIGroup(apiGroupInfo *genericapiserver.APIGroupInfo) error {
	if m.InstallAPIGroupFunc != nil {
		return m.InstallAPIGroupFunc(apiGroupInfo)
	}
	return nil
}

type mockGoTypeResolver struct {
	KindToGoTypeFunc            func(kind, version string) (goType resource.Kind, exists bool)
	CustomRouteReturnGoTypeFunc func(kind, version, path, verb string) (goType any, exists bool)
	CustomRouteQueryGoTypeFunc  func(kind, version, path, verb string) (goType runtime.Object, exists bool)
}

func (m *mockGoTypeResolver) KindToGoType(kind, version string) (goType resource.Kind, exists bool) {
	if m.KindToGoTypeFunc != nil {
		return m.KindToGoTypeFunc(kind, version)
	}
	return resource.Kind{}, false
}
func (m *mockGoTypeResolver) CustomRouteReturnGoType(kind, version, path, verb string) (goType any, exists bool) {
	if m.CustomRouteReturnGoTypeFunc != nil {
		return m.CustomRouteReturnGoTypeFunc(kind, version, path, verb)
	}
	return nil, false
}
func (m *mockGoTypeResolver) CustomRouteQueryGoType(kind, version, path, verb string) (goType runtime.Object, exists bool) {
	if m.CustomRouteQueryGoTypeFunc != nil {
		return m.CustomRouteQueryGoTypeFunc(kind, version, path, verb)
	}
	return nil, false
}
