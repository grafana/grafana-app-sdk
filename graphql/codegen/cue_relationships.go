package codegen

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/grafana-app-sdk/codegen"
)

// MeshRelationshipConfig defines a Mesh-style relationship configuration
// used for cross-service relationships in GraphQL Mesh architecture
type MeshRelationshipConfig struct {
	// Source configuration
	SourceType  string `json:"sourceType"`  // e.g., "PlaylistItem"
	SourceField string `json:"sourceField"` // e.g., "dashboard"

	// Target configuration
	TargetType      string            `json:"targetType"`      // e.g., "Dashboard"
	TargetService   string            `json:"targetService"`   // e.g., "dashboard"
	TargetQuery     string            `json:"targetQuery"`     // e.g., "dashboard"
	TargetArguments map[string]string `json:"targetArguments"` // e.g., {"name": "{source.value}"}

	// Selection requirements
	RequiredSelection []string `json:"requiredSelection"` // e.g., ["value", "type"]

	// Optional transformation
	Transform func(source interface{}) map[string]interface{} `json:"-"`
}

// CUERelationshipParser extracts relationship definitions from CUE kinds
// and generates Mesh-style relationship configurations for the gateway
type CUERelationshipParser struct {
	kinds []codegen.Kind
}

// NewCUERelationshipParser creates a parser for extracting relationships from CUE kinds
func NewCUERelationshipParser(kinds []codegen.Kind) *CUERelationshipParser {
	return &CUERelationshipParser{
		kinds: kinds,
	}
}

// ParseRelationships extracts all relationship definitions from CUE kinds
// and returns Mesh-style relationship configurations
func (p *CUERelationshipParser) ParseRelationships() ([]MeshRelationshipConfig, error) {
	var relationships []MeshRelationshipConfig

	for _, kind := range p.kinds {
		// Get the current version of the kind
		currentVersion := kind.Version(kind.Properties().Current)
		if currentVersion == nil {
			continue // Skip kinds without current version
		}

		// Look for _relationships field in the CUE schema using field iteration
		// Hidden fields require special handling with cue.Hidden(true) option
		relationshipsField, found := p.findRelationshipsField(currentVersion.Schema)
		if !found {
			continue // No relationships defined for this kind
		}

		// Parse each relationship definition
		kindRelationships, err := p.parseKindRelationships(kind, relationshipsField)
		if err != nil {
			return nil, fmt.Errorf("failed to parse relationships for kind %s: %w", kind.Name(), err)
		}

		relationships = append(relationships, kindRelationships...)
	}

	return relationships, nil
}

// findRelationshipsField looks for the _relationships field in a CUE value
// Hidden fields require iterating with the Hidden(true) option
func (p *CUERelationshipParser) findRelationshipsField(schema cue.Value) (cue.Value, bool) {
	// Iterate through fields including hidden ones
	iter, err := schema.Fields(cue.Hidden(true))
	if err != nil {
		return cue.Value{}, false
	}

	for iter.Next() {
		if iter.Selector().String() == "_relationships" {
			return iter.Value(), true
		}
	}

	return cue.Value{}, false
}

// parseKindRelationships parses relationship definitions for a specific kind
func (p *CUERelationshipParser) parseKindRelationships(kind codegen.Kind, relationshipsField cue.Value) ([]MeshRelationshipConfig, error) {
	var relationships []MeshRelationshipConfig

	// Iterate through each relationship definition
	iter, err := relationshipsField.Fields()
	if err != nil {
		return nil, fmt.Errorf("failed to iterate relationships fields: %w", err)
	}

	for iter.Next() {
		fieldPath := iter.Label()       // e.g., "spec.items.dashboard"
		relationshipDef := iter.Value() // The relationship configuration

		// Parse this specific relationship
		rel, err := p.parseRelationshipDefinition(kind, fieldPath, relationshipDef)
		if err != nil {
			return nil, fmt.Errorf("failed to parse relationship %s: %w", fieldPath, err)
		}

		relationships = append(relationships, rel)
	}

	return relationships, nil
}

// parseRelationshipDefinition parses a single relationship definition
func (p *CUERelationshipParser) parseRelationshipDefinition(kind codegen.Kind, fieldPath string, def cue.Value) (MeshRelationshipConfig, error) {
	// Extract target information
	target := def.LookupPath(cue.ParsePath("target"))
	if !target.Exists() {
		return MeshRelationshipConfig{}, fmt.Errorf("relationship missing 'target' field")
	}

	targetKind, err := target.LookupPath(cue.ParsePath("kind")).String()
	if err != nil {
		return MeshRelationshipConfig{}, fmt.Errorf("relationship target missing 'kind': %w", err)
	}

	targetGroup, err := target.LookupPath(cue.ParsePath("group")).String()
	if err != nil {
		return MeshRelationshipConfig{}, fmt.Errorf("relationship target missing 'group': %w", err)
	}

	targetVersion, _ := target.LookupPath(cue.ParsePath("version")).String()
	if targetVersion == "" {
		targetVersion = "v1alpha1" // Default version
	}

	// Extract resolver information
	resolver := def.LookupPath(cue.ParsePath("resolver"))
	if !resolver.Exists() {
		return MeshRelationshipConfig{}, fmt.Errorf("relationship missing 'resolver' field")
	}

	sourceField, err := resolver.LookupPath(cue.ParsePath("sourceField")).String()
	if err != nil {
		return MeshRelationshipConfig{}, fmt.Errorf("relationship resolver missing 'sourceField': %w", err)
	}

	targetQuery, err := resolver.LookupPath(cue.ParsePath("targetQuery")).String()
	if err != nil {
		return MeshRelationshipConfig{}, fmt.Errorf("relationship resolver missing 'targetQuery': %w", err)
	}

	// Parse target arguments
	targetArgs := make(map[string]string)
	targetArgsField := resolver.LookupPath(cue.ParsePath("targetArgs"))
	if targetArgsField.Exists() {
		argsIter, err := targetArgsField.Fields()
		if err == nil {
			for argsIter.Next() {
				key := argsIter.Label()
				value, err := argsIter.Value().String()
				if err == nil {
					targetArgs[key] = value
				}
			}
		}
	}

	// Parse optional condition
	condition, _ := resolver.LookupPath(cue.ParsePath("condition")).String()

	// Parse optional cardinality
	cardinality, _ := resolver.LookupPath(cue.ParsePath("cardinality")).String()
	if cardinality == "" {
		cardinality = "one" // Default to one-to-one
	}

	// Determine source type and field name from field path
	sourceType := kind.Properties().Kind
	sourceFieldName := p.extractFieldName(fieldPath)

	// Create Mesh-style relationship configuration
	relationshipConfig := MeshRelationshipConfig{
		SourceType:        sourceType,
		SourceField:       sourceFieldName,
		TargetType:        targetKind,
		TargetService:     p.normalizeServiceName(targetGroup),
		TargetQuery:       targetQuery,
		TargetArguments:   targetArgs,
		RequiredSelection: []string{sourceField},
	}

	// Add condition-based transform if specified
	if condition != "" {
		relationshipConfig.Transform = p.createConditionTransform(condition, sourceField)
	}

	return relationshipConfig, nil
}

// extractFieldName extracts the final field name from a path like "spec.items.dashboard"
func (p *CUERelationshipParser) extractFieldName(fieldPath string) string {
	parts := strings.Split(fieldPath, ".")
	return parts[len(parts)-1]
}

// normalizeServiceName converts a group name to a service name
// e.g., "dashboard.grafana.app" -> "dashboard"
func (p *CUERelationshipParser) normalizeServiceName(group string) string {
	parts := strings.Split(group, ".")
	return strings.ToLower(parts[0])
}

// createConditionTransform creates a transform function based on a CUE condition
func (p *CUERelationshipParser) createConditionTransform(condition, sourceField string) func(source interface{}) map[string]interface{} {
	return func(source interface{}) map[string]interface{} {
		sourceData, ok := source.(map[string]interface{})
		if !ok {
			return nil
		}

		// Simple condition evaluation - can be enhanced with more sophisticated parsing
		switch condition {
		case "type == 'dashboard_by_uid'":
			if sourceData["type"] == "dashboard_by_uid" {
				return sourceData
			}
		case "type == 'dashboard_by_tag'":
			if sourceData["type"] == "dashboard_by_tag" {
				return sourceData
			}
		case "relatedPlaylistUID != null":
			if sourceData["relatedPlaylistUID"] != nil && sourceData["relatedPlaylistUID"] != "" {
				return sourceData
			}
		default:
			// For more complex conditions, we could implement a proper CUE expression evaluator
			// For now, return the source data if we can't evaluate the condition
			return sourceData
		}

		return nil // Condition not met, skip this relationship
	}
}

// CUERelationshipMetadata represents the structure of relationship metadata in CUE
type CUERelationshipMetadata struct {
	Target   CUERelationshipTarget   `json:"target"`
	Resolver CUERelationshipResolver `json:"resolver"`
}

// CUERelationshipTarget represents the target kind information
type CUERelationshipTarget struct {
	Kind    string `json:"kind"`
	Group   string `json:"group"`
	Version string `json:"version"`
}

// CUERelationshipResolver represents the resolver configuration
type CUERelationshipResolver struct {
	SourceField string            `json:"sourceField"`
	Condition   string            `json:"condition,omitempty"`
	TargetQuery string            `json:"targetQuery"`
	TargetArgs  map[string]string `json:"targetArgs,omitempty"`
	Cardinality string            `json:"cardinality,omitempty"`
}

// GetRelationshipFromCUE extracts a single relationship from a CUE value
// This is a utility function for testing and direct CUE value access
func GetRelationshipFromCUE(cueValue cue.Value, fieldPath string) (*MeshRelationshipConfig, error) {
	parser := NewCUERelationshipParser([]codegen.Kind{})

	// Create a mock kind for parsing
	mockKind := &mockKind{
		name: "TestKind",
		props: codegen.KindProperties{
			Kind: "TestKind",
		},
	}

	config, err := parser.parseRelationshipDefinition(mockKind, fieldPath, cueValue)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// mockKind is a simple implementation of codegen.Kind for testing
type mockKind struct {
	name  string
	props codegen.KindProperties
}

func (m *mockKind) Name() string {
	return m.name
}

func (m *mockKind) Properties() codegen.KindProperties {
	return m.props
}

func (m *mockKind) Versions() []codegen.KindVersion {
	return []codegen.KindVersion{}
}

func (m *mockKind) Version(version string) *codegen.KindVersion {
	return nil
}

// ValidationError represents a validation error in relationship parsing
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error in field %s: %s", e.Field, e.Message)
}

// ValidateRelationshipDefinition validates that a relationship definition has all required fields
func ValidateRelationshipDefinition(def cue.Value) error {
	// Check required target fields
	target := def.LookupPath(cue.ParsePath("target"))
	if !target.Exists() {
		return ValidationError{Field: "target", Message: "target field is required"}
	}

	if _, err := target.LookupPath(cue.ParsePath("kind")).String(); err != nil {
		return ValidationError{Field: "target.kind", Message: "kind is required"}
	}

	if _, err := target.LookupPath(cue.ParsePath("group")).String(); err != nil {
		return ValidationError{Field: "target.group", Message: "group is required"}
	}

	// Check required resolver fields
	resolver := def.LookupPath(cue.ParsePath("resolver"))
	if !resolver.Exists() {
		return ValidationError{Field: "resolver", Message: "resolver field is required"}
	}

	if _, err := resolver.LookupPath(cue.ParsePath("sourceField")).String(); err != nil {
		return ValidationError{Field: "resolver.sourceField", Message: "sourceField is required"}
	}

	if _, err := resolver.LookupPath(cue.ParsePath("targetQuery")).String(); err != nil {
		return ValidationError{Field: "resolver.targetQuery", Message: "targetQuery is required"}
	}

	return nil
}
