package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeClusterName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"full module path", "github.com/grafana/app", "grafana-app"},
		{"two segments", "grafana/app", "grafana-app"},
		{"single segment", "myapp", "myapp"},
		{"uppercase", "github.com/Grafana/App", "grafana-app"},
		{"special characters", "github.com/my_org/my.app", "my-org-my-app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeClusterName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeClusterName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGrafanaImageVersionAtLeast(t *testing.T) {
	tests := []struct {
		image    string
		major    int
		minor    int
		patch    int
		expected bool
	}{{
		image:    "notvalid",
		major:    12,
		minor:    0,
		patch:    0,
		expected: true,
	}, {
		image:    "grafana:latest",
		major:    12,
		minor:    0,
		patch:    0,
		expected: true,
	}, {
		image:    "grafana:12.0.0",
		major:    12,
		minor:    0,
		patch:    0,
		expected: true,
	}, {
		image:    "grafana:12.0.1",
		major:    12,
		minor:    0,
		patch:    0,
		expected: true,
	}, {
		image:    "grafana:12.1.0",
		major:    12,
		minor:    0,
		patch:    0,
		expected: true,
	}, {
		image:    "grafana:13.0.0",
		major:    12,
		minor:    0,
		patch:    0,
		expected: true,
	}, {
		image:    "grafana:11.9.9",
		major:    12,
		minor:    0,
		patch:    0,
		expected: false,
	}}

	for _, test := range tests {
		t.Run(fmt.Sprintf("'%s' >= %d.%d.%d (%v)", test.image, test.major, test.minor, test.patch, test.expected), func(t *testing.T) {
			assert.Equal(t, test.expected, grafanaImageVersionAtLeast(test.image, test.major, test.minor, test.patch))
		})
	}
}

func TestGenerateK3dConfig_KubePort(t *testing.T) {
	tests := []struct {
		name     string
		kubePort int
		expected string
	}{
		{name: "unset falls back to default", kubePort: 0, expected: "8556"},
		{name: "custom port", kubePort: 8557, expected: "8557"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := generateK3dConfig("/tmp/project", localEnvConfig{Port: 9999, KubePort: tc.kubePort})
			require.NoError(t, err)

			var cfg struct {
				KubeAPI struct {
					HostPort string `json:"hostPort"`
				} `json:"kubeAPI"`
			}
			require.NoError(t, json.Unmarshal(out, &cfg), "generated k3d config must be valid JSON")
			assert.Equal(t, tc.expected, cfg.KubeAPI.HostPort)
		})
	}
}
