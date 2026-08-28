package jennies

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"k8s.io/utils/ptr"

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
			props: codegen.AppManifestProperties{OperatorURL: ptr.To("https://foo.bar:8443")},
			want:  ptr.To("https://foo.bar:8443"),
		},
		{
			name: "structured operator.url only",
			props: codegen.AppManifestProperties{
				Operator: &codegen.AppManifestPropertiesOperatorInfo{URL: ptr.To("https://foo.bar:8443")},
			},
			want: ptr.To("https://foo.bar:8443"),
		},
		{
			name: "both set to same value",
			props: codegen.AppManifestProperties{
				OperatorURL: ptr.To("https://foo.bar:8443"),
				Operator:    &codegen.AppManifestPropertiesOperatorInfo{URL: ptr.To("https://foo.bar:8443")},
			},
			want: ptr.To("https://foo.bar:8443"),
		},
		{
			name: "both set to different values errors",
			props: codegen.AppManifestProperties{
				OperatorURL: ptr.To("https://deprecated:8443"),
				Operator:    &codegen.AppManifestPropertiesOperatorInfo{URL: ptr.To("https://structured:8443")},
			},
			wantErr: "both set but differ",
		},
		{
			name: "operator set without url falls back to deprecated",
			props: codegen.AppManifestProperties{
				OperatorURL: ptr.To("https://foo.bar:8443"),
				Operator:    &codegen.AppManifestPropertiesOperatorInfo{},
			},
			want: ptr.To("https://foo.bar:8443"),
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
				Operator: &codegen.AppManifestPropertiesOperatorInfo{URL: ptr.To("https://foo.bar")},
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
		// Both endpoints are served by default, so nothing is written to the manifest.
		name:     "both enabled",
		search:   codegen.KindSearch{Endpoint: true, Trash: true},
		expected: nil,
	}, {
		name:     "search opt-out",
		search:   codegen.KindSearch{Endpoint: false, Trash: true},
		expected: &app.ManifestVersionKindSearch{Endpoint: ptr.To(false)},
	}, {
		name:     "trash opt-out",
		search:   codegen.KindSearch{Endpoint: true, Trash: false},
		expected: &app.ManifestVersionKindSearch{Trash: ptr.To(false)},
	}, {
		name:     "both opt-out",
		search:   codegen.KindSearch{Endpoint: false, Trash: false},
		expected: &app.ManifestVersionKindSearch{Endpoint: ptr.To(false), Trash: ptr.To(false)},
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
			want: spec.Extensions{"x-grafana-declared-authz-resource": "foos"},
		},
		{
			name: "full authz",
			route: codegen.CustomRoute{
				Authz: &codegen.CustomRouteAuthz{
					Resource:    "foos",
					Subresource: ptr.To("reconcile"),
					Verb:        ptr.To("create"),
				},
			},
			want: spec.Extensions{
				"x-grafana-declared-authz-resource":    "foos",
				"x-grafana-declared-authz-subresource": "reconcile",
				"x-grafana-declared-authz-verb":        "create",
			},
		},
		{
			name: "authz alongside extensions",
			route: codegen.CustomRoute{
				Extensions: map[string]any{"x-foo": true},
				Authz: &codegen.CustomRouteAuthz{
					Resource: "foos",
					Verb:     ptr.To("get"),
				},
			},
			want: spec.Extensions{
				"x-foo":                             true,
				"x-grafana-declared-authz-resource": "foos",
				"x-grafana-declared-authz-verb":     "get",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, customRouteExtensions(test.route))
		})
	}
}
