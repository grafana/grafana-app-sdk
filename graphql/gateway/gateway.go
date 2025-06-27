package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/graphql-go/graphql"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/grafana/grafana-app-sdk/graphql/subgraph"
	"github.com/grafana/grafana-app-sdk/logging"
)

// FederatedGateway manages multiple GraphQL subgraphs and composes them into a unified schema
type FederatedGateway struct {
	subgraphs      map[string]subgraph.GraphQLSubgraph
	composedSchema *graphql.Schema
	logger         logging.Logger
	// TODO: Add Mesh Compose and Hive Gateway clients when available
	// meshClient  *MeshComposeClient
	// hiveClient  *HiveGatewayClient
}

// GatewayConfig holds configuration for the federated gateway
type GatewayConfig struct {
	Logger logging.Logger
}

// NewFederatedGateway creates a new federated GraphQL gateway
func NewFederatedGateway(config GatewayConfig) *FederatedGateway {
	return &FederatedGateway{
		subgraphs: make(map[string]subgraph.GraphQLSubgraph),
		logger:    config.Logger,
	}
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
				// Prefix field names with group version to avoid conflicts
				prefix := g.createFieldPrefix(sg.GetGroupVersion())
				prefixedName := prefix + fieldName

				if _, exists := queryFields[prefixedName]; exists {
					return nil, fmt.Errorf("query field conflict: %s", prefixedName)
				}

				// Convert FieldDefinition to Field
				// For now, create a simple field without args to avoid type conversion issues
				queryFields[prefixedName] = &graphql.Field{
					Type:        fieldDef.Type,
					Resolve:     fieldDef.Resolve,
					Description: fieldDef.Description,
				}
			}
		}

		// Extract mutation fields
		if mutationType := schema.MutationType(); mutationType != nil {
			for fieldName, fieldDef := range mutationType.Fields() {
				prefix := g.createFieldPrefix(sg.GetGroupVersion())
				prefixedName := prefix + fieldName

				if _, exists := mutationFields[prefixedName]; exists {
					return nil, fmt.Errorf("mutation field conflict: %s", prefixedName)
				}

				// Convert FieldDefinition to Field
				// For now, create a simple field without args to avoid type conversion issues
				mutationFields[prefixedName] = &graphql.Field{
					Type:        fieldDef.Type,
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

// createFieldPrefix creates a field name prefix based on group version
func (g *FederatedGateway) createFieldPrefix(gv schema.GroupVersion) string {
	// Convert group.version to prefix like "playlist_"
	if gv.Group == "" {
		return ""
	}

	// Take the first part of the group (before any dots)
	parts := strings.Split(gv.Group, ".")
	if len(parts) > 0 {
		return strings.ToLower(parts[0]) + "_"
	}

	return ""
}

// HandleGraphQL handles HTTP GraphQL requests to the composed schema
func (g *FederatedGateway) HandleGraphQL(w http.ResponseWriter, r *http.Request) {
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
