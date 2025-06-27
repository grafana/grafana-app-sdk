package codegen

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/graphql-go/graphql"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// RelationshipConfig represents a parsed @relation attribute
type RelationshipConfig struct {
	// FieldName is the GraphQL field name for this relationship
	FieldName string

	// Kind is the target kind (e.g., "dashboard.grafana.app/Dashboard")
	Kind string

	// SourceField is the local field path containing reference value
	SourceField string

	// TargetField is the target field path to match against
	TargetField string

	// Optional indicates if the relationship can be null
	Optional bool

	// Cardinality is "one" or "many"
	Cardinality string

	// Match is the matching strategy ("exact", "array_contains", etc.)
	Match string

	// TargetGVK is the parsed group/version/kind
	TargetGVK schema.GroupVersionKind
}

// RelationshipParser parses @relation attributes from CUE values
type RelationshipParser struct {
	cueContext            *cue.Context
	explicitRelationships map[string]map[string]*RelationshipConfig // [kindName][fieldName] -> RelationshipConfig
}

// NewRelationshipParser creates a new relationship parser
func NewRelationshipParser(ctx *cue.Context) *RelationshipParser {
	return &RelationshipParser{
		cueContext:            ctx,
		explicitRelationships: make(map[string]map[string]*RelationshipConfig),
	}
}

// RegisterRelationship allows explicit registration of relationships for Phase 3.1
// This provides a simple way to add relationships without CUE attribute parsing
func (p *RelationshipParser) RegisterRelationship(kindName string, config *RelationshipConfig) {
	if p.explicitRelationships[kindName] == nil {
		p.explicitRelationships[kindName] = make(map[string]*RelationshipConfig)
	}
	p.explicitRelationships[kindName][config.FieldName] = config
}

// ParseRelationships returns relationships for a kind - combines CUE-parsed and explicitly registered
func (p *RelationshipParser) ParseRelationships(kind resource.Kind) (map[string]*RelationshipConfig, error) {
	relationships := make(map[string]*RelationshipConfig)

	// TODO: Phase 3.2 - Add CUE @relation attribute parsing here
	// For now, just return explicitly registered relationships

	// Add explicitly registered relationships
	kindName := kind.Kind()
	if kindRelationships, exists := p.explicitRelationships[kindName]; exists {
		for fieldName, config := range kindRelationships {
			relationships[fieldName] = config
		}
	}

	return relationships, nil
}

// walkFields recursively walks CUE fields looking for @relation attributes
func (p *RelationshipParser) walkFields(value cue.Value, path string, relationships map[string]*RelationshipConfig) error {
	// Check if this field has a @relation attribute
	if p.hasRelationAttribute(value) {
		rel, err := p.parseRelationAttribute(value, path)
		if err != nil {
			return fmt.Errorf("failed to parse @relation attribute at path %s: %w", path, err)
		}
		relationships[rel.FieldName] = rel
	}

	// Recursively walk struct fields
	if value.Kind() == cue.StructKind {
		iter, err := value.Fields(cue.Optional(true))
		if err != nil {
			return err
		}

		for iter.Next() {
			fieldName := iter.Label()
			fieldValue := iter.Value()

			// Build field path
			fieldPath := fieldName
			if path != "" {
				fieldPath = path + "." + fieldName
			}

			// Recursively check this field
			err := p.walkFields(fieldValue, fieldPath, relationships)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// hasRelationAttribute checks if a CUE value has a @relation attribute
func (p *RelationshipParser) hasRelationAttribute(value cue.Value) bool {
	attrs := value.Attributes(cue.ValueAttr)
	for _, attr := range attrs {
		if attr.Name() == "relation" {
			return true
		}
	}
	return false
}

// parseRelationAttribute parses a single @relation attribute
func (p *RelationshipParser) parseRelationAttribute(value cue.Value, path string) (*RelationshipConfig, error) {
	attrs := value.Attributes(cue.ValueAttr)

	var relationAttr *cue.Attribute
	for _, attr := range attrs {
		if attr.Name() == "relation" {
			relationAttr = &attr
			break
		}
	}

	if relationAttr == nil {
		return nil, fmt.Errorf("@relation attribute not found")
	}

	// Parse attribute parameters
	rel := &RelationshipConfig{
		FieldName:   p.getFieldNameFromPath(path),
		Optional:    true,            // Default to optional
		Cardinality: "one",           // Default to one-to-one
		Match:       "exact",         // Default to exact match
		TargetField: "metadata.name", // Default target field
	}

	// Parse required parameters
	kindParam, err := relationAttr.String(0) // First parameter is kind
	if err != nil {
		return nil, fmt.Errorf("kind parameter is required: %w", err)
	}
	rel.Kind = strings.Trim(kindParam, `"`)

	fieldParam, err := relationAttr.String(1) // Second parameter is field
	if err != nil {
		return nil, fmt.Errorf("field parameter is required: %w", err)
	}
	rel.SourceField = strings.Trim(fieldParam, `"`)

	// Parse optional parameters
	if numArgs := relationAttr.NumArgs(); numArgs > 2 {
		for i := 2; i < numArgs; i++ {
			param, err := relationAttr.String(i)
			if err != nil {
				continue
			}
			param = strings.Trim(param, `"`)

			// Parse key=value parameters
			if strings.Contains(param, "=") {
				parts := strings.SplitN(param, "=", 2)
				key, value := parts[0], parts[1]

				switch key {
				case "target":
					rel.TargetField = value
				case "optional":
					rel.Optional = value == "true"
				case "cardinality":
					rel.Cardinality = value
				case "match":
					rel.Match = value
				}
			}
		}
	}

	// Parse target GVK
	gvk, err := p.parseKindToGVK(rel.Kind)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target kind %s: %w", rel.Kind, err)
	}
	rel.TargetGVK = gvk

	return rel, nil
}

// getFieldNameFromPath extracts the field name from a dot-separated path
func (p *RelationshipParser) getFieldNameFromPath(path string) string {
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}

// parseKindToGVK converts a kind string to GroupVersionKind
func (p *RelationshipParser) parseKindToGVK(kindStr string) (schema.GroupVersionKind, error) {
	// Handle formats like "dashboard.grafana.app/Dashboard" or "apps/Deployment"
	parts := strings.Split(kindStr, "/")
	if len(parts) != 2 {
		return schema.GroupVersionKind{}, fmt.Errorf("invalid kind format, expected 'group/Kind': %s", kindStr)
	}

	group := parts[0]
	kind := parts[1]

	// For now, we'll assume v0alpha1 as the version
	// In a production system, this would need to be more sophisticated
	version := "v0alpha1"

	// Handle core Kubernetes resources (no group)
	if group == "apps" || group == "core" {
		// These would be standard Kubernetes resources
	}

	return schema.GroupVersionKind{
		Group:   group,
		Version: version,
		Kind:    kind,
	}, nil
}

// RelationshipResolver generates GraphQL resolvers for relationships
type RelationshipResolver struct {
	config   *RelationshipConfig
	registry SubgraphRegistry
}

// SubgraphRegistry interface for finding target subgraphs
type SubgraphRegistry interface {
	GetSubgraphForKind(gvk schema.GroupVersionKind) (SubgraphInterface, error)
}

// SubgraphInterface represents a GraphQL subgraph
type SubgraphInterface interface {
	GetStorage(gvr schema.GroupVersionResource) Storage
}

// Storage interface for querying resources
type Storage interface {
	Get(namespace, name string) (interface{}, error)
	List(namespace string, opts ...interface{}) (interface{}, error)
}

// NewRelationshipResolver creates a new relationship resolver
func NewRelationshipResolver(config *RelationshipConfig, registry SubgraphRegistry) *RelationshipResolver {
	return &RelationshipResolver{
		config:   config,
		registry: registry,
	}
}

// CreateGraphQLField creates a GraphQL field for this relationship
func (r *RelationshipResolver) CreateGraphQLField() *graphql.Field {
	// Determine GraphQL type based on cardinality
	var fieldType graphql.Type

	// For now, we'll use a generic JSON type for the target
	// In Phase 3.2, we'll generate proper types for target kinds
	targetType := graphql.NewScalar(graphql.ScalarConfig{
		Name:        "RelatedResource",
		Description: "A related resource from another subgraph",
		Serialize:   func(value interface{}) interface{} { return value },
	})

	if r.config.Cardinality == "many" {
		fieldType = graphql.NewList(targetType)
	} else {
		if r.config.Optional {
			fieldType = targetType
		} else {
			fieldType = graphql.NewNonNull(targetType)
		}
	}

	return &graphql.Field{
		Type:        fieldType,
		Description: fmt.Sprintf("Related %s via %s", r.config.Kind, r.config.SourceField),
		Resolve:     r.createResolverFunc(),
	}
}

// createResolverFunc creates the actual GraphQL resolver function
func (r *RelationshipResolver) createResolverFunc() graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		// Extract reference value from source object
		refValue, err := r.extractFieldValue(p.Source, r.config.SourceField)
		if err != nil {
			if r.config.Optional {
				return nil, nil // Return null for optional relationships
			}
			return nil, err
		}

		// Handle null/empty reference values
		if refValue == nil || refValue == "" {
			if r.config.Optional {
				return nil, nil
			}
			return nil, fmt.Errorf("required relationship field %s is empty", r.config.SourceField)
		}

		// Find target subgraph
		subgraph, err := r.registry.GetSubgraphForKind(r.config.TargetGVK)
		if err != nil {
			if r.config.Optional {
				return nil, nil // Graceful degradation for optional relationships
			}
			return nil, fmt.Errorf("target subgraph not found for %s: %w", r.config.Kind, err)
		}

		// Get target storage
		gvr := schema.GroupVersionResource{
			Group:    r.config.TargetGVK.Group,
			Version:  r.config.TargetGVK.Version,
			Resource: strings.ToLower(r.config.TargetGVK.Kind) + "s", // Simple pluralization
		}

		storage := subgraph.GetStorage(gvr)
		if storage == nil {
			if r.config.Optional {
				return nil, nil
			}
			return nil, fmt.Errorf("storage not found for %s", gvr)
		}

		// Query target resource
		return r.queryTargetResource(storage, refValue, p)
	}
}

// extractFieldValue extracts a value from an object using dot notation
func (r *RelationshipResolver) extractFieldValue(source interface{}, fieldPath string) (interface{}, error) {
	if source == nil {
		return nil, fmt.Errorf("source object is nil")
	}

	// Convert source to map for field access
	sourceMap, ok := source.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("source is not a map: %T", source)
	}

	// Handle dot notation field paths
	parts := strings.Split(fieldPath, ".")
	current := interface{}(sourceMap)

	for _, part := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot access field %s: current value is not a map", part)
		}

		value, exists := currentMap[part]
		if !exists {
			return nil, fmt.Errorf("field %s not found", part)
		}

		current = value
	}

	return current, nil
}

// queryTargetResource queries the target resource based on relationship config
func (r *RelationshipResolver) queryTargetResource(storage Storage, refValue interface{}, p graphql.ResolveParams) (interface{}, error) {
	// For now, implement simple exact matching
	// In Phase 3.2, we'll add support for complex matching strategies

	switch r.config.Match {
	case "exact":
		return r.queryExactMatch(storage, refValue, p)
	default:
		return nil, fmt.Errorf("unsupported match strategy: %s", r.config.Match)
	}
}

// queryExactMatch performs exact value matching
func (r *RelationshipResolver) queryExactMatch(storage Storage, refValue interface{}, p graphql.ResolveParams) (interface{}, error) {
	refStr, ok := refValue.(string)
	if !ok {
		return nil, fmt.Errorf("reference value must be string for exact match, got %T", refValue)
	}

	// Extract namespace from context
	// For now, we'll use a simple approach - in production, this would be more sophisticated
	namespace := "default"
	if p.Context != nil {
		if ns, ok := p.Context.Value("namespace").(string); ok {
			namespace = ns
		}
	}

	// Query by target field
	if r.config.TargetField == "metadata.name" {
		return storage.Get(namespace, refStr)
	} else if r.config.TargetField == "metadata.uid" {
		// For UID lookup, we might need to list and filter
		// This is a simplified implementation
		return storage.Get(namespace, refStr)
	}

	return nil, fmt.Errorf("unsupported target field: %s", r.config.TargetField)
}
