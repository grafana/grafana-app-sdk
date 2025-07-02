package security

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/graphql-go/graphql"
)

// SecurityConfig holds configuration for the entire security system
type SecurityConfig struct {
	RBAC    RBACConfig
	Logging SecurityLoggingConfig
}

// SecurityLoggingConfig configures security audit logging
type SecurityLoggingConfig struct {
	// EnableAuditLog enables detailed audit logging
	EnableAuditLog bool

	// LogLevel sets the minimum log level for security events
	LogLevel string

	// LogFailedAttempts controls whether failed permission checks are logged
	LogFailedAttempts bool

	// LogSuccessfulAccess controls whether successful access is logged
	LogSuccessfulAccess bool
}

// SecureGraphQLGateway wraps the GraphQL gateway with security features
type SecureGraphQLGateway struct {
	schema            *graphql.Schema
	rbacMiddleware    *GraphQLRBACMiddleware
	contextExtractor  *GraphQLContextExtractor
	permissionChecker PermissionChecker
	logger            logging.Logger
	config            SecurityConfig
}

// NewSecureGraphQLGateway creates a new secure GraphQL gateway
func NewSecureGraphQLGateway(
	schema *graphql.Schema,
	logger logging.Logger,
	config SecurityConfig,
) (*SecureGraphQLGateway, error) {
	// Create permission checker
	permissionChecker := NewDefaultRBACPermissionChecker(config.RBAC, logger)

	// Create RBAC middleware
	rbacMiddleware := NewGraphQLRBACMiddleware(permissionChecker, logger, config.RBAC)

	// Create context extractor
	contextExtractor := NewGraphQLContextExtractor(logger)

	gateway := &SecureGraphQLGateway{
		schema:            schema,
		rbacMiddleware:    rbacMiddleware,
		contextExtractor:  contextExtractor,
		permissionChecker: permissionChecker,
		logger:            logger,
		config:            config,
	}

	// Apply RBAC middleware to schema
	if err := rbacMiddleware.WrapSchema(schema); err != nil {
		return nil, fmt.Errorf("failed to apply RBAC middleware: %w", err)
	}

	return gateway, nil
}

// HandleGraphQLRequest handles a GraphQL request with security checks
func (g *SecureGraphQLGateway) HandleGraphQLRequest(
	ctx context.Context,
	query string,
	variables map[string]interface{},
	pluginCtx backend.PluginContext,
) (*graphql.Result, error) {
	// Extract and add security context
	ctx = g.contextExtractor.ExtractSecurityContext(ctx, pluginCtx)
	ctx = context.WithValue(ctx, "pluginContext", pluginCtx)

	// Log the request
	user := ExtractUserFromPluginContext(pluginCtx)
	g.logger.Debug("GraphQL request received",
		"user", user.Name,
		"role", user.Role,
		"query_length", len(query),
	)

	// Execute the GraphQL query
	result := graphql.Do(graphql.Params{
		Schema:         *g.schema,
		RequestString:  query,
		VariableValues: variables,
		Context:        ctx,
	})

	// Log any errors
	if len(result.Errors) > 0 {
		g.logger.Warn("GraphQL request completed with errors",
			"user", user.Name,
			"error_count", len(result.Errors),
		)

		// Check if errors are permission-related
		for _, err := range result.Errors {
			if isPermissionError(err) {
				g.logger.Warn("Permission error in GraphQL request",
					"user", user.Name,
					"error", err.Message,
				)
			}
		}
	}

	return result, nil
}

// CreateHTTPHandler creates an HTTP handler for GraphQL requests
func (g *SecureGraphQLGateway) CreateHTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This is a simplified HTTP handler - in practice you'd want to:
		// 1. Parse the GraphQL request from HTTP body
		// 2. Extract plugin context from HTTP headers/cookies
		// 3. Handle CORS
		// 4. Return proper HTTP status codes

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null, "errors": [{"message": "HTTP handler not fully implemented"}]}`))
	}
}

// isPermissionError checks if a GraphQL error is permission-related
func isPermissionError(err error) bool {
	return err != nil && (err.Error() == "authentication required" ||
		err.Error()[:13] == "access denied" ||
		err.Error()[:16] == "permission denied")
}

// ValidationRules provides GraphQL validation rules with security considerations
func (g *SecureGraphQLGateway) ValidationRules() []graphql.ValidationRuleFn {
	// Note: This is a placeholder for custom validation rules
	// In practice, you'd implement proper depth limiting, complexity analysis, etc.
	return []graphql.ValidationRuleFn{}
}

// GetDefaultSecurityConfig returns a sensible default security configuration
func GetDefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		RBAC: GetDefaultRBACConfig(),
		Logging: SecurityLoggingConfig{
			EnableAuditLog:      true,
			LogLevel:            "info",
			LogFailedAttempts:   true,
			LogSuccessfulAccess: false, // Set to true for detailed audit trails
		},
	}
}

// Example configuration for different environments
func GetDevelopmentSecurityConfig() SecurityConfig {
	config := GetDefaultSecurityConfig()
	config.RBAC.DefaultPolicy = "allow"       // More permissive in development
	config.Logging.LogSuccessfulAccess = true // More verbose logging
	return config
}

func GetProductionSecurityConfig() SecurityConfig {
	config := GetDefaultSecurityConfig()
	config.RBAC.DefaultPolicy = "deny" // Stricter in production
	config.Logging.LogFailedAttempts = true
	return config
}

// ExampleUsage demonstrates how to integrate the security system
func ExampleUsage() {
	// Create logger using slog with proper handler
	logger := logging.NewSLogLogger(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create your GraphQL schema (this would be your actual schema)
	schema, _ := graphql.NewSchema(graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name: "Query",
			Fields: graphql.Fields{
				"hello": &graphql.Field{
					Type: graphql.String,
					Resolve: func(p graphql.ResolveParams) (interface{}, error) {
						return "Hello World!", nil
					},
				},
			},
		}),
	})

	// Create secure gateway
	securityConfig := GetDefaultSecurityConfig()
	gateway, err := NewSecureGraphQLGateway(&schema, logger, securityConfig)
	if err != nil {
		logger.Error("Failed to create secure gateway", "error", err)
		return
	}

	// Use the gateway
	ctx := context.Background()
	pluginCtx := backend.PluginContext{
		User: &backend.User{
			Name:  "test-user",
			Login: "test-user",
			Email: "test@example.com",
			Role:  "Editor",
		},
	}

	result, err := gateway.HandleGraphQLRequest(ctx, "{ hello }", nil, pluginCtx)
	if err != nil {
		logger.Error("GraphQL request failed", "error", err)
		return
	}

	logger.Info("GraphQL request completed", "result", result.Data)
}

// Custom permission checker example
type CustomPermissionChecker struct {
	*DefaultRBACPermissionChecker
	customRules map[string]func(User, string, string) bool
}

func NewCustomPermissionChecker(config RBACConfig, logger logging.Logger) *CustomPermissionChecker {
	return &CustomPermissionChecker{
		DefaultRBACPermissionChecker: NewDefaultRBACPermissionChecker(config, logger),
		customRules:                  make(map[string]func(User, string, string) bool),
	}
}

func (c *CustomPermissionChecker) AddCustomRule(name string, rule func(User, string, string) bool) {
	c.customRules[name] = rule
}

func (c *CustomPermissionChecker) CheckFieldPermission(ctx context.Context, user User, resourceType, fieldName string) error {
	// First check custom rules
	for name, rule := range c.customRules {
		if !rule(user, resourceType, fieldName) {
			return fmt.Errorf("custom rule %s denied access", name)
		}
	}

	// Then check default RBAC rules
	return c.DefaultRBACPermissionChecker.CheckFieldPermission(ctx, user, resourceType, fieldName)
}
