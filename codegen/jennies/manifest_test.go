package jennies

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

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
