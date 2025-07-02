# GraphQL Federation RBAC Security System

A comprehensive Role-Based Access Control (RBAC) system for GraphQL Federation that provides field-level security for the Grafana App Platform.

## Features

- 🔒 **Field-Level Permissions**: Control access to individual GraphQL fields
- 👥 **Role-Based Access**: Admin, Editor, Viewer roles with granular permissions
- 👤 **User-Specific Permissions**: Override role permissions for specific users
- 🔑 **Group-Based Permissions**: Assign permissions based on user groups
- 🛡️ **Data Redaction**: Automatically redact sensitive information
- 📊 **Security Audit Logging**: Comprehensive logging of access attempts
- ⚡ **High Performance**: Minimal overhead permission checking
- 🧪 **Fully Tested**: Comprehensive test suite included

## Quick Start

### 1. Basic Setup

```go
package main

import (
    "log/slog"
    "os"

    "github.com/grafana/grafana-app-sdk/graphql/security"
    "github.com/grafana/grafana-app-sdk/logging"
)

func main() {
    // Create logger
    logger := logging.NewSLogLogger(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
    
    // Get default security configuration
    securityConfig := security.GetDefaultSecurityConfig()
    
    // Create your GraphQL schema
    schema := createYourGraphQLSchema()
    
    // Create secure gateway
    gateway, err := security.NewSecureGraphQLGateway(&schema, logger, securityConfig)
    if err != nil {
        panic(err)
    }
    
    // Use the gateway for requests
    ctx := context.Background()
    pluginCtx := backend.PluginContext{
        User: &backend.User{
            Name:  "john-doe",
            Login: "john",
            Email: "john@example.com",
            Role:  "Editor",
        },
    }
    
    result, err := gateway.HandleGraphQLRequest(ctx, "{ dashboard { id title } }", nil, pluginCtx)
    // ... handle result
}
```

### 2. Custom Permission Configuration

```go
// Create custom RBAC configuration
customConfig := security.RBACConfig{
    Enabled:       true,
    DefaultPolicy: "deny",  // or "allow"
    AdminBypass:   true,    // Admins bypass all checks
    
    // Role-based permissions
    RoleBasedPermissions: map[string]security.RolePermissions{
        "data_analyst": {
            Resources: map[string][]string{
                "dashboard": {"read"},
                "query":     {"read", "execute"},
            },
            Fields: map[string][]string{
                "dashboard.metrics": {"read"},
                "query.results":     {"read"},
            },
            DeniedFields: []string{
                "user.password",
                "*.secret",
            },
        },
    },
    
    // User-specific permissions
    UserSpecificPermissions: map[string]security.UserPermissions{
        "senior-dev": {
            AdditionalPermissions: []string{
                "debug.logs",
                "system.metrics",
            },
        },
    },
    
    // Group-based permissions  
    GroupBasedPermissions: map[string]security.GroupPermissions{
        "dev-team": {
            Resources: map[string][]string{
                "debug": {"read"},
            },
            Fields: map[string][]string{
                "debug.trace": {"read"},
            },
        },
    },
}
```

## Default Role Permissions

### Admin Role
- **Resources**: Full access to all resources (`*: *`)
- **Fields**: Read/write access to all fields (`*.*: read, write`)
- **Bypass**: Bypasses all permission checks when `AdminBypass: true`

### Editor Role
- **Resources**: Read/write access to dashboards, playlists, issues
- **Fields**: Read/write access to dashboard.*, playlist.*, issue.* fields
- **Restrictions**: Denied access to user.password, *.sensitive fields

### Viewer Role  
- **Resources**: Read-only access to dashboards, playlists, issues
- **Fields**: Read-only access to dashboard.*, playlist.*, issue.* fields
- **Restrictions**: Denied access to user.password, *.sensitive, *.secret fields

## Security Features

### Field-Level Access Control

```go
// The system automatically checks permissions for each GraphQL field
query := `{
  dashboard(id: "123") {
    id          # ✅ Allowed for viewers
    title       # ✅ Allowed for viewers  
    secret      # ❌ Denied for non-admins
  }
  user {
    name        # ✅ Allowed for viewers
    password    # ❌ Denied for all non-admins
  }
}`
```

### Data Redaction

```go
// Sensitive fields are automatically redacted
result := `{
  "user": {
    "name": "john-doe",
    "password": "[REDACTED]"  // Automatically redacted for non-admins
  }
}`
```

### Security Context

```go
// Extract security information from requests
ctx := context.Background()
user := security.User{Name: "john", Role: "editor"}
ctx = security.WithUser(ctx, user)

secCtx := security.SecurityContext{
    User:        user,
    Permissions: []string{"dashboard.read", "playlist.write"},
    RequestTime: time.Now(),
    ClientIP:    "192.168.1.100",
}
ctx = security.WithSecurityContext(ctx, secCtx)
```

## Integration with Existing Gateway

The RBAC system integrates seamlessly with your existing GraphQL gateway:

```go
// Apply RBAC middleware to your schema
rbacMiddleware := security.NewGraphQLRBACMiddleware(permissionChecker, logger, config.RBAC)
err := rbacMiddleware.WrapSchema(yourExistingSchema)
```

## Custom Permission Checkers

Extend the system with custom business logic:

```go
customChecker := security.NewCustomPermissionChecker(config, logger)

// Add custom rule
customChecker.AddCustomRule("business_hours", func(user security.User, resourceType, fieldName string) bool {
    // Only allow access during business hours for certain users
    hour := time.Now().Hour()
    return hour >= 9 && hour <= 17
})

// Add time-based rule
customChecker.AddCustomRule("weekend_restriction", func(user security.User, resourceType, fieldName string) bool {
    // Restrict sensitive operations on weekends
    if strings.Contains(fieldName, "sensitive") {
        return time.Now().Weekday() != time.Saturday && time.Now().Weekday() != time.Sunday
    }
    return true
})
```

## Environment-Specific Configurations

### Development
```go
devConfig := security.GetDevelopmentSecurityConfig()
// - More permissive (DefaultPolicy: "allow")
// - Verbose logging enabled
// - All access attempts logged
```

### Production
```go
prodConfig := security.GetProductionSecurityConfig()
// - Strict security (DefaultPolicy: "deny") 
// - Failed attempts logged
// - Optimized performance
```

## Security Audit Logging

The system provides comprehensive audit logging:

```json
{
  "level": "warn",
  "msg": "GraphQL field access denied",
  "user": "john-doe",
  "type": "Query", 
  "field": "sensitiveData",
  "resource": "dashboard",
  "error": "permission denied: insufficient role"
}
```

## Testing

Run the comprehensive test suite:

```bash
cd graphql/security
go test -v
```

The tests cover:
- Default role permissions
- Custom permission configurations  
- User extraction from plugin context
- Security context handling
- Performance benchmarks

## Performance

The RBAC system is designed for high performance:
- Minimal memory allocation during permission checks
- Fast hash map lookups for role/user permissions
- Configurable logging levels to reduce overhead
- Benchmarks included to monitor performance

## Security Best Practices

1. **Use Principle of Least Privilege**: Start with `DefaultPolicy: "deny"`
2. **Regular Permission Audits**: Use `GetUserPermissions()` to audit user access
3. **Monitor Failed Attempts**: Enable `LogFailedAttempts: true` in production
4. **Validate Custom Rules**: Test custom permission checkers thoroughly
5. **Secure Logging**: Ensure audit logs are stored securely
6. **Regular Updates**: Keep the RBAC configuration updated as roles change

## Troubleshooting

### Common Issues

**Q: Permission always denied even for admins**  
A: Check that `AdminBypass: true` is set in your RBAC configuration.

**Q: Custom permissions not working**  
A: Ensure field paths use the format `resourceType.fieldName` (e.g., `dashboard.title`).

**Q: Tests failing with nil pointer**  
A: Make sure to create logger with proper handler: `logging.NewSLogLogger(slog.NewTextHandler(...))`

### Debug Mode

Enable debug logging to see detailed permission checks:

```go
logger := logging.NewSLogLogger(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
```

## Contributing

When extending the RBAC system:

1. Add comprehensive tests for new features
2. Update this README with new configuration options
3. Ensure backward compatibility
4. Follow the existing code patterns
5. Add appropriate logging for security events

## License

This RBAC system is part of the Grafana App SDK and follows the same licensing terms. 
