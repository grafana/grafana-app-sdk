package main

import "testing"

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
