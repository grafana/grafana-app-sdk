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

### 0. Try the Demo First

Before integrating, see the RBAC system in action:

```bash
# Command line demo - see permission enforcement in action
cd graphql/security/demo
go run main.go

# Interactive web demo - test different queries and roles
# Visit http://localhost:8080 after running the above command
```

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

### Quick Integration

The RBAC system integrates seamlessly with your existing GraphQL Federation:

```go
import "github.com/grafana/grafana-app-sdk/graphql/security"

// Replace your existing GraphQL handler
func setupSecureGraphQLGateway() {
    // 1. Get your existing federated schema
    registry := gateway.AutoDiscovery(playlistProvider, investigationsProvider)
    federatedSchema := registry.GetFederatedSchema()
    
    // 2. Set up security config
    securityConfig := security.GetProductionSecurityConfig()
    
    // 3. Create secure gateway 
    secureGateway, err := security.NewSecureGraphQLGateway(
        federatedSchema, 
        logger, 
        securityConfig,
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // 4. Use secure gateway in your HTTP handler
    http.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
        // Extract user from your authentication system
        pluginCtx := backend.PluginContext{
            User: &backend.User{
                Name:  getUserName(r),
                Role:  getUserRole(r), // "Admin", "Editor", "Viewer"
                Email: getUserEmail(r),
            },
        }
        
        // Parse GraphQL request
        var gqlRequest struct {
            Query     string                 `json:"query"`
            Variables map[string]interface{} `json:"variables"`
        }
        json.NewDecoder(r.Body).Decode(&gqlRequest)
        
        // Execute with RBAC protection
        result, err := secureGateway.HandleGraphQLRequest(
            r.Context(), 
            gqlRequest.Query, 
            gqlRequest.Variables, 
            pluginCtx,
        )
        
        json.NewEncoder(w).Encode(result)
    })
}
```

### Manual Integration

For more control, apply RBAC middleware directly:

```go
// Apply RBAC middleware to your schema
rbacMiddleware := security.NewGraphQLRBACMiddleware(permissionChecker, logger, config.RBAC)
err := rbacMiddleware.WrapSchema(yourExistingSchema)
```

### Testing Your Integration

Once integrated, test with different user roles:

```bash
# Test as Admin (full access)
curl -X POST http://localhost:8080/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin-token" \
  -d '{"query": "{ dashboard { id title sensitiveData } }"}'

# Test as Viewer (limited access)  
curl -X POST http://localhost:8080/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer viewer-token" \
  -d '{"query": "{ dashboard { id title sensitiveData } }"}'
```

Expected results:
- **Admin**: Gets all fields including `sensitiveData`
- **Viewer**: Gets `id` and `title` but `sensitiveData` returns access denied error

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

### Unit Tests

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

### Live Demo

#### Command Line Demo

Test the RBAC system with a practical demonstration:

```bash
cd graphql/security/demo
go run main.go
```

This will run 4 test scenarios showing different permission levels:

```
--- Test 1: Admin User - Full Access ---
[SUCCESS] Successful Response:
{
  "dashboard": {
    "id": "123",
    "sensitiveData": "SECRET_API_KEY_12345",  # ✅ Admin sees sensitive data
    "title": "Sample Dashboard"
  },
  "user": {
    "name": "John Doe", 
    "password": "super_secret_password"      # ✅ Admin sees passwords
  }
}

--- Test 2: Editor User - Limited Access ---
[ERRORS] GraphQL Errors:
  [DENIED] Access Denied: access denied to field user.user  # 🔒 Editor blocked
[SUCCESS] Successful Response:
{
  "dashboard": {...},     # ✅ Editor can access dashboards
  "user": null           # 🔒 But user data is blocked
}

--- Test 3: Viewer User - Read Only ---
[SUCCESS] Successful Response:
{
  "dashboard": {         # ✅ Viewer can read dashboards
    "id": "123",
    "title": "Sample Dashboard"
  },
  "playlist": {...}      # ✅ And playlists
}

--- Test 4: Viewer User - Denied Sensitive Data ---
[DENIED] Should be denied access to sensitive data  # 🔒 Sensitive fields blocked
```

You'll also see detailed security audit logs:

```json
{"level":"WARN","msg":"GraphQL field access denied","user":"editor","type":"Query","field":"user","resource":"user","error":"access denied to field user.user"}
{"level":"INFO","msg":"Access denied - no matching permissions","user":"viewer","resource":"dashboard","field":"sensitiveData"}
```

#### Interactive Web Demo

For hands-on testing with a web interface:

```bash
cd graphql/security/demo
go run main.go
```

Then visit **http://localhost:8080** in your browser.

**Features:**
- Select different user roles (Admin, Editor, Viewer)
- Try pre-built example queries
- Test your own custom queries
- See real-time permission enforcement
- View detailed security logs in browser console

**Example Queries to Test:**

```graphql
# Safe query - all roles can access
{ dashboard(id: "123") { id title } }

# Sensitive query - only Admin can access
{ dashboard(id: "123") { id title sensitiveData } }

# Restricted query - only Admin can access 
{ user { name password } }

# Complex query - mixed permissions
{ 
  dashboard(id: "123") { id title sensitiveData }
  playlist { name items }
  user { name password }
}
```

**What You'll See:**
- **Admin role**: Full access to all fields including sensitive data
- **Editor role**: Access to dashboards/playlists but blocked from user passwords
- **Viewer role**: Read-only access, blocked from all sensitive fields

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

**Q: Demo shows weird characters (emojis)**  
A: Your terminal doesn't support emoji rendering. The functionality works correctly - just ignore the display issues.

**Q: Demo server won't start - "address already in use"**  
A: Port 8080 is already in use. Either kill the existing process or modify the demo to use a different port.

**Q: All users have the same permissions in demo**  
A: Check that you're changing the user role in the web interface dropdown or passing the correct role in the X-User-Role header.

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

## Files Overview

The RBAC security system consists of:

- **`rbac.go`** - Core permission checking logic and RBAC configuration
- **`middleware.go`** - GraphQL middleware that wraps resolvers with security checks  
- **`integration.go`** - Complete secure gateway and integration helpers
- **`rbac_test.go`** - Comprehensive test suite with all permission scenarios
- **`README.md`** - This documentation
- **`demo/main.go`** - Interactive demo for testing and learning

## License

This RBAC system is part of the Grafana App SDK and follows the same licensing terms. 
