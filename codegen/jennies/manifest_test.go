package jennies

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
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
