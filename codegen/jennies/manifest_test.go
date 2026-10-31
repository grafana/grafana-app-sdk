package jennies

import (
	"cuelang.org/go/cue"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/routes"
)

func TestResolveOperatorURL(t *testing.T) {
	tests := []struct {
		name    string
		props   codegen.AppManifestProperties
		want    *string
		wantErr string
	}{
		{
			name:  "neither set",
			props: codegen.AppManifestProperties{},
			want:  nil,
		},
		{
			name:  "deprecated operatorURL only",
			props: codegen.AppManifestProperties{OperatorURL: new("https://foo.bar:8443")},
			want:  new("https://foo.bar:8443"),
		},
		{
			name: "structured operator.url only",
			props: codegen.AppManifestProperties{
				Operator: &codegen.AppManifestPropertiesOperatorInfo{URL: new("https://foo.bar:8443")},
			},
			want: new("https://foo.bar:8443"),
		},
		{
			name: "both set to same value",
			props: codegen.AppManifestProperties{
				OperatorURL: new("https://foo.bar:8443"),
				Operator:    &codegen.AppManifestPropertiesOperatorInfo{URL: new("https://foo.bar:8443")},
			},
			want: new("https://foo.bar:8443"),
		},
		{
			name: "both set to different values errors",
			props: codegen.AppManifestProperties{
				OperatorURL: new("https://deprecated:8443"),
				Operator:    &codegen.AppManifestPropertiesOperatorInfo{URL: new("https://structured:8443")},
			},
			wantErr: "both set but differ",
		},
		{
			name: "operator set without url falls back to deprecated",
			props: codegen.AppManifestProperties{
				OperatorURL: new("https://foo.bar:8443"),
				Operator:    &codegen.AppManifestPropertiesOperatorInfo{},
			},
			want: new("https://foo.bar:8443"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveOperatorURL(tt.props)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOperatorWebhookPaths(t *testing.T) {
	tests := []struct {
		name                                    string
		props                                   codegen.AppManifestProperties
		wantConversion, wantValidation, wantMut string
	}{
		{
			name:           "no operator uses defaults",
			props:          codegen.AppManifestProperties{},
			wantConversion: "/convert",
			wantValidation: "/validate",
			wantMut:        "/mutate",
		},
		{
			name: "operator without webhooks uses defaults",
			props: codegen.AppManifestProperties{
				Operator: &codegen.AppManifestPropertiesOperatorInfo{URL: new("https://foo.bar")},
			},
			wantConversion: "/convert",
			wantValidation: "/validate",
			wantMut:        "/mutate",
		},
		{
			name: "custom paths override defaults",
			props: codegen.AppManifestProperties{
				Operator: &codegen.AppManifestPropertiesOperatorInfo{
					Webhooks: &codegen.AppManifestPropertiesOperatorWebhookProperties{
						ConversionPath: "/custom/convert",
						ValidationPath: "/custom/validate",
						MutationPath:   "/custom/mutate",
					},
				},
			},
			wantConversion: "/custom/convert",
			wantValidation: "/custom/validate",
			wantMut:        "/custom/mutate",
		},
		{
			name: "partial override keeps defaults for unset paths",
			props: codegen.AppManifestProperties{
				Operator: &codegen.AppManifestPropertiesOperatorInfo{
					Webhooks: &codegen.AppManifestPropertiesOperatorWebhookProperties{
						ValidationPath: "/custom/validate",
					},
				},
			},
			wantConversion: "/convert",
			wantValidation: "/custom/validate",
			wantMut:        "/mutate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conversion, validation, mutation := operatorWebhookPaths(tt.props)
			assert.Equal(t, tt.wantConversion, conversion)
			assert.Equal(t, tt.wantValidation, validation)
			assert.Equal(t, tt.wantMut, mutation)
		})
	}
}

func TestJoinKindNames(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect string
	}{
		{
			name:   "empty",
			input:  []string{},
			expect: "",
		},
		{
			name:   "single kind",
			input:  []string{"Blueprints"},
			expect: "Blueprints",
		},
		{
			name:   "two kinds",
			input:  []string{"Blueprints", "Timelines"},
			expect: "Blueprints and Timelines",
		},
		{
			name:   "three kinds with Oxford comma",
			input:  []string{"Blueprints", "StepTypes", "Timelines"},
			expect: "Blueprints, StepTypes, and Timelines",
		},
		{
			name:   "four kinds",
			input:  []string{"As", "Bs", "Cs", "Ds"},
			expect: "As, Bs, Cs, and Ds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinKindNames(tt.input)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestProcessKindVersion_Search(t *testing.T) {
	for _, tc := range []struct {
		name     string
		search   codegen.KindSearch
		expected *app.ManifestVersionKindSearch
	}{{
		// Only a choice that differs from its default is written to the manifest,
		// so all three at their defaults produce nothing at all.
		name:     "defaults",
		search:   codegen.KindSearch{Endpoint: true, Trash: true, Hybrid: false},
		expected: nil,
	}, {
		name:     "search opt-out",
		search:   codegen.KindSearch{Endpoint: false, Trash: true},
		expected: &app.ManifestVersionKindSearch{Endpoint: new(false)},
	}, {
		name:     "trash opt-out",
		search:   codegen.KindSearch{Endpoint: true, Trash: false},
		expected: &app.ManifestVersionKindSearch{Trash: new(false)},
	}, {
		name:     "both opt-out",
		search:   codegen.KindSearch{Endpoint: false, Trash: false},
		expected: &app.ManifestVersionKindSearch{Endpoint: new(false), Trash: new(false)},
	}, {
		// Hybrid defaults off, so unlike the other two it is written when true.
		name:     "hybrid opt-in",
		search:   codegen.KindSearch{Endpoint: true, Trash: true, Hybrid: true},
		expected: &app.ManifestVersionKindSearch{Hybrid: new(true)},
	}, {
		name:     "hybrid opt-in with trash opt-out",
		search:   codegen.KindSearch{Endpoint: true, Trash: false, Hybrid: true},
		expected: &app.ManifestVersionKindSearch{Trash: new(false), Hybrid: new(true)},
	}, {
		// Global defaults off, so it is written only when true.
		name:     "global opt-in",
		search:   codegen.KindSearch{Endpoint: true, Trash: true, Global: true},
		expected: &app.ManifestVersionKindSearch{Global: new(true)},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			mver, err := processKindVersion(codegen.VersionedKind{
				Kind:         "Foo",
				PluralName:   "Foos",
				Scope:        "Namespaced",
				FolderScoped: true,
				Search:       tc.search,
			}, "v1", false)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, mver.Search)
		})
	}
}

func TestProcessKindVersion_Embed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		embed    *codegen.KindEmbed
		expected *app.ManifestVersionKindEmbed
	}{
		{name: "unset", embed: nil, expected: nil},
		{
			name: "independent fields retain declaration order",
			embed: &codegen.KindEmbed{
				Fields: []codegen.EmbedField{
					{Name: "summary", Path: "spec.summary"},
					{Name: "title", Path: "spec.title"},
				},
			},
			expected: &app.ManifestVersionKindEmbed{
				Fields: []app.ManifestVersionKindEmbedField{
					{Name: "summary", Path: "spec.summary"},
					{Name: "title", Path: "spec.title"},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mver, err := processKindVersion(codegen.VersionedKind{
				Kind:         "Foo",
				PluralName:   "Foos",
				Scope:        "Namespaced",
				FolderScoped: true,
				Embed:        tc.embed,
			}, "v1", false)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, mver.Embed)
			assert.Empty(t, mver.SearchFields)
		})
	}
}

func TestProcessKindVersion_IndependentSearchAndEmbedFields(t *testing.T) {
	schema := cuecontext.New().CompileString(`spec: { title: string, summary: string, count: int }`)
	require.NoError(t, schema.Err())

	mver, err := processKindVersion(codegen.VersionedKind{
		Kind:         "Foo",
		PluralName:   "Foos",
		Scope:        "Namespaced",
		FolderScoped: true,
		Schema:       schema,
		SearchFields: []codegen.SearchField{
			{Name: "title", Path: "spec.title", Type: "string", Capabilities: []string{"text"}},
			{Name: "count", Path: "spec.count", Type: "int64", Capabilities: []string{"filter"}},
		},
		Embed: &codegen.KindEmbed{
			Fields: []codegen.EmbedField{
				{Name: "summary", Path: "spec.summary"},
			},
		},
	}, "v1", false)
	require.NoError(t, err)
	assert.Equal(t, []app.ManifestVersionKindSearchField{
		{Name: "title", Path: "spec.title", Type: "string", Capabilities: []string{"text"}},
		{Name: "count", Path: "spec.count", Type: "int64", Capabilities: []string{"filter"}},
	}, mver.SearchFields)
	require.NotNil(t, mver.Embed)
	assert.Equal(t, []app.ManifestVersionKindEmbedField{
		{Name: "summary", Path: "spec.summary"},
	}, mver.Embed.Fields)
}

func TestBuildManifestData_GlobalEmbedReembedVersion(t *testing.T) {
	ctx := cuecontext.New()
	for _, tt := range []struct {
		name    string
		embed   map[string]codegen.ResourceEmbed
		wantErr string
	}{
		{
			name:  "one revision for different versioned fields",
			embed: map[string]codegen.ResourceEmbed{"foos": {ReembedVersion: 3}},
		},
		{
			name:    "versioned fields require a resource revision",
			wantErr: "foos",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manifest := &codegen.SimpleManifest{
				AppManifestProperties: codegen.AppManifestProperties{
					AppName:   "test",
					FullGroup: "test.grafana.app",
					Embed:     tt.embed,
				},
				AllVersions: map[string]*codegen.SimpleVersion{
					"v1": {
						VersionProperties: codegen.VersionProperties{Name: "v1"},
						AllKinds: []codegen.VersionedKind{{
							Kind:       "Foo",
							PluralName: "Foos",
							Scope:      "Namespaced",
							Schema:     ctx.CompileString(`spec: { title: string }`),
							Embed: &codegen.KindEmbed{
								Fields: []codegen.EmbedField{{Name: "title", Path: "spec.title"}},
							},
						}},
					},
					"v2": {
						VersionProperties: codegen.VersionProperties{Name: "v2"},
						AllKinds: []codegen.VersionedKind{{
							Kind:       "Foo",
							PluralName: "Foos",
							Scope:      "Namespaced",
							Schema:     ctx.CompileString(`spec: { displayName: string }`),
							Embed: &codegen.KindEmbed{
								Fields: []codegen.EmbedField{{Name: "title", Path: "spec.displayName"}},
							},
						}},
					},
				},
			}

			got, err := buildManifestData(manifest, false)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, map[string]app.ManifestResourceEmbed{"foos": {ReembedVersion: 3}}, got.Embed)
			require.Len(t, got.Versions, 2)
			require.Len(t, got.Versions[0].Kinds, 1)
			require.Len(t, got.Versions[1].Kinds, 1)
			require.NotNil(t, got.Versions[0].Kinds[0].Embed)
			require.NotNil(t, got.Versions[1].Kinds[0].Embed)
			assert.Equal(t, []app.ManifestVersionKindEmbedField{{Name: "title", Path: "spec.title"}}, got.Versions[0].Kinds[0].Embed.Fields)
			assert.Equal(t, []app.ManifestVersionKindEmbedField{{Name: "title", Path: "spec.displayName"}}, got.Versions[1].Kinds[0].Embed.Fields)
		})
	}
}

func TestBuildManifestData_ValidateRoutes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "valid route", path: "/query/{id}"},
		{name: "invalid route", path: "/{resource}/query", wantErr: "v1 /{resource}/query: a route must start with a literal segment"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manifest := &codegen.SimpleManifest{
				AppManifestProperties: codegen.AppManifestProperties{AppName: "test", FullGroup: "test.grafana.app"},
				AllVersions: map[string]*codegen.SimpleVersion{
					"v1": {
						VersionProperties: codegen.VersionProperties{Name: "v1"},
						ImportedRoutes: app.ManifestVersionRoutes{ //nolint:staticcheck // ImportedRoutes uses the legacy route representation.
							Cluster: map[string]spec3.PathProps{tt.path: {Get: &spec3.Operation{}}},
						},
					},
				},
			}
			_, err := buildManifestData(manifest, false)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBuildManifestData_RejectsReservedKindRoutes(t *testing.T) {
	schema := cuecontext.New().CompileString("{}")
	manifest := &codegen.SimpleManifest{
		AppManifestProperties: codegen.AppManifestProperties{AppName: "test", FullGroup: "test.grafana.app"},
		AllVersions: map[string]*codegen.SimpleVersion{
			"v1": {
				VersionProperties: codegen.VersionProperties{Name: "v1"},
				AllKinds: []codegen.VersionedKind{{
					Kind:       "Foo",
					PluralName: "foos",
					Scope:      "Namespaced",
					Schema:     schema,
				}},
				CustomRoutes: &codegen.VersionCustomRoutes{
					Namespaced: map[string]map[string]codegen.CustomRoute{
						"foos/search": {},
					},
				},
			},
		},
	}

	_, err := buildManifestData(manifest, false)
	require.ErrorContains(t, err, "custom route 'foos/search' conflicts with reserved 'search' route for kind 'foos'")
}

func TestCustomRouteExtensions(t *testing.T) {
	tests := []struct {
		name  string
		route codegen.CustomRoute
		want  spec.Extensions
	}{
		{
			name:  "no extensions and no authz",
			route: codegen.CustomRoute{},
			want:  nil,
		},
		{
			name: "extensions only",
			route: codegen.CustomRoute{
				Extensions: map[string]any{"x-foo": true},
			},
			want: spec.Extensions{"x-foo": true},
		},
		{
			name: "authz resource only",
			route: codegen.CustomRoute{
				Authz: &codegen.CustomRouteAuthz{Resource: "foos"},
			},
			want: spec.Extensions{routes.ExtensionAuthzResource: "foos"},
		},
		{
			name: "full authz",
			route: codegen.CustomRoute{
				Authz: &codegen.CustomRouteAuthz{
					Resource:    "foos",
					Subresource: new("reconcile"),
					Verb:        new("create"),
				},
			},
			want: spec.Extensions{
				routes.ExtensionAuthzResource:    "foos",
				routes.ExtensionAuthzSubresource: "reconcile",
				routes.ExtensionAuthzVerb:        "create",
			},
		},
		{
			name: "authz alongside extensions",
			route: codegen.CustomRoute{
				Extensions: map[string]any{"x-foo": true},
				Authz: &codegen.CustomRouteAuthz{
					Resource: "foos",
					Verb:     new("get"),
				},
			},
			want: spec.Extensions{
				"x-foo":                       true,
				routes.ExtensionAuthzResource: "foos",
				routes.ExtensionAuthzVerb:     "get",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, customRouteExtensions(test.route))
		})
	}
}

func TestProcessKindVersion_ListKeys(t *testing.T) {
	for _, tc := range []struct {
		name        string
		value, want *bool
	}{
		{name: "omitted"},
		{name: "enabled", value: new(true)},
		{name: "disabled", value: new(false), want: new(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, err := processKindVersion(codegen.VersionedKind{
				Kind: "Foo", Scope: "Namespaced", ListKeys: tc.value,
			}, "v1", false)
			require.NoError(t, err)
			if tc.want == nil {
				assert.Nil(t, kind.Storage)
			} else {
				require.NotNil(t, kind.Storage)
				assert.Equal(t, tc.want, kind.Storage.ListKeys)
			}
		})
	}
}

func TestCustomRouteResponseToSpec3Responses_EmptyResponse(t *testing.T) {
	tests := []struct {
		name         string
		metadata     codegen.CustomRouteResponseMetadata
		wantProps    []string
		wantRequired []string
	}{
		{
			name:     "no metadata",
			metadata: codegen.CustomRouteResponseMetadata{},
		},
		{
			name:         "typeMeta",
			metadata:     codegen.CustomRouteResponseMetadata{TypeMeta: true},
			wantProps:    []string{keyAPIVersion, keyKind},
			wantRequired: []string{keyAPIVersion, keyKind},
		},
		{
			name:         "typeMeta and objectMeta",
			metadata:     codegen.CustomRouteResponseMetadata{TypeMeta: true, ObjectMeta: true},
			wantProps:    []string{keyAPIVersion, keyKind, keyMetadata},
			wantRequired: []string{keyAPIVersion, keyKind, keyMetadata},
		},
		{
			name:         "typeMeta and listMeta",
			metadata:     codegen.CustomRouteResponseMetadata{TypeMeta: true, ListMeta: true},
			wantProps:    []string{keyAPIVersion, keyKind, keyMetadata},
			wantRequired: []string{keyAPIVersion, keyKind, keyMetadata},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(`{}`)
			responses, _, err := customRouteResponseToSpec3Responses(v, tt.metadata, "test")
			require.NoError(t, err)
			require.NotNil(t, responses.Default)
			schema := responses.Default.Content["application/json"].Schema
			props := make([]string, 0, len(schema.Properties))
			for k := range schema.Properties {
				props = append(props, k)
			}
			assert.ElementsMatch(t, tt.wantProps, props)
			assert.ElementsMatch(t, tt.wantRequired, schema.Required)
		})
	}
}

func TestBuildPathPropsFromMethods_Parameters(t *testing.T) {
	ctx := cuecontext.New()
	schema := func(src string) cue.Value { return ctx.CompileString(src) }
	get := func(t *testing.T, path string, request codegen.CustomRouteRequest) ([]*spec3.Parameter, error) {
		t.Helper()
		props, _, err := buildPathPropsFromMethods(path, map[string]codegen.CustomRoute{"GET": {Name: "getThing", Request: request}})
		if err != nil {
			return nil, err
		}
		return props.Get.Parameters, nil
	}
	byName := func(params []*spec3.Parameter) map[string]*spec3.Parameter {
		out := map[string]*spec3.Parameter{}
		for _, p := range params {
			out[p.Name] = p
		}
		return out
	}

	t.Run("a path parameter is a required string unless it has a schema", func(t *testing.T) {
		params, err := get(t, "/things/{kind}/{id}/{group}", codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{
			{Name: "kind", In: "path", Description: "Kind of the thing", Example: "Dashboard"},
			{Name: "group", In: "path", Description: "API group of the thing"},
			{Name: "id", In: "path", Schema: schema("int")},
		}})
		require.NoError(t, err)
		require.Len(t, params, 3)
		assert.Equal(t, []string{"group", "id", "kind"}, []string{params[0].Name, params[1].Name, params[2].Name}, "in name order")
		got := byName(params)
		for _, p := range params {
			assert.Equal(t, "path", p.In)
			assert.True(t, p.Required)
		}
		assert.Equal(t, spec.StringProperty(), got["group"].Schema)
		assert.Equal(t, spec.StringOrArray{"integer"}, got["id"].Schema.Type)
		assert.Equal(t, "Dashboard", got["kind"].Example)
		assert.Nil(t, got["group"].Example, "an example is only emitted when declared")
	})

	t.Run("a parameter that matches the rest of the path is found by its name", func(t *testing.T) {
		for _, path := range []string{"/files/{path}", "/files/{path...}", "/files/{path:*}"} {
			params, err := get(t, path, codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{{Name: "path", In: "path"}}})
			require.NoError(t, err, path)
			require.Len(t, params, 1, path)
		}
	})

	t.Run("a path parameter must be in the path, and cannot be optional", func(t *testing.T) {
		_, err := get(t, "/things/{id}", codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{{Name: "idd", In: "path"}}})
		require.ErrorContains(t, err, `path parameter "idd" is not a parameter in the path "/things/{id}"`)
		_, err = get(t, "/things/{id}", codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{{Name: "id", In: "path", Required: new(false)}}})
		require.ErrorContains(t, err, `path parameter "id" cannot be optional`)
	})

	t.Run("query parameters are defined by their entries", func(t *testing.T) {
		params, err := get(t, "/search", codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{
			{Name: "q", In: "query", Required: new(true), Description: "Terms to look for", Example: "dashboards"},
			{Name: "limit", In: "query", Schema: schema("int | *10"), Example: int64(25), Deprecated: true},
			{Name: "force", In: "query", Schema: schema("bool")},
		}})
		require.NoError(t, err)
		got := byName(params)
		require.Len(t, params, 3)
		assert.Equal(t, "query", got["q"].In)
		assert.True(t, got["q"].Required)
		assert.Equal(t, spec.StringProperty(), got["q"].Schema, "a string unless it has a schema")
		assert.Equal(t, "Terms to look for", got["q"].Description)
		assert.Equal(t, "dashboards", got["q"].Example)
		assert.False(t, got["limit"].Required, "a query parameter is optional unless it says so")
		assert.Equal(t, spec.StringOrArray{"integer"}, got["limit"].Schema.Type)
		assert.EqualValues(t, 10, got["limit"].Schema.Default, "the default comes from the schema")
		assert.Equal(t, int64(25), got["limit"].Example)
		assert.True(t, got["limit"].Deprecated)
		assert.Equal(t, spec.StringOrArray{"boolean"}, got["force"].Schema.Type)
	})

	t.Run("the deprecated query still defines query parameters, from its doc comments", func(t *testing.T) {
		params, err := get(t, "/search", codegen.CustomRouteRequest{Query: schema(`{
			// The search terms.
			q: string
			limit?: int
		}`)})
		require.NoError(t, err)
		got := byName(params)
		assert.Equal(t, "The search terms.", got["q"].Description)
		assert.True(t, got["q"].Required)
		assert.False(t, got["limit"].Required)
	})

	t.Run("a route declares query parameters in one place", func(t *testing.T) {
		_, err := get(t, "/search", codegen.CustomRouteRequest{
			Query:      schema(`{q: string}`),
			Parameters: []codegen.CustomRouteParam{{Name: "limit", In: "query"}},
		})
		require.ErrorContains(t, err, "declares query parameters in parameters and in the deprecated query")
	})

	t.Run("header and cookie parameters are emitted, after the path and query ones", func(t *testing.T) {
		params, err := get(t, "/things/{id}", codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{
			{Name: "session", In: "cookie", Required: new(true)},
			{Name: "X-Request-Id", In: "header", Description: "Correlates the request in logs", Example: "7f0c"},
			{Name: "page", In: "query"},
			{Name: "id", In: "path"},
		}})
		require.NoError(t, err)
		require.Len(t, params, 4)
		assert.Equal(t, []string{"path", "query", "header", "cookie"}, []string{params[0].In, params[1].In, params[2].In, params[3].In})
		got := byName(params)
		assert.False(t, got["X-Request-Id"].Required, "optional unless it says so")
		assert.True(t, got["session"].Required)
	})

	t.Run("a parameter is identified by its name and where it is", func(t *testing.T) {
		_, err := get(t, "/things/{id}", codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{
			{Name: "id", In: "path"}, {Name: "id", In: "query"}, {Name: "id", In: "header"},
		}})
		require.NoError(t, err, "the same name in other places is another parameter")
		_, err = get(t, "/things/{id}", codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{
			{Name: "id", In: "query"}, {Name: "id", In: "query"},
		}})
		require.ErrorContains(t, err, `query parameter "id" is declared twice`)
	})

	t.Run("an example has to satisfy the schema", func(t *testing.T) {
		_, err := get(t, "/search", codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{
			{Name: "limit", In: "query", Schema: schema("int"), Example: "many"},
		}})
		require.ErrorContains(t, err, `example many of query parameter "limit" does not satisfy its schema`)
		_, err = get(t, "/search", codegen.CustomRouteRequest{Parameters: []codegen.CustomRouteParam{
			{Name: "q", In: "query", Example: int64(3)},
		}})
		require.ErrorContains(t, err, `example 3 of query parameter "q" does not satisfy its schema`, "a parameter is a string without a schema")
	})
}

func TestBuildPathPropsFromMethods_OperationID(t *testing.T) {
	for _, tc := range []struct {
		name  string
		route codegen.CustomRoute
		want  string
	}{
		{name: "the route's name by default", route: codegen.CustomRoute{Name: "createStar"}, want: "createStar"},
		{name: "an override", route: codegen.CustomRoute{Name: "createStar", OperationID: "addStar"}, want: "addStar"},
		{name: "a default when there is no name", route: codegen.CustomRoute{}, want: defaultRouteName("PUT", "/star")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			props, _, err := buildPathPropsFromMethods("/star", map[string]codegen.CustomRoute{"PUT": tc.route})
			require.NoError(t, err)
			assert.Equal(t, tc.want, props.Put.OperationId)
		})
	}
}

func TestProcessKindVersion_FixedNames(t *testing.T) {
	for _, fixed := range [][]string{nil, {"current"}, {"a", "b"}} {
		kind, err := processKindVersion(codegen.VersionedKind{
			Kind: "Foo", Scope: "Namespaced", FixedNames: fixed,
		}, "v1", false)
		require.NoError(t, err)
		assert.Equal(t, fixed, kind.FixedNames)
	}
}
