package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-app-sdk/graphql/subgraph"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/graphql-go/graphql"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestTokenBucketLimiter(t *testing.T) {
	// Create a limiter with 2 requests per second and burst of 3
	limiter := NewTokenBucketLimiter(2.0, 3, time.Minute)

	key := "test-key"

	// Should allow initial burst
	for i := 0; i < 3; i++ {
		if !limiter.Allow(key) {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// Should reject the next request (burst exhausted)
	if limiter.Allow(key) {
		t.Error("Expected request to be rejected after burst exhausted")
	}

	// Wait for tokens to replenish (need 0.5 seconds for 1 token at 2/sec rate)
	time.Sleep(600 * time.Millisecond)

	// Should allow one more request
	if !limiter.Allow(key) {
		t.Error("Expected request to be allowed after tokens replenished")
	}
}

func TestRateLimitedGateway(t *testing.T) {
	logger := &logging.NoOpLogger{}

	// Create gateway with strict rate limiting
	config := GatewayConfig{
		Logger: logger,
		RateLimit: RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 1.0, // Very strict: 1 request per second
			BurstSize:         2,   // Allow 2 requests in burst
			KeyExtractor:      DefaultKeyExtractor,
			CleanupInterval:   time.Minute,
		},
	}

	gw := NewFederatedGateway(config)

	// Create a test request
	requestBody := map[string]interface{}{
		"query": "{ __typename }",
	}
	bodyBytes, _ := json.Marshal(requestBody)

	// Test that initial requests are allowed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.100:12345"

		w := httptest.NewRecorder()
		gw.HandleGraphQL(w, req)

		// We expect schema composition to fail (no subgraphs), but rate limiting should pass
		if w.Code == http.StatusTooManyRequests {
			t.Errorf("Request %d should not be rate limited", i+1)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.100:12345"

	w := httptest.NewRecorder()
	gw.HandleGraphQL(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected rate limit error, got status %d", w.Code)
	}

	// Check response format
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Errorf("Failed to decode rate limit response: %v", err)
	}

	errors, ok := response["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Error("Expected errors in rate limit response")
	}

	// Check that Retry-After header is set
	if retryAfter := w.Header().Get("Retry-After"); retryAfter == "" {
		t.Error("Expected Retry-After header in rate limit response")
	}
}

func TestKeyExtractors(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		remoteAddr  string
		extractor   KeyExtractorFunc
		expectedKey string
	}{
		{
			name:        "DefaultKeyExtractor with X-Forwarded-For",
			headers:     map[string]string{"X-Forwarded-For": "203.0.113.1, 70.41.3.18"},
			remoteAddr:  "192.168.1.100:12345",
			extractor:   DefaultKeyExtractor,
			expectedKey: "203.0.113.1",
		},
		{
			name:        "DefaultKeyExtractor with X-Real-IP",
			headers:     map[string]string{"X-Real-IP": "203.0.113.2"},
			remoteAddr:  "192.168.1.100:12345",
			extractor:   DefaultKeyExtractor,
			expectedKey: "203.0.113.2",
		},
		{
			name:        "DefaultKeyExtractor with RemoteAddr",
			headers:     map[string]string{},
			remoteAddr:  "192.168.1.100:12345",
			extractor:   DefaultKeyExtractor,
			expectedKey: "192.168.1.100",
		},
		{
			name:        "UserKeyExtractor with Authorization",
			headers:     map[string]string{"Authorization": "Bearer token123"},
			remoteAddr:  "192.168.1.100:12345",
			extractor:   UserKeyExtractor,
			expectedKey: "user:Bearer token123",
		},
		{
			name:        "UserKeyExtractor fallback to IP",
			headers:     map[string]string{},
			remoteAddr:  "192.168.1.100:12345",
			extractor:   UserKeyExtractor,
			expectedKey: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/graphql", strings.NewReader("{}"))
			req.RemoteAddr = tt.remoteAddr

			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			key := tt.extractor(req)
			if key != tt.expectedKey {
				t.Errorf("Expected key %q, got %q", tt.expectedKey, key)
			}
		})
	}
}

func TestRateLimitReset(t *testing.T) {
	logger := &logging.NoOpLogger{}

	config := GatewayConfig{
		Logger: logger,
		RateLimit: RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 1.0,
			BurstSize:         1, // Only allow 1 request
			KeyExtractor:      DefaultKeyExtractor,
			CleanupInterval:   time.Minute,
		},
	}

	gw := NewFederatedGateway(config)

	requestBody := map[string]interface{}{
		"query": "{ __typename }",
	}
	bodyBytes, _ := json.Marshal(requestBody)

	// First request should be allowed
	req1 := httptest.NewRequest("POST", "/graphql", bytes.NewReader(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")
	req1.RemoteAddr = "192.168.1.100:12345"

	w1 := httptest.NewRecorder()
	gw.HandleGraphQL(w1, req1)

	if w1.Code == http.StatusTooManyRequests {
		t.Error("First request should not be rate limited")
	}

	// Second request should be rate limited
	req2 := httptest.NewRequest("POST", "/graphql", bytes.NewReader(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "192.168.1.100:12345"

	w2 := httptest.NewRecorder()
	gw.HandleGraphQL(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Error("Second request should be rate limited")
	}

	// Reset rate limit for the IP
	gw.ResetRateLimit("192.168.1.100")

	// Third request should now be allowed again
	req3 := httptest.NewRequest("POST", "/graphql", bytes.NewReader(bodyBytes))
	req3.Header.Set("Content-Type", "application/json")
	req3.RemoteAddr = "192.168.1.100:12345"

	w3 := httptest.NewRecorder()
	gw.HandleGraphQL(w3, req3)

	if w3.Code == http.StatusTooManyRequests {
		t.Error("Third request should not be rate limited after reset")
	}
}

func TestGetRateLimitStatus(t *testing.T) {
	logger := &logging.NoOpLogger{}

	// Test with rate limiting disabled
	config1 := GatewayConfig{
		Logger: logger,
		RateLimit: RateLimitConfig{
			Enabled: false,
		},
	}

	gw1 := NewFederatedGateway(config1)
	enabled, _ := gw1.GetRateLimitStatus()
	if enabled {
		t.Error("Rate limiting should be disabled")
	}

	// Test with rate limiting enabled
	config2 := GatewayConfig{
		Logger: logger,
		RateLimit: RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 100,
			BurstSize:         200,
			KeyExtractor:      DefaultKeyExtractor,
		},
	}

	gw2 := NewFederatedGateway(config2)
	enabled, config := gw2.GetRateLimitStatus()
	if !enabled {
		t.Error("Rate limiting should be enabled")
	}

	if config.RequestsPerSecond != 100 {
		t.Errorf("Expected RequestsPerSecond to be 100, got %f", config.RequestsPerSecond)
	}
}

// Mock subgraph for testing
type mockSubgraph struct {
	gv schema.GroupVersion
}

func (m *mockSubgraph) GetGroupVersion() schema.GroupVersion {
	return m.gv
}

func (m *mockSubgraph) GetSchema() *graphql.Schema {
	// Return nil to trigger schema composition error - this is fine for rate limit testing
	return nil
}

func (m *mockSubgraph) GetResolvers() subgraph.ResolverMap {
	return make(subgraph.ResolverMap)
}

func (m *mockSubgraph) GetKinds() []resource.Kind {
	return []resource.Kind{}
}

func TestRateLimitWithSubgraphs(t *testing.T) {
	logger := &logging.NoOpLogger{}

	config := GatewayConfig{
		Logger: logger,
		RateLimit: RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: 10,
			BurstSize:         5,
			KeyExtractor:      DefaultKeyExtractor,
		},
	}

	gw := NewFederatedGateway(config)

	// Register a mock subgraph
	mockSG := &mockSubgraph{
		gv: schema.GroupVersion{Group: "test.grafana.com", Version: "v1"},
	}

	err := gw.RegisterSubgraph(mockSG.GetGroupVersion(), mockSG)
	if err != nil {
		t.Fatalf("Failed to register subgraph: %v", err)
	}

	// Test that rate limiting still works with subgraphs registered
	requestBody := map[string]interface{}{
		"query": "{ __typename }",
	}
	bodyBytes, _ := json.Marshal(requestBody)

	// Make requests up to the burst limit
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.200:12345"

		w := httptest.NewRecorder()
		gw.HandleGraphQL(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Errorf("Request %d should not be rate limited", i+1)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest("POST", "/graphql", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.200:12345"

	w := httptest.NewRecorder()
	gw.HandleGraphQL(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Error("Request should be rate limited after burst exhausted")
	}
}
