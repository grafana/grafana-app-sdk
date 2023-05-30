package kubeconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/plugin/kubeconfig"
)

func TestCachingInitializer(t *testing.T) {
	t.Run("should only call initialiser once with same config", func(t *testing.T) {
		var numCalls int

		ini := kubeconfig.CachingInitializer(func(cfg kubeconfig.NamespacedConfig) (string, error) {
			numCalls++
			return "", nil
		})

		cfg := kubeconfig.NamespacedConfig{
			CRC32:     123,
			Namespace: "default",
			RestConfig: rest.Config{
				Host: "https://some.url:443",
			},
		}

		_, _ = ini(cfg)
		_, _ = ini(cfg)
		_, _ = ini(cfg)

		assert.Equal(t, 1, numCalls)
	})

	t.Run("should call multiple times if config keeps changing", func(t *testing.T) {
		var numCalls int

		ini := kubeconfig.CachingInitializer(func(cfg kubeconfig.NamespacedConfig) (string, error) {
			numCalls++
			return "", nil
		})

		_, _ = ini(kubeconfig.NamespacedConfig{
			CRC32:     123,
			Namespace: "default",
			RestConfig: rest.Config{
				Host: "https://some.url:443",
			},
		})
		_, _ = ini(kubeconfig.NamespacedConfig{
			CRC32:     213,
			Namespace: "test",
			RestConfig: rest.Config{
				Host: "https://some.url:443",
			},
		})
		_, _ = ini(kubeconfig.NamespacedConfig{
			CRC32:     321,
			Namespace: "rest",
			RestConfig: rest.Config{
				Host: "https://some.url:443",
			},
		})

		assert.Equal(t, 3, numCalls)
	})
}
