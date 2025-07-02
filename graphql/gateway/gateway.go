// Package gateway provides a federated GraphQL gateway with rate limiting capabilities.
//
// Rate Limiting Usage:
//
// Basic IP-based rate limiting:
//
//	config := gateway.GatewayConfig{
//	  Logger: logger,
//	  RateLimit: gateway.RateLimitConfig{
//	    Enabled:           true,
//	    RequestsPerSecond: 100,    // Allow 100 requests per second
//	    BurstSize:         200,    // Allow bursts up to 200 requests
//	    KeyExtractor:      gateway.DefaultKeyExtractor, // Rate limit by IP
//	    CleanupInterval:   5 * time.Minute,
//	  },
//	}
//	gw := gateway.NewFederatedGateway(config)
//
// User-based rate limiting:
//
//	config := gateway.GatewayConfig{
//	  Logger: logger,
//	  RateLimit: gateway.RateLimitConfig{
//	    Enabled:           true,
//	    RequestsPerSecond: 50,
//	    BurstSize:         100,
//	    KeyExtractor:      gateway.UserKeyExtractor, // Rate limit by user
//	    CleanupInterval:   10 * time.Minute,
//	  },
//	}
//
// Custom key extraction:
//
//	customExtractor := func(r *http.Request) string {
//	  // Rate limit by API key
//	  if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
//	    return "api:" + apiKey
//	  }
//	  // Fall back to IP
//	  return gateway.DefaultKeyExtractor(r)
//	}
//	config.RateLimit.KeyExtractor = customExtractor
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/graphql-go/graphql"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/graphql/subgraph"
	"github.com/grafana/grafana-app-sdk/logging"
)

// RateLimiter defines the interface for rate limiting
type RateLimiter interface {
	// Allow returns true if the request should be allowed, false if it should be throttled
	Allow(key string) bool
	// Reset clears all rate limiting state for a given key
	Reset(key string)
}

// TokenBucketLimiter implements a token bucket rate limiter
type TokenBucketLimiter struct {
	rate     float64 // tokens per second
	capacity int     // bucket capacity
	buckets  map[string]*bucket
	mutex    sync.RWMutex
	cleanup  time.Duration // cleanup interval for unused buckets
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
	mutex    sync.Mutex
}

// NewTokenBucketLimiter creates a new token bucket rate limiter
func NewTokenBucketLimiter(rate float64, capacity int, cleanup time.Duration) *TokenBucketLimiter {
	limiter := &TokenBucketLimiter{
		rate:     rate,
		capacity: capacity,
		buckets:  make(map[string]*bucket),
		cleanup:  cleanup,
	}

	// Start cleanup goroutine
	go limiter.cleanupLoop()

	return limiter
}

// Allow checks if a request should be allowed based on the token bucket algorithm
func (t *TokenBucketLimiter) Allow(key string) bool {
	t.mutex.RLock()
	b, exists := t.buckets[key]
	t.mutex.RUnlock()

	if !exists {
		t.mutex.Lock()
		// Double-check after acquiring write lock
		if b, exists = t.buckets[key]; !exists {
			b = &bucket{
				tokens:   float64(t.capacity),
				lastSeen: time.Now(),
			}
			t.buckets[key] = b
		}
		t.mutex.Unlock()
	}

	return t.consumeToken(b)
}

// Reset clears the rate limiting state for a key
func (t *TokenBucketLimiter) Reset(key string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	delete(t.buckets, key)
}

// consumeToken attempts to consume a token from the bucket
func (t *TokenBucketLimiter) consumeToken(b *bucket) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.lastSeen = now

	// Add tokens based on elapsed time
	b.tokens += elapsed * t.rate
	if b.tokens > float64(t.capacity) {
		b.tokens = float64(t.capacity)
	}

	// Try to consume a token
	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}

// cleanupLoop periodically removes unused buckets
func (t *TokenBucketLimiter) cleanupLoop() {
	if t.cleanup <= 0 {
		return
	}

	ticker := time.NewTicker(t.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		t.mutex.Lock()
		cutoff := time.Now().Add(-t.cleanup)
		for key, bucket := range t.buckets {
			bucket.mutex.Lock()
			if bucket.lastSeen.Before(cutoff) {
				delete(t.buckets, key)
			}
			bucket.mutex.Unlock()
		}
		t.mutex.Unlock()
	}
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	// Enabled controls whether rate limiting is active
	Enabled bool
	// RequestsPerSecond defines the rate limit (tokens per second)
	RequestsPerSecond float64
	// BurstSize defines the maximum burst capacity
	BurstSize int
	// KeyExtractor defines how to extract the rate limiting key from requests
	KeyExtractor KeyExtractorFunc
	// CleanupInterval defines how often to clean up unused rate limit buckets
	CleanupInterval time.Duration
}

// KeyExtractorFunc extracts a rate limiting key from an HTTP request
type KeyExtractorFunc func(*http.Request) string

// DefaultKeyExtractor extracts the client IP address as the rate limiting key
func DefaultKeyExtractor(r *http.Request) string {
	// Try X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if ips := strings.Split(xff, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Try X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}

	return r.RemoteAddr
}

// UserKeyExtractor extracts rate limiting key from user context/headers
func UserKeyExtractor(r *http.Request) string {
	// Try Authorization header first
	if auth := r.Header.Get("Authorization"); auth != "" {
		return "user:" + auth
	}

	// Fall back to IP-based limiting
	return DefaultKeyExtractor(r)
}

// FederatedGateway manages multiple GraphQL subgraphs and composes them into a unified schema
type FederatedGateway struct {
	subgraphs       map[string]subgraph.GraphQLSubgraph
	composedSchema  *graphql.Schema
	logger          logging.Logger
	rateLimiter     RateLimiter
	rateLimitConfig RateLimitConfig
	// TODO: Add Mesh Compose and Hive Gateway clients when available
	// meshClient  *MeshComposeClient
	// hiveClient  *HiveGatewayClient
}

// GatewayConfig holds configuration for the federated gateway
type GatewayConfig struct {
	Logger    logging.Logger
	RateLimit RateLimitConfig
}

// NewFederatedGateway creates a new federated GraphQL gateway
func NewFederatedGateway(config GatewayConfig) *FederatedGateway {
	gateway := &FederatedGateway{
		subgraphs:       make(map[string]subgraph.GraphQLSubgraph),
		logger:          config.Logger,
		rateLimitConfig: config.RateLimit,
	}

	// Initialize rate limiter if enabled
	if config.RateLimit.Enabled {
		// Set defaults if not provided
		rps := config.RateLimit.RequestsPerSecond
		if rps <= 0 {
			rps = 100 // Default to 100 requests per second
		}

		burst := config.RateLimit.BurstSize
		if burst <= 0 {
			burst = int(rps * 2) // Default burst to 2x the rate
		}

		cleanup := config.RateLimit.CleanupInterval
		if cleanup <= 0 {
			cleanup = 5 * time.Minute // Default cleanup interval
		}

		keyExtractor := config.RateLimit.KeyExtractor
		if keyExtractor == nil {
			keyExtractor = DefaultKeyExtractor
		}

		gateway.rateLimiter = NewTokenBucketLimiter(rps, burst, cleanup)
		gateway.rateLimitConfig.KeyExtractor = keyExtractor

		gateway.logger.Info("Rate limiting enabled",
			"requestsPerSecond", rps,
			"burstSize", burst,
			"cleanupInterval", cleanup)
	}

	return gateway
}

// RegisterSubgraph registers a new subgraph with the gateway
func (g *FederatedGateway) RegisterSubgraph(gv schema.GroupVersion, sg subgraph.GraphQLSubgraph) error {
	if sg == nil {
		return fmt.Errorf("subgraph cannot be nil")
	}

	key := gv.String()
	if _, exists := g.subgraphs[key]; exists {
		return fmt.Errorf("subgraph for %s already registered", key)
	}

	g.subgraphs[key] = sg
	g.logger.Debug("Registered GraphQL subgraph", "groupVersion", key)

	// Mark composed schema as stale
	g.composedSchema = nil

	return nil
}

// ComposeSchema composes all registered subgraphs into a unified schema
func (g *FederatedGateway) ComposeSchema() (*graphql.Schema, error) {
	if g.composedSchema != nil {
		return g.composedSchema, nil
	}

	if len(g.subgraphs) == 0 {
		return nil, fmt.Errorf("no subgraphs registered")
	}

	// For now, use simple schema merging
	// TODO: Replace with Mesh Compose + Hive Gateway integration
	composedSchema, err := g.mergeSubgraphSchemas()
	if err != nil {
		return nil, fmt.Errorf("failed to compose schemas: %w", err)
	}

	g.composedSchema = composedSchema
	g.logger.Info("Composed GraphQL schema", "subgraphs", len(g.subgraphs))

	return g.composedSchema, nil
}

// mergeSubgraphSchemas performs simple schema merging for multiple subgraphs
// This is a temporary implementation until Mesh Compose + Hive Gateway integration
func (g *FederatedGateway) mergeSubgraphSchemas() (*graphql.Schema, error) {
	queryFields := make(graphql.Fields)
	mutationFields := make(graphql.Fields)

	// Merge fields from all subgraphs
	for key, sg := range g.subgraphs {
		schema := sg.GetSchema()
		if schema == nil {
			return nil, fmt.Errorf("subgraph %s has nil schema", key)
		}

		// Extract query fields
		if queryType := schema.QueryType(); queryType != nil {
			for fieldName, fieldDef := range queryType.Fields() {
				if _, exists := queryFields[fieldName]; exists {
					return nil, fmt.Errorf("query field conflict: %s", fieldName)
				}

				// Convert FieldDefinition to Field
				// Properly convert []*graphql.Argument to graphql.FieldConfigArgument
				args := make(graphql.FieldConfigArgument)
				for _, arg := range fieldDef.Args {
					args[arg.PrivateName] = &graphql.ArgumentConfig{
						Type:         arg.Type,
						DefaultValue: arg.DefaultValue,
						Description:  arg.PrivateDescription,
					}
				}

				queryFields[fieldName] = &graphql.Field{
					Type:        fieldDef.Type,
					Args:        args,
					Resolve:     fieldDef.Resolve,
					Description: fieldDef.Description,
				}
			}
		}

		// Extract mutation fields
		if mutationType := schema.MutationType(); mutationType != nil {
			for fieldName, fieldDef := range mutationType.Fields() {
				if _, exists := mutationFields[fieldName]; exists {
					return nil, fmt.Errorf("mutation field conflict: %s", fieldName)
				}

				// Convert FieldDefinition to Field
				// Properly convert []*graphql.Argument to graphql.FieldConfigArgument
				args := make(graphql.FieldConfigArgument)
				for _, arg := range fieldDef.Args {
					args[arg.PrivateName] = &graphql.ArgumentConfig{
						Type:         arg.Type,
						DefaultValue: arg.DefaultValue,
						Description:  arg.PrivateDescription,
					}
				}

				mutationFields[fieldName] = &graphql.Field{
					Type:        fieldDef.Type,
					Args:        args,
					Resolve:     fieldDef.Resolve,
					Description: fieldDef.Description,
				}
			}
		}
	}

	// Create composed query type
	composedQuery := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Query",
		Fields: queryFields,
	})

	// Create composed mutation type (if we have mutations)
	var composedMutation *graphql.Object
	if len(mutationFields) > 0 {
		composedMutation = graphql.NewObject(graphql.ObjectConfig{
			Name:   "Mutation",
			Fields: mutationFields,
		})
	}

	// Create the composed schema
	schemaConfig := graphql.SchemaConfig{
		Query: composedQuery,
	}
	if composedMutation != nil {
		schemaConfig.Mutation = composedMutation
	}

	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// HandleGraphQL handles HTTP GraphQL requests to the composed schema
func (g *FederatedGateway) HandleGraphQL(w http.ResponseWriter, r *http.Request) {
	// Apply rate limiting if enabled
	if g.rateLimiter != nil && g.rateLimitConfig.KeyExtractor != nil {
		key := g.rateLimitConfig.KeyExtractor(r)
		if !g.rateLimiter.Allow(key) {
			g.logger.Debug("Request rate limited", "key", key)
			g.writeRateLimitErrorResponse(w)
			return
		}
	}

	// Ensure schema is composed
	schema, err := g.ComposeSchema()
	if err != nil {
		g.writeErrorResponse(w, fmt.Sprintf("Failed to compose schema: %v", err))
		return
	}

	// Parse request body
	var requestBody struct {
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables"`
		OperationName string                 `json:"operationName,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		g.writeErrorResponse(w, "Invalid JSON request body")
		return
	}

	// Execute GraphQL query
	result := graphql.Do(graphql.Params{
		Schema:         *schema,
		RequestString:  requestBody.Query,
		VariableValues: requestBody.Variables,
		OperationName:  requestBody.OperationName,
		Context:        r.Context(),
	})

	// Write response
	g.writeGraphQLResponse(w, result)
}

// HandleGraphQLWithContext handles GraphQL requests with a provided context
func (g *FederatedGateway) HandleGraphQLWithContext(ctx context.Context, query string, variables map[string]interface{}) *graphql.Result {
	schema, err := g.ComposeSchema()
	if err != nil {
		// Return a simple error result
		return &graphql.Result{
			Data: nil,
		}
	}

	return graphql.Do(graphql.Params{
		Schema:         *schema,
		RequestString:  query,
		VariableValues: variables,
		Context:        ctx,
	})
}

// GetSubgraphs returns all registered subgraphs
func (g *FederatedGateway) GetSubgraphs() map[string]subgraph.GraphQLSubgraph {
	// Return a copy to prevent external modification
	result := make(map[string]subgraph.GraphQLSubgraph)
	for key, sg := range g.subgraphs {
		result[key] = sg
	}
	return result
}

// GetComposedSchema returns the current composed schema, or nil if not yet composed
func (g *FederatedGateway) GetComposedSchema() *graphql.Schema {
	return g.composedSchema
}

// writeErrorResponse writes an error response in GraphQL format
func (g *FederatedGateway) writeErrorResponse(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	response := map[string]interface{}{
		"errors": []map[string]interface{}{
			{"message": message},
		},
	}

	json.NewEncoder(w).Encode(response)
}

// writeGraphQLResponse writes a GraphQL result as JSON
func (g *FederatedGateway) writeGraphQLResponse(w http.ResponseWriter, result *graphql.Result) {
	w.Header().Set("Content-Type", "application/json")

	if len(result.Errors) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(result)
}

// writeRateLimitErrorResponse writes a rate limit exceeded error response
func (g *FederatedGateway) writeRateLimitErrorResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60") // Suggest retry after 60 seconds
	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"errors": []map[string]interface{}{
			{
				"message": "Rate limit exceeded. Too many requests.",
				"extensions": map[string]interface{}{
					"code":      "RATE_LIMITED",
					"timestamp": time.Now().Unix(),
				},
			},
		},
	}

	json.NewEncoder(w).Encode(response)
}

// ResetRateLimit resets rate limiting for a specific key (useful for testing or administrative purposes)
func (g *FederatedGateway) ResetRateLimit(key string) {
	if g.rateLimiter != nil {
		g.rateLimiter.Reset(key)
		g.logger.Debug("Reset rate limit", "key", key)
	}
}

// GetRateLimitStatus returns whether rate limiting is enabled and its configuration
func (g *FederatedGateway) GetRateLimitStatus() (enabled bool, config RateLimitConfig) {
	return g.rateLimiter != nil, g.rateLimitConfig
}

// TODO: Mesh Compose and Hive Gateway integration
// These will be implemented when those tools are available

/*
// MeshComposeClient will integrate with Mesh Compose for advanced schema composition
type MeshComposeClient struct {
	// Configuration for Mesh Compose
}

// HiveGatewayClient will integrate with Hive Gateway for query planning and execution
type HiveGatewayClient struct {
	// Configuration for Hive Gateway
}

func (m *MeshComposeClient) ComposeSchemas(subgraphs []SubgraphSchema) (*ComposedSchema, error) {
	// Implementation will use Mesh Compose API
	return nil, fmt.Errorf("not implemented")
}

func (h *HiveGatewayClient) ExecuteQuery(schema *ComposedSchema, query string) (*QueryResult, error) {
	// Implementation will use Hive Gateway API
	return nil, fmt.Errorf("not implemented")
}
*/
