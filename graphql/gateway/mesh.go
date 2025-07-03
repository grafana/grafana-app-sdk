package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	codecgen "github.com/grafana/grafana-app-sdk/graphql/codegen"
	"github.com/grafana/grafana-app-sdk/graphql/subgraph"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/graphql-go/graphql"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// MeshStyleGateway implements Mesh-style GraphQL schema composition with CUE-based relationships
type MeshStyleGateway struct {
	subgraphs       map[string]CUEAwareSubgraph
	composedSchema  *graphql.Schema
	relationships   []codecgen.MeshRelationshipConfig
	logger          logging.Logger
	rateLimiter     RateLimiter
	rateLimitConfig RateLimitConfig
}

// CUEAwareSubgraph extends the basic subgraph interface with CUE relationship support
type CUEAwareSubgraph interface {
	subgraph.GraphQLSubgraph
	GetRelationships() []codecgen.MeshRelationshipConfig
}

// MeshGatewayConfig holds configuration for the Mesh-style gateway
type MeshGatewayConfig struct {
	Logger    logging.Logger
	RateLimit RateLimitConfig
}

// NewMeshStyleGateway creates a new Mesh-style GraphQL gateway
func NewMeshStyleGateway(config MeshGatewayConfig) *MeshStyleGateway {
	var rateLimiter RateLimiter
	if config.RateLimit.Enabled {
		rateLimiter = NewTokenBucketLimiter(
			config.RateLimit.RequestsPerSecond,
			config.RateLimit.BurstSize,
			config.RateLimit.CleanupInterval,
		)
	}

	return &MeshStyleGateway{
		subgraphs:       make(map[string]CUEAwareSubgraph),
		logger:          config.Logger,
		rateLimiter:     rateLimiter,
		rateLimitConfig: config.RateLimit,
	}
}

// RegisterSubgraph registers a CUE-aware subgraph with the gateway
func (g *MeshStyleGateway) RegisterSubgraph(gv schema.GroupVersion, sg CUEAwareSubgraph) error {
	key := fmt.Sprintf("%s/%s", gv.Group, gv.Version)
	g.subgraphs[key] = sg

	if g.logger != nil {
		g.logger.Info("Registered Mesh-style subgraph", "group", gv.Group, "version", gv.Version)
	}

	return nil
}

// ParseAndConfigureRelationships extracts relationships from all subgraphs and configures the gateway
func (g *MeshStyleGateway) ParseAndConfigureRelationships() error {
	var allRelationships []codecgen.MeshRelationshipConfig

	// Collect explicitly defined relationships from subgraphs
	for _, sg := range g.subgraphs {
		allRelationships = append(allRelationships, sg.GetRelationships()...)
	}

	// TODO: In Phase 3, add CUE relationship parsing here
	// For now, we work with explicitly defined relationships only
	g.relationships = allRelationships

	if g.logger != nil {
		g.logger.Info("Configured relationships", "count", len(allRelationships))
	}

	return nil
}

// ComposeSchema creates a unified GraphQL schema with relationship fields
func (g *MeshStyleGateway) ComposeSchema() (*graphql.Schema, error) {
	// First parse and configure relationships
	if err := g.ParseAndConfigureRelationships(); err != nil {
		return nil, fmt.Errorf("failed to parse relationships: %w", err)
	}

	// Generate base schema from subgraphs
	baseSchema, err := g.generateBaseSchema()
	if err != nil {
		return nil, fmt.Errorf("failed to generate base schema: %w", err)
	}

	// Add relationship fields to schema
	schemaWithRelationships, err := g.addRelationshipFields(baseSchema, g.relationships)
	if err != nil {
		return nil, fmt.Errorf("failed to add relationship fields: %w", err)
	}

	g.composedSchema = schemaWithRelationships

	if g.logger != nil {
		g.logger.Info("Composed Mesh-style schema", "subgraphs", len(g.subgraphs), "relationships", len(g.relationships))
	}

	return g.composedSchema, nil
}

// generateBaseSchema creates the base schema by merging all subgraph schemas
func (g *MeshStyleGateway) generateBaseSchema() (*graphql.Schema, error) {
	queryFields := make(graphql.Fields)
	mutationFields := make(graphql.Fields)

	// Merge fields from all subgraphs
	for key, sg := range g.subgraphs {
		schema := sg.GetSchema()
		if schema == nil {
			continue
		}

		// Add query fields with prefixing to avoid conflicts
		if schema.QueryType() != nil {
			for name, fieldDef := range schema.QueryType().Fields() {
				prefixedName := g.prefixFieldName(key, name)

				// Convert FieldDefinition to Field
				args := make(graphql.FieldConfigArgument)
				for _, arg := range fieldDef.Args {
					args[arg.PrivateName] = &graphql.ArgumentConfig{
						Type:         arg.Type,
						DefaultValue: arg.DefaultValue,
						Description:  arg.PrivateDescription,
					}
				}

				queryFields[prefixedName] = &graphql.Field{
					Type:        fieldDef.Type,
					Args:        args,
					Resolve:     fieldDef.Resolve,
					Description: fieldDef.Description,
				}
			}
		}

		// Add mutation fields with prefixing
		if schema.MutationType() != nil {
			for name, fieldDef := range schema.MutationType().Fields() {
				prefixedName := g.prefixFieldName(key, name)

				// Convert FieldDefinition to Field
				args := make(graphql.FieldConfigArgument)
				for _, arg := range fieldDef.Args {
					args[arg.PrivateName] = &graphql.ArgumentConfig{
						Type:         arg.Type,
						DefaultValue: arg.DefaultValue,
						Description:  arg.PrivateDescription,
					}
				}

				mutationFields[prefixedName] = &graphql.Field{
					Type:        fieldDef.Type,
					Args:        args,
					Resolve:     fieldDef.Resolve,
					Description: fieldDef.Description,
				}
			}
		}
	}

	// Create schema config
	config := graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: queryFields,
		}),
	}

	// Add mutations if any exist
	if len(mutationFields) > 0 {
		config.Mutation = graphql.NewObject(graphql.ObjectConfig{
			Name:   "Mutation",
			Fields: mutationFields,
		})
	}

	schema, err := graphql.NewSchema(config)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// addRelationshipFields adds relationship fields to the schema based on relationship configs
func (g *MeshStyleGateway) addRelationshipFields(baseSchema *graphql.Schema, relationships []codecgen.MeshRelationshipConfig) (*graphql.Schema, error) {
	// For now, return the base schema
	// In a full implementation, this would:
	// 1. Find the source types in the schema
	// 2. Add relationship fields to those types
	// 3. Create resolvers for the relationship fields
	// 4. Return the enhanced schema

	// This is a placeholder - the full implementation would be quite complex
	// and would require deep schema manipulation
	return baseSchema, nil
}

// prefixFieldName creates a prefixed field name to avoid conflicts between subgraphs
func (g *MeshStyleGateway) prefixFieldName(subgraphKey, fieldName string) string {
	// Convert "group/version" to "group_version_fieldName"
	prefix := strings.ReplaceAll(subgraphKey, "/", "_")
	prefix = strings.ReplaceAll(prefix, ".", "_")
	return fmt.Sprintf("%s_%s", prefix, fieldName)
}

// GetSubgraphs returns all registered subgraphs
func (g *MeshStyleGateway) GetSubgraphs() map[string]CUEAwareSubgraph {
	return g.subgraphs
}

// GetComposedSchema returns the composed schema
func (g *MeshStyleGateway) GetComposedSchema() *graphql.Schema {
	return g.composedSchema
}

// GetRelationships returns all parsed relationships
func (g *MeshStyleGateway) GetRelationships() []codecgen.MeshRelationshipConfig {
	return g.relationships
}

// HandleGraphQL processes GraphQL requests
func (g *MeshStyleGateway) HandleGraphQL(w http.ResponseWriter, r *http.Request) {
	// Rate limiting check
	if g.rateLimiter != nil && g.rateLimitConfig.Enabled {
		key := g.rateLimitConfig.KeyExtractor(r)
		if !g.rateLimiter.Allow(key) {
			g.writeRateLimitErrorResponse(w)
			return
		}
	}

	// For now, return a simple response indicating Mesh-style gateway is active
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"data": {"message": "Mesh-style gateway active"}}`))
}

// Helper methods for HTTP responses
func (g *MeshStyleGateway) writeRateLimitErrorResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Exceeded", "true")
	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"errors": []map[string]interface{}{
			{
				"message": "Rate limit exceeded. Please try again later.",
				"extensions": map[string]interface{}{
					"code": "RATE_LIMITED",
				},
			},
		},
	}

	json.NewEncoder(w).Encode(response)
}
