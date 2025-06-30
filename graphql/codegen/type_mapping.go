package codegen

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/graphql-go/graphql"
)

// CUETypeMapper converts CUE types to GraphQL types
type CUETypeMapper struct {
	// Cache for generated types to avoid duplicates
	typeCache map[string]graphql.Type

	// Generated enum types
	enumTypes map[string]*graphql.Enum

	// Generated object types
	objectTypes map[string]*graphql.Object
}

// NewCUETypeMapper creates a new CUE type mapper
func NewCUETypeMapper() *CUETypeMapper {
	return &CUETypeMapper{
		typeCache:   make(map[string]graphql.Type),
		enumTypes:   make(map[string]*graphql.Enum),
		objectTypes: make(map[string]*graphql.Object),
	}
}

// MapCUEToGraphQL converts a CUE value to a GraphQL type
func (m *CUETypeMapper) MapCUEToGraphQL(cueValue cue.Value, typeName string) graphql.Type {
	// Check cache first
	if cachedType, exists := m.typeCache[typeName]; exists {
		return cachedType
	}

	var graphqlType graphql.Type

	switch cueValue.Kind() {
	case cue.StringKind:
		graphqlType = m.mapStringType(cueValue, typeName)
	case cue.IntKind, cue.FloatKind, cue.NumberKind:
		graphqlType = m.mapNumberType(cueValue, typeName)
	case cue.BoolKind:
		graphqlType = graphql.Boolean
	case cue.ListKind:
		graphqlType = m.mapListType(cueValue, typeName)
	case cue.StructKind:
		graphqlType = m.mapStructType(cueValue, typeName)
	default:
		// Fallback to JSON scalar for unsupported types
		graphqlType = m.createJSONScalar()
	}

	// Cache the result
	m.typeCache[typeName] = graphqlType
	return graphqlType
}

// mapStringType maps CUE string types to GraphQL types
func (m *CUETypeMapper) mapStringType(cueValue cue.Value, typeName string) graphql.Type {
	// Check for string constraints/enums
	constraints := m.extractStringConstraints(cueValue)

	// If we have a fixed set of values, create an enum
	if len(constraints.EnumValues) > 0 {
		return m.createEnum(typeName, constraints.EnumValues)
	}

	// Check for special string formats
	if constraints.Format != "" {
		switch constraints.Format {
		case "date-time":
			return graphql.DateTime
		case "email":
			return graphql.String // Could create custom Email scalar
		case "uri":
			return graphql.String // Could create custom URI scalar
		}
	}

	return graphql.String
}

// mapNumberType maps CUE number types to GraphQL types
func (m *CUETypeMapper) mapNumberType(cueValue cue.Value, typeName string) graphql.Type {
	if m.isIntegerConstrained(cueValue) {
		return graphql.Int
	}
	return graphql.Float
}

// mapListType maps CUE list types to GraphQL list types
func (m *CUETypeMapper) mapListType(cueValue cue.Value, typeName string) graphql.Type {
	// Get the element type from the list
	elemValue := m.getListElementType(cueValue)
	if !elemValue.Exists() {
		// Fallback to JSON if we can't determine element type
		return graphql.NewList(m.createJSONScalar())
	}

	elemType := m.MapCUEToGraphQL(elemValue, typeName+"Item")
	return graphql.NewList(elemType)
}

// mapStructType maps CUE struct types to GraphQL object types
func (m *CUETypeMapper) mapStructType(cueValue cue.Value, typeName string) graphql.Type {
	// Check if we already have this object type
	if objType, exists := m.objectTypes[typeName]; exists {
		return objType
	}

	fields := make(graphql.Fields)

	// Iterate through struct fields
	iter, err := cueValue.Fields(cue.Optional(true))
	if err != nil {
		// Fallback to JSON scalar if we can't iterate fields
		return m.createJSONScalar()
	}

	for iter.Next() {
		fieldName := iter.Label()
		fieldValue := iter.Value()

		// Convert CUE field name to GraphQL field name
		graphqlFieldName := m.toGraphQLFieldName(fieldName)

		// Map field type
		fieldTypeName := fmt.Sprintf("%s%s", typeName, strings.Title(graphqlFieldName))
		fieldType := m.MapCUEToGraphQL(fieldValue, fieldTypeName)

		// Check if field is optional
		isOptional := m.isOptionalField(fieldValue)
		if !isOptional {
			fieldType = graphql.NewNonNull(fieldType)
		}

		fields[graphqlFieldName] = &graphql.Field{
			Type:        fieldType,
			Description: m.extractFieldDescription(fieldValue),
		}
	}

	// Create the object type
	objType := graphql.NewObject(graphql.ObjectConfig{
		Name:   typeName,
		Fields: fields,
	})

	// Cache it
	m.objectTypes[typeName] = objType
	return objType
}

// StringConstraints represents constraints on a string type
type StringConstraints struct {
	EnumValues []string
	Pattern    string
	Format     string
	MinLength  int
	MaxLength  int
}

// extractStringConstraints extracts constraints from a CUE string value
func (m *CUETypeMapper) extractStringConstraints(cueValue cue.Value) StringConstraints {
	constraints := StringConstraints{}

	// Try to extract enum values from disjunctions
	op, values := cueValue.Expr()
	if op == cue.OrOp {
		for _, val := range values {
			if val.Kind() == cue.StringKind {
				str, err := val.String()
				if err == nil {
					constraints.EnumValues = append(constraints.EnumValues, str)
				}
			}
		}
	}

	// TODO: Extract other constraints like pattern, format, length limits
	// This would require more sophisticated CUE constraint analysis

	return constraints
}

// isIntegerConstrained checks if a number type is constrained to integers
func (m *CUETypeMapper) isIntegerConstrained(cueValue cue.Value) bool {
	// Check if the value has integer constraints
	// This is a simplified check - a full implementation would analyze CUE constraints
	return cueValue.Kind() == cue.IntKind
}

// getListElementType gets the element type from a CUE list
func (m *CUETypeMapper) getListElementType(cueValue cue.Value) cue.Value {
	// For CUE lists, we need to look at the element constraint
	// This is a simplified implementation
	iter, err := cueValue.List()
	if err != nil {
		return cue.Value{}
	}

	// Get the first element to infer type
	if iter.Next() {
		return iter.Value()
	}

	return cue.Value{}
}

// createEnum creates a GraphQL enum type
func (m *CUETypeMapper) createEnum(typeName string, values []string) *graphql.Enum {
	// Check if we already have this enum
	if enumType, exists := m.enumTypes[typeName]; exists {
		return enumType
	}

	enumValues := make(graphql.EnumValueConfigMap)
	for _, value := range values {
		enumValues[value] = &graphql.EnumValueConfig{
			Value: value,
		}
	}

	enumType := graphql.NewEnum(graphql.EnumConfig{
		Name:   typeName,
		Values: enumValues,
	})

	m.enumTypes[typeName] = enumType
	return enumType
}

// createJSONScalar creates a JSON scalar type for fallback cases
// Note: This should be replaced with a shared scalar to prevent duplicates
func (m *CUETypeMapper) createJSONScalar() *graphql.Scalar {
	// TODO: Use shared JSON scalar from generator to prevent duplicates
	// For now, we'll still create individual scalars but this needs to be fixed
	return createSharedJSONScalar()
}

// toGraphQLFieldName converts CUE field names to GraphQL field names
func (m *CUETypeMapper) toGraphQLFieldName(cueFieldName string) string {
	// Remove CUE-specific characters like quotes
	name := strings.Trim(cueFieldName, `"`)

	// Convert to camelCase if needed
	// For now, keep the original name
	return name
}

// isOptionalField checks if a CUE field is optional
func (m *CUETypeMapper) isOptionalField(cueValue cue.Value) bool {
	// In CUE, optional fields are marked with ?
	// This is a simplified check - would need more sophisticated analysis
	return true // For now, assume all fields are optional
}

// extractFieldDescription extracts documentation from CUE field
func (m *CUETypeMapper) extractFieldDescription(cueValue cue.Value) string {
	// Extract CUE comments as field descriptions
	// This would require accessing CUE's AST for comments
	return ""
}

// EnhancedGraphQLGenerator extends the basic generator with enhanced type mapping
type EnhancedGraphQLGenerator struct {
	*GraphQLGenerator
	typeMapper *CUETypeMapper
}

// NewEnhancedGraphQLGenerator creates a generator with enhanced type mapping
func NewEnhancedGraphQLGenerator(base *GraphQLGenerator) *EnhancedGraphQLGenerator {
	return &EnhancedGraphQLGenerator{
		GraphQLGenerator: base,
		typeMapper:       NewCUETypeMapper(),
	}
}

// GenerateEnhancedSchema generates a GraphQL schema with enhanced type mapping
func (g *EnhancedGraphQLGenerator) GenerateEnhancedSchema() (*graphql.Schema, error) {
	// This would be an enhanced version of GenerateSchema that uses
	// the CUE type mapper for more sophisticated type generation

	// For now, delegate to the base implementation
	// In a full implementation, this would:
	// 1. Use CUE type mapper for spec/status fields instead of JSON scalars
	// 2. Generate proper object types for nested structures
	// 3. Create enums from CUE string constraints
	// 4. Handle more sophisticated type relationships

	return g.GraphQLGenerator.GenerateSchema()
}

// Example usage showing enhanced type mapping capability:
//
// CUE Schema:
// ```cue
// #Dashboard: {
//   spec: {
//     title: string
//     tags: [...string]
//     refresh: "5s" | "10s" | "30s" | "1m" | "5m"  // Enum
//     panels: [...#Panel]                          // Nested objects
//   }
// }
// ```
//
// Generated GraphQL:
// ```graphql
// type DashboardSpec {
//   title: String!
//   tags: [String!]!
//   refresh: RefreshInterval!  # Generated enum
//   panels: [Panel!]!          # Generated object types
// }
//
// enum RefreshInterval {
//   FIVE_SECONDS
//   TEN_SECONDS
//   THIRTY_SECONDS
//   ONE_MINUTE
//   FIVE_MINUTES
// }
// ```
