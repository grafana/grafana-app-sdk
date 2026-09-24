package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/k8s"
)

func TestBuildClientGenerator_TokenExchange(t *testing.T) {
	t.Setenv("API_ACCESS_TOKEN_EXCHANGE_URL", "https://example.com/exchange")

	generator := BuildClientGenerator(rest.Config{Host: "https://example.com"})

	_, ok := generator.(*k8s.ClientRegistry)
	assert.True(t, ok, "expected a plain *k8s.ClientRegistry when token exchange is configured")
}

func TestBuildClientGenerator_SingleTenant(t *testing.T) {
	t.Setenv("API_ACCESS_TOKEN_EXCHANGE_URL", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/frontend/settings", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"namespace": "stack-123"})
	}))
	defer server.Close()

	generator := BuildClientGenerator(rest.Config{Host: server.URL})

	pinning, ok := generator.(*namespacePinningClientGenerator)
	require.True(t, ok, "expected a *namespacePinningClientGenerator when token exchange is not configured")

	ns, err := pinning.resolver.resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "stack-123", ns)
}
