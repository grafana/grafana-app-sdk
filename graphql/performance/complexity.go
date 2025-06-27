package performance

import (
	"fmt"
	"strings"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
)

// ComplexityAnalyzer analyzes and limits GraphQL query complexity
type ComplexityAnalyzer struct {
	// Maximum allowed complexity
	maxComplexity int

	// Complexity calculation rules
	rules ComplexityRules

	// Field-specific complexity overrides
	fieldComplexity map[string]int

	// Introspection query complexity
	introspectionComplexity int
}

// ComplexityRules defines how to calculate query complexity
type ComplexityRules struct {
	// Base cost for any field
	DefaultFieldCost int

	// Cost multiplier for list fields
	ListMultiplier int

	// Cost for relationship fields (cross-subgraph queries)
	RelationshipCost int

	// Additional cost per nesting level
	DepthMultiplier int

	// Cost for arguments (pagination, filtering)
	ArgumentCost int
}

// DefaultComplexityRules returns sensible defaults
func DefaultComplexityRules() ComplexityRules {
	return ComplexityRules{
		DefaultFieldCost: 1,
		ListMultiplier:   10,
		RelationshipCost: 5,
		DepthMultiplier:  2,
		ArgumentCost:     1,
	}
}

// ComplexityConfig configures the complexity analyzer
type ComplexityConfig struct {
	MaxComplexity           int
	Rules                   ComplexityRules
	FieldComplexity         map[string]int
	IntrospectionComplexity int
	Enabled                 bool
}

// NewComplexityAnalyzer creates a new complexity analyzer
func NewComplexityAnalyzer(config ComplexityConfig) *ComplexityAnalyzer {
	if config.MaxComplexity == 0 {
		config.MaxComplexity = 1000 // Default max complexity
	}
	if config.IntrospectionComplexity == 0 {
		config.IntrospectionComplexity = 100
	}
	if config.FieldComplexity == nil {
		config.FieldComplexity = make(map[string]int)
	}

	return &ComplexityAnalyzer{
		maxComplexity:           config.MaxComplexity,
		rules:                   config.Rules,
		fieldComplexity:         config.FieldComplexity,
		introspectionComplexity: config.IntrospectionComplexity,
	}
}

// AnalyzeQuery calculates the complexity of a GraphQL query
func (ca *ComplexityAnalyzer) AnalyzeQuery(query *ast.Document, schema *graphql.Schema) (int, error) {
	if query == nil {
		return 0, fmt.Errorf("query document is nil")
	}

	// Check for introspection queries
	if ca.isIntrospectionQuery(query) {
		return ca.introspectionComplexity, nil
	}

	// Calculate complexity by walking the AST
	complexity := 0

	for _, def := range query.Definitions {
		if opDef, ok := def.(*ast.OperationDefinition); ok {
			complexity += ca.analyzeSelectionSet(opDef.SelectionSet, 0, schema)
		}
	}

	return complexity, nil
}

// analyzeSelectionSet recursively analyzes a selection set
func (ca *ComplexityAnalyzer) analyzeSelectionSet(selectionSet *ast.SelectionSet, depth int, schema *graphql.Schema) int {
	if selectionSet == nil {
		return 0
	}

	complexity := 0

	for _, selection := range selectionSet.Selections {
		switch sel := selection.(type) {
		case *ast.Field:
			fieldComplexity := ca.calculateFieldComplexity(sel, depth, schema)
			complexity += fieldComplexity

			// Recursively analyze nested selections
			if sel.SelectionSet != nil {
				complexity += ca.analyzeSelectionSet(sel.SelectionSet, depth+1, schema)
			}
		case *ast.InlineFragment:
			if sel.SelectionSet != nil {
				complexity += ca.analyzeSelectionSet(sel.SelectionSet, depth, schema)
			}
		case *ast.FragmentSpread:
			// For fragment spreads, we'd need to look up the fragment definition
			// For now, add a base cost
			complexity += ca.rules.DefaultFieldCost
		}
	}

	return complexity
}

// ValidateComplexity checks if a query exceeds the maximum allowed complexity
func (ca *ComplexityAnalyzer) ValidateComplexity(query *ast.Document, schema *graphql.Schema) error {
	complexity, err := ca.AnalyzeQuery(query, schema)
	if err != nil {
		return fmt.Errorf("failed to analyze query complexity: %w", err)
	}

	if complexity > ca.maxComplexity {
		return fmt.Errorf("query complexity %d exceeds maximum allowed complexity %d", complexity, ca.maxComplexity)
	}

	return nil
}

// calculateFieldComplexity calculates complexity for a specific field
func (ca *ComplexityAnalyzer) calculateFieldComplexity(field *ast.Field, depth int, schema *graphql.Schema) int {
	fieldName := field.Name.Value

	// Check for field-specific complexity overrides
	if override, exists := ca.fieldComplexity[fieldName]; exists {
		return override
	}

	complexity := ca.rules.DefaultFieldCost

	// Add complexity for arguments
	if field.Arguments != nil && len(field.Arguments) > 0 {
		complexity += len(field.Arguments) * ca.rules.ArgumentCost
	}

	// Check if this is a list field
	if ca.isListField(fieldName, schema) {
		complexity *= ca.rules.ListMultiplier

		// Apply pagination limits if available
		if limit := ca.extractPaginationLimit(field); limit > 0 {
			// Use actual limit instead of default multiplier
			complexity = ca.rules.DefaultFieldCost + limit
		}
	}

	// Check if this is a relationship field
	if ca.isRelationshipField(fieldName) {
		complexity += ca.rules.RelationshipCost
	}

	// Apply depth multiplier
	complexity += depth * ca.rules.DepthMultiplier

	return complexity
}

// isIntrospectionQuery checks if the query is for GraphQL introspection
func (ca *ComplexityAnalyzer) isIntrospectionQuery(query *ast.Document) bool {
	for _, def := range query.Definitions {
		if opDef, ok := def.(*ast.OperationDefinition); ok {
			if ca.hasIntrospectionFields(opDef.SelectionSet) {
				return true
			}
		}
	}
	return false
}

// hasIntrospectionFields checks if a selection set contains introspection fields
func (ca *ComplexityAnalyzer) hasIntrospectionFields(selectionSet *ast.SelectionSet) bool {
	if selectionSet == nil {
		return false
	}

	for _, selection := range selectionSet.Selections {
		if field, ok := selection.(*ast.Field); ok {
			if strings.HasPrefix(field.Name.Value, "__") {
				return true
			}
			// Recursively check nested selections
			if ca.hasIntrospectionFields(field.SelectionSet) {
				return true
			}
		}
	}
	return false
}

// isListField checks if a field returns a list type
func (ca *ComplexityAnalyzer) isListField(fieldName string, schema *graphql.Schema) bool {
	// This is a simplified check - in a real implementation, you'd inspect the schema
	// to determine if the field returns a list type

	// Common patterns for list fields
	listPatterns := []string{"s", "List", "list", "items", "results"}

	for _, pattern := range listPatterns {
		if strings.HasSuffix(fieldName, pattern) {
			return true
		}
	}

	return false
}

// isRelationshipField checks if a field represents a relationship
func (ca *ComplexityAnalyzer) isRelationshipField(fieldName string) bool {
	// Relationship fields are typically those that resolve to other types
	// This is a simplified check - in practice, you'd have more sophisticated detection

	relationshipPatterns := []string{"dashboard", "playlist", "folder", "user", "team"}

	for _, pattern := range relationshipPatterns {
		if strings.Contains(strings.ToLower(fieldName), pattern) {
			return true
		}
	}

	return false
}

// extractPaginationLimit extracts pagination limit from field arguments
func (ca *ComplexityAnalyzer) extractPaginationLimit(field *ast.Field) int {
	if field.Arguments == nil {
		return 0
	}

	for _, arg := range field.Arguments {
		argName := arg.Name.Value
		if argName == "limit" || argName == "first" || argName == "last" {
			if intValue, ok := arg.Value.(*ast.IntValue); ok {
				if limit := intValue.Value; limit != "" {
					// Parse the limit value
					var parsedLimit int
					fmt.Sscanf(limit, "%d", &parsedLimit)
					return parsedLimit
				}
			}
		}
	}

	return 0
}

// ComplexityReport provides detailed complexity analysis
type ComplexityReport struct {
	TotalComplexity   int            `json:"total_complexity"`
	MaxComplexity     int            `json:"max_complexity"`
	ExceedsLimit      bool           `json:"exceeds_limit"`
	FieldComplexities map[string]int `json:"field_complexities"`
	RelationshipCount int            `json:"relationship_count"`
	ListFieldCount    int            `json:"list_field_count"`
	MaxDepth          int            `json:"max_depth"`
	Recommendations   []string       `json:"recommendations"`
}

// GenerateReport creates a detailed complexity report
func (ca *ComplexityAnalyzer) GenerateReport(query *ast.Document, schema *graphql.Schema) (*ComplexityReport, error) {
	totalComplexity, err := ca.AnalyzeQuery(query, schema)
	if err != nil {
		return nil, err
	}

	report := &ComplexityReport{
		TotalComplexity:   totalComplexity,
		MaxComplexity:     ca.maxComplexity,
		ExceedsLimit:      totalComplexity > ca.maxComplexity,
		FieldComplexities: make(map[string]int),
		Recommendations:   make([]string, 0),
	}

	// Analyze individual fields
	for _, def := range query.Definitions {
		if opDef, ok := def.(*ast.OperationDefinition); ok {
			ca.analyzeReportFields(opDef.SelectionSet, 0, report, schema)
		}
	}

	// Generate recommendations
	report.generateRecommendations()

	return report, nil
}

// analyzeReportFields recursively analyzes fields for the report
func (ca *ComplexityAnalyzer) analyzeReportFields(selectionSet *ast.SelectionSet, depth int, report *ComplexityReport, schema *graphql.Schema) {
	if selectionSet == nil {
		return
	}

	for _, selection := range selectionSet.Selections {
		if field, ok := selection.(*ast.Field); ok {
			fieldName := field.Name.Value
			fieldComplexity := ca.calculateFieldComplexity(field, depth, schema)
			report.FieldComplexities[fieldName] = fieldComplexity

			if ca.isRelationshipField(fieldName) {
				report.RelationshipCount++
			}
			if ca.isListField(fieldName, schema) {
				report.ListFieldCount++
			}

			if depth > report.MaxDepth {
				report.MaxDepth = depth
			}

			// Recursively analyze nested fields
			ca.analyzeReportFields(field.SelectionSet, depth+1, report, schema)
		}
	}
}

// generateRecommendations adds suggestions to reduce query complexity
func (r *ComplexityReport) generateRecommendations() {
	if r.ExceedsLimit {
		r.Recommendations = append(r.Recommendations, "Query exceeds maximum complexity limit")
	}

	if r.MaxDepth > 5 {
		r.Recommendations = append(r.Recommendations, "Consider reducing query depth by using fragments or multiple queries")
	}

	if r.ListFieldCount > 3 {
		r.Recommendations = append(r.Recommendations, "Consider adding pagination limits to list fields")
	}

	if r.RelationshipCount > 5 {
		r.Recommendations = append(r.Recommendations, "Consider reducing the number of relationship traversals")
	}

	// Find the most expensive fields
	maxFieldComplexity := 0
	var expensiveField string
	for field, complexity := range r.FieldComplexities {
		if complexity > maxFieldComplexity {
			maxFieldComplexity = complexity
			expensiveField = field
		}
	}

	if maxFieldComplexity > 50 {
		r.Recommendations = append(r.Recommendations,
			fmt.Sprintf("Field '%s' has high complexity (%d) - consider optimizing or caching",
				expensiveField, maxFieldComplexity))
	}
}

// GetComplexityWithValidation returns complexity and validation error if any
func (ca *ComplexityAnalyzer) GetComplexityWithValidation(query *ast.Document, schema *graphql.Schema) (int, error) {
	complexity, err := ca.AnalyzeQuery(query, schema)
	if err != nil {
		return 0, err
	}

	if complexity > ca.maxComplexity {
		return complexity, fmt.Errorf("query complexity %d exceeds maximum allowed complexity %d", complexity, ca.maxComplexity)
	}

	return complexity, nil
}

// Example usage:
//
// ```go
// // Configure complexity analysis
// complexityConfig := ComplexityConfig{
//     MaxComplexity: 1000,
//     Rules: DefaultComplexityRules(),
//     FieldComplexity: map[string]int{
//         "expensiveField": 100,
//         "simpleField": 1,
//     },
//     Enabled: true,
// }
//
// analyzer := NewComplexityAnalyzer(complexityConfig)
//
// // Validate query before execution
// if err := analyzer.ValidateComplexity(queryDoc, schema); err != nil {
//     return fmt.Errorf("query too complex: %w", err)
// }
//
// // Generate detailed report
// report, err := analyzer.GenerateReport(queryDoc, schema)
// if err != nil {
//     return err
// }
//
// log.Printf("Query complexity: %d/%d", report.TotalComplexity, report.MaxComplexity)
// ```
