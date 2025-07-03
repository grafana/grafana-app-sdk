package gateway

import (
	"context"
	"fmt"
	"strings"

	codecgen "github.com/grafana/grafana-app-sdk/graphql/codegen"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/graphql-go/graphql"
)

// RelationshipResolver handles the resolution of cross-service relationships
type RelationshipResolver struct {
	gateway *MeshStyleGateway
	logger  logging.Logger
}

// NewRelationshipResolver creates a new relationship resolver
func NewRelationshipResolver(gateway *MeshStyleGateway, logger logging.Logger) *RelationshipResolver {
	return &RelationshipResolver{
		gateway: gateway,
		logger:  logger,
	}
}

// CreateRelationshipResolver creates a GraphQL field resolver for a specific relationship
func (r *RelationshipResolver) CreateRelationshipResolver(rel codecgen.MeshRelationshipConfig) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		// Extract source data
		source, ok := p.Source.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid source type for relationship %s", rel.SourceField)
		}

		// Check if relationship should resolve based on conditions
		if rel.Transform != nil {
			transformedArgs := rel.Transform(source)
			if transformedArgs == nil {
				// Condition not met, return nil
				return nil, nil
			}
		}

		// Extract required selection values
		targetArgs := make(map[string]interface{})
		for key, template := range rel.TargetArguments {
			value, err := r.resolveArgumentTemplate(template, source)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve argument %s: %w", key, err)
			}
			targetArgs[key] = value
		}

		// Find the target subgraph
		targetSubgraph, err := r.findSubgraphForService(rel.TargetService)
		if err != nil {
			return nil, fmt.Errorf("failed to find subgraph for service %s: %w", rel.TargetService, err)
		}

		// Execute the target query
		result, err := r.executeSubgraphQuery(p.Context, targetSubgraph, rel.TargetQuery, targetArgs)
		if err != nil {
			if r.logger != nil {
				r.logger.Error("Failed to resolve relationship", "error", err, "relationship", rel.SourceField)
			}
			// Return nil instead of error for optional relationships
			return nil, nil
		}

		return result, nil
	}
}

// resolveArgumentTemplate resolves template strings in target arguments
// Templates can reference source fields using {source.fieldName} syntax
func (r *RelationshipResolver) resolveArgumentTemplate(template string, source map[string]interface{}) (interface{}, error) {
	// Simple template resolution - look for {source.fieldName} patterns
	if strings.HasPrefix(template, "{source.") && strings.HasSuffix(template, "}") {
		// Extract field path from {source.fieldName}
		fieldPath := template[8 : len(template)-1] // Remove {source. and }

		// Navigate the source object to find the field value
		return r.extractFieldValue(source, fieldPath)
	}

	// Return template as-is if it's not a template
	return template, nil
}

// extractFieldValue extracts a field value from a nested object using dot notation
func (r *RelationshipResolver) extractFieldValue(source map[string]interface{}, fieldPath string) (interface{}, error) {
	parts := strings.Split(fieldPath, ".")
	current := source

	for i, part := range parts {
		value, exists := current[part]
		if !exists {
			return nil, fmt.Errorf("field %s not found in source", strings.Join(parts[:i+1], "."))
		}

		if i == len(parts)-1 {
			// Last part, return the value
			return value, nil
		}

		// Navigate deeper into the object
		nextMap, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("field %s is not an object", strings.Join(parts[:i+1], "."))
		}
		current = nextMap
	}

	return nil, fmt.Errorf("unexpected end of field path")
}

// findSubgraphForService finds the subgraph that handles a particular service
func (r *RelationshipResolver) findSubgraphForService(serviceName string) (CUEAwareSubgraph, error) {
	// Look through all registered subgraphs to find one that matches the service
	for key, sg := range r.gateway.GetSubgraphs() {
		// Check if the subgraph handles this service
		// For now, we'll match based on the group name in the key
		if strings.Contains(strings.ToLower(key), strings.ToLower(serviceName)) {
			return sg, nil
		}
	}

	return nil, fmt.Errorf("no subgraph found for service %s", serviceName)
}

// executeSubgraphQuery executes a GraphQL query against a specific subgraph
func (r *RelationshipResolver) executeSubgraphQuery(ctx context.Context, sg CUEAwareSubgraph, query string, args map[string]interface{}) (interface{}, error) {
	schema := sg.GetSchema()
	if schema == nil {
		return nil, fmt.Errorf("subgraph has no schema")
	}

	// Build GraphQL query string
	queryString, err := r.buildQueryString(query, args)
	if err != nil {
		return nil, fmt.Errorf("failed to build query string: %w", err)
	}

	// Execute the query
	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: queryString,
		Context:       ctx,
	})

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL query failed: %v", result.Errors)
	}

	return result.Data, nil
}

// buildQueryString builds a GraphQL query string from a query name and arguments
func (r *RelationshipResolver) buildQueryString(queryName string, args map[string]interface{}) (string, error) {
	// Simple query builder for basic queries
	// In a full implementation, this would be more sophisticated

	if len(args) == 0 {
		return fmt.Sprintf("{ %s }", queryName), nil
	}

	// Build arguments string
	var argParts []string
	for key, value := range args {
		switch v := value.(type) {
		case string:
			argParts = append(argParts, fmt.Sprintf(`%s: "%s"`, key, v))
		case int, int64, float64:
			argParts = append(argParts, fmt.Sprintf(`%s: %v`, key, v))
		default:
			argParts = append(argParts, fmt.Sprintf(`%s: "%v"`, key, v))
		}
	}

	argsString := strings.Join(argParts, ", ")
	return fmt.Sprintf("{ %s(%s) }", queryName, argsString), nil
}

// RelationshipFieldAdder adds relationship fields to GraphQL types
type RelationshipFieldAdder struct {
	resolver *RelationshipResolver
	logger   logging.Logger
}

// NewRelationshipFieldAdder creates a new relationship field adder
func NewRelationshipFieldAdder(resolver *RelationshipResolver, logger logging.Logger) *RelationshipFieldAdder {
	return &RelationshipFieldAdder{
		resolver: resolver,
		logger:   logger,
	}
}

// AddRelationshipFields adds relationship fields to a GraphQL schema
func (a *RelationshipFieldAdder) AddRelationshipFields(schema *graphql.Schema, relationships []codecgen.MeshRelationshipConfig) (*graphql.Schema, error) {
	// This is a placeholder for the complex schema manipulation required
	// In a full implementation, this would:
	// 1. Find the source types in the schema
	// 2. Add relationship fields to those types
	// 3. Create resolvers for the relationship fields
	// 4. Return the enhanced schema

	// For Phase 2, we return the schema unchanged
	// The relationship resolution logic is ready, but schema manipulation is complex
	if a.logger != nil {
		a.logger.Info("Relationship fields ready to be added", "count", len(relationships))
	}

	return schema, nil
}
