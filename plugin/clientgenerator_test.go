package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/resource"
)

func TestBuildClientGenerator_TokenExchange(t *testing.T) {
	t.Setenv("API_ACCESS_TOKEN_EXCHANGE_URL", "https://example.com/exchange")

	generator := BuildClientGenerator(rest.Config{Host: "https://example.com"})

	_, ok := generator.(*k8s.ClientRegistry)
	assert.True(t, ok, "expected a plain *k8s.ClientRegistry when token exchange is configured")
}

func TestBuildClientGenerator_SingleTenant(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		wantPinNS string
	}{
		{name: "namespace from env", envValue: "stack-123", wantPinNS: "stack-123"},
		{name: "default namespace", envValue: "", wantPinNS: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("API_ACCESS_TOKEN_EXCHANGE_URL", "")
			t.Setenv("API_ACCESS_NAMESPACE", tt.envValue)

			generator := BuildClientGenerator(rest.Config{Host: "https://example.com"})

			pinning, ok := generator.(*namespacePinningClientGenerator)
			require.True(t, ok, "expected a *namespacePinningClientGenerator when token exchange is not configured")
			assert.Equal(t, tt.wantPinNS, pinning.namespace)

			client := &namespacePinningClient{namespace: pinning.namespace}
			assert.Equal(t, tt.wantPinNS, client.pin(resource.NamespaceAll))
			assert.Equal(t, "other", client.pin("other"))
		})
	}
}
