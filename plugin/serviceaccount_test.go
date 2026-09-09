package plugin

import (
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/config"
)

func TestServiceAccountKubeConfig(t *testing.T) {
	t.Run("returns nil when app URL is missing", func(t *testing.T) {
		t.Setenv(config.AppClientSecret, "secret")

		if cfg := serviceAccountKubeConfig(); cfg != nil {
			t.Fatalf("expected nil, got config %+v", cfg)
		}
	})

	t.Run("returns nil when client secret is missing", func(t *testing.T) {
		t.Setenv(config.AppURL, "https://grafana.example.com")

		if cfg := serviceAccountKubeConfig(); cfg != nil {
			t.Fatalf("expected nil, got config %+v", cfg)
		}
	})

	t.Run("builds a rest.Config from the service account env vars", func(t *testing.T) {
		t.Setenv(config.AppURL, "https://grafana.example.com")
		t.Setenv(config.AppClientSecret, "secret")

		cfg := serviceAccountKubeConfig()
		if cfg == nil {
			t.Fatal("expected a non-nil config")
		}
		if cfg.Host != "https://grafana.example.com" {
			t.Errorf("expected Host %q, got %q", "https://grafana.example.com", cfg.Host)
		}
		if cfg.APIPath != "/apis" {
			t.Errorf("expected APIPath %q, got %q", "/apis", cfg.APIPath)
		}
		if cfg.BearerToken != "secret" {
			t.Errorf("expected BearerToken %q, got %q", "secret", cfg.BearerToken)
		}
	})
}
