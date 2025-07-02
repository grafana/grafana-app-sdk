package security

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/graphql-go/graphql"
)

// GraphQLRBACMiddleware wraps GraphQL resolvers with RBAC permission checks
type GraphQLRBACMiddleware struct {
	permissionChecker PermissionChecker
	logger            logging.Logger
	config            RBACConfig
}

// NewGraphQLRBACMiddleware creates a new GraphQL RBAC middleware
func NewGraphQLRBACMiddleware(permissionChecker PermissionChecker, logger logging.Logger, config RBACConfig) *GraphQLRBACMiddleware {
	return &GraphQLRBACMiddleware{
		permissionChecker: permissionChecker,
		logger:            logger,
		config:            config,
	}
}

// WrapSchema wraps all fields in a GraphQL schema with RBAC checks
func (m *GraphQLRBACMiddleware) WrapSchema(schema *graphql.Schema) error {
	if !m.config.Enabled {
		return nil
	}

	m.logger.Info("Applying RBAC middleware to GraphQL schema")

	// Wrap Query type fields
	if schema.QueryType() != nil {
		m.wrapObjectFields(schema.QueryType(), "Query")
	}

	// Wrap Mutation type fields
	if schema.MutationType() != nil {
		m.wrapObjectFields(schema.MutationType(), "Mutation")
	}

	// Wrap Subscription type fields
	if schema.SubscriptionType() != nil {
		m.wrapObjectFields(schema.SubscriptionType(), "Subscription")
	}

	return nil
}

// wrapObjectFields wraps all fields in a GraphQL object type
func (m *GraphQLRBACMiddleware) wrapObjectFields(objType *graphql.Object, typeName string) {
	for fieldName, field := range objType.Fields() {
		originalResolver := field.Resolve
		if originalResolver == nil {
			continue
		}

		// Wrap the resolver with RBAC check
		field.Resolve = m.wrapResolver(typeName, fieldName, originalResolver)
	}
}

// wrapResolver wraps a single GraphQL resolver with RBAC permission check
func (m *GraphQLRBACMiddleware) wrapResolver(
	typeName, fieldName string,
	originalResolver graphql.FieldResolveFn,
) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		// Extract user from context
		user, hasUser := GetUser(p.Context)
		if !hasUser {
			// Try to extract from plugin context if available
			if pluginCtx, ok := p.Context.Value("pluginContext").(backend.PluginContext); ok {
				user = ExtractUserFromPluginContext(pluginCtx)
			} else {
				m.logger.Warn("No user found in GraphQL context", "type", typeName, "field", fieldName)
				return nil, fmt.Errorf("authentication required")
			}
		}

		// Check field permission
		resourceType := m.inferResourceType(typeName, fieldName, p)
		if err := m.permissionChecker.CheckFieldPermission(p.Context, user, resourceType, fieldName); err != nil {
			m.logger.Warn("GraphQL field access denied",
				"user", user.Name,
				"type", typeName,
				"field", fieldName,
				"resource", resourceType,
				"error", err,
			)

			// Return appropriate error based on policy
			if m.config.DefaultPolicy == "deny" {
				return nil, fmt.Errorf("access denied to field %s.%s", resourceType, fieldName)
			}

			// For "allow" policy, we might return a filtered/redacted response
			return m.getRedactedResponse(typeName, fieldName), nil
		}

		// Add security audit logging
		m.logger.Debug("GraphQL field access granted",
			"user", user.Name,
			"type", typeName,
			"field", fieldName,
			"resource", resourceType,
		)

		// Call original resolver
		result, err := originalResolver(p)

		// Post-process result for additional security checks
		if err == nil {
			result = m.postProcessResult(user, resourceType, fieldName, result)
		}

		return result, err
	}
}

// inferResourceType attempts to infer the resource type from the GraphQL context
func (m *GraphQLRBACMiddleware) inferResourceType(typeName, fieldName string, p graphql.ResolveParams) string {
	// Check if there's explicit resource type in args
	if resourceType, ok := p.Args["resourceType"].(string); ok {
		return resourceType
	}

	// Check if there's a parent object that indicates resource type
	if p.Source != nil {
		sourceType := reflect.TypeOf(p.Source)
		if sourceType.Kind() == reflect.Ptr {
			sourceType = sourceType.Elem()
		}
		if sourceType.Kind() == reflect.Struct {
			return sourceType.Name()
		}
	}

	// Try to infer from field name patterns
	resourceType := m.inferResourceFromFieldName(fieldName)
	if resourceType != "" {
		return resourceType
	}

	// Default to using the GraphQL type name
	return typeName
}

// inferResourceFromFieldName infers resource type from common field naming patterns
func (m *GraphQLRBACMiddleware) inferResourceFromFieldName(fieldName string) string {
	// Common patterns in GraphQL field names
	patterns := map[string]string{
		"dashboard":  "dashboard",
		"playlist":   "playlist",
		"issue":      "issue",
		"user":       "user",
		"folder":     "folder",
		"datasource": "datasource",
		"plugin":     "plugin",
	}

	fieldLower := strings.ToLower(fieldName)
	for pattern, resource := range patterns {
		if strings.Contains(fieldLower, pattern) {
			return resource
		}
	}

	return ""
}

// postProcessResult applies additional security filtering to resolver results
func (m *GraphQLRBACMiddleware) postProcessResult(user User, resourceType, fieldName string, result interface{}) interface{} {
	// For sensitive fields, we might want to redact or filter certain data
	if m.isSensitiveField(resourceType, fieldName) {
		return m.redactSensitiveData(user, result)
	}

	return result
}

// isSensitiveField checks if a field contains sensitive information
func (m *GraphQLRBACMiddleware) isSensitiveField(resourceType, fieldName string) bool {
	sensitivePatterns := []string{
		"password", "secret", "token", "key", "credential",
		"private", "internal", "admin", "system",
	}

	fieldLower := strings.ToLower(fieldName)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(fieldLower, pattern) {
			return true
		}
	}

	return false
}

// redactSensitiveData redacts sensitive information based on user permissions
func (m *GraphQLRBACMiddleware) redactSensitiveData(user User, result interface{}) interface{} {
	// For non-admin users, redact sensitive fields
	if !user.IsAdmin {
		// This is a simplified example - in practice you'd want more sophisticated redaction
		if resultMap, ok := result.(map[string]interface{}); ok {
			redacted := make(map[string]interface{})
			for k, v := range resultMap {
				if m.isSensitiveField("", k) {
					redacted[k] = "[REDACTED]"
				} else {
					redacted[k] = v
				}
			}
			return redacted
		}
	}

	return result
}

// getRedactedResponse returns a safe redacted response for denied fields
func (m *GraphQLRBACMiddleware) getRedactedResponse(typeName, fieldName string) interface{} {
	// Return appropriate null/empty response based on field type
	// This is a simplified implementation
	return nil
}

// GraphQLContextExtractor extracts security context from HTTP requests
type GraphQLContextExtractor struct {
	logger logging.Logger
}

// NewGraphQLContextExtractor creates a new context extractor
func NewGraphQLContextExtractor(logger logging.Logger) *GraphQLContextExtractor {
	return &GraphQLContextExtractor{
		logger: logger,
	}
}

// ExtractSecurityContext extracts security information and adds it to GraphQL context
func (e *GraphQLContextExtractor) ExtractSecurityContext(ctx context.Context, req interface{}) context.Context {
	// Try to extract plugin context
	if pluginCtx, ok := ctx.Value("pluginContext").(backend.PluginContext); ok {
		user := ExtractUserFromPluginContext(pluginCtx)

		secCtx := SecurityContext{
			User:        user,
			RequestTime: time.Now(),
			// ClientIP could be extracted from HTTP headers if available
		}

		ctx = WithUser(ctx, user)
		ctx = WithSecurityContext(ctx, secCtx)

		e.logger.Debug("Security context extracted for GraphQL request",
			"user", user.Name,
			"role", user.Role,
		)
	}

	return ctx
}
