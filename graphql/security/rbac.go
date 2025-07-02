package security

import (
	"context"
	"fmt"
	"time"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// User represents a user in the GraphQL context
type User struct {
	Name     string
	Email    string
	Login    string
	Role     string
	Groups   []string
	IsAdmin  bool
	IsEditor bool
	IsViewer bool
}

// PermissionChecker defines the interface for checking field-level permissions
type PermissionChecker interface {
	// CheckFieldPermission checks if a user has permission to access a specific field
	CheckFieldPermission(ctx context.Context, user User, resourceType, fieldName string) error

	// CheckResourcePermission checks if a user has permission to access a resource
	CheckResourcePermission(ctx context.Context, user User, resourceType, action string) error

	// GetUserPermissions returns all permissions for a user
	GetUserPermissions(ctx context.Context, user User) ([]string, error)
}

// RBACConfig configures the RBAC system
type RBACConfig struct {
	// Enabled controls whether RBAC is active
	Enabled bool

	// DefaultPolicy defines the default action when no explicit permission is found
	// "allow" or "deny"
	DefaultPolicy string

	// AdminBypass allows admins to bypass all permission checks
	AdminBypass bool

	// RoleBasedPermissions maps roles to resource and field permissions
	RoleBasedPermissions map[string]RolePermissions

	// UserSpecificPermissions maps specific users to permissions
	UserSpecificPermissions map[string]UserPermissions

	// GroupBasedPermissions maps groups to permissions
	GroupBasedPermissions map[string]GroupPermissions
}

// RolePermissions defines permissions for a role
type RolePermissions struct {
	// Resources maps resource types to allowed actions
	Resources map[string][]string

	// Fields maps resource.field to allowed actions
	Fields map[string][]string

	// DeniedFields explicitly denies access to specific fields
	DeniedFields []string
}

// UserPermissions defines permissions for a specific user
type UserPermissions struct {
	// Additional permissions beyond role-based permissions
	AdditionalPermissions []string

	// Denied permissions that override role-based permissions
	DeniedPermissions []string
}

// GroupPermissions defines permissions for a group
type GroupPermissions struct {
	// Resources maps resource types to allowed actions
	Resources map[string][]string

	// Fields maps resource.field to allowed actions
	Fields map[string][]string
}

// DefaultRBACPermissionChecker implements PermissionChecker
type DefaultRBACPermissionChecker struct {
	config RBACConfig
	logger logging.Logger
}

// NewDefaultRBACPermissionChecker creates a new RBAC permission checker
func NewDefaultRBACPermissionChecker(config RBACConfig, logger logging.Logger) *DefaultRBACPermissionChecker {
	if config.DefaultPolicy == "" {
		config.DefaultPolicy = "deny" // Secure by default
	}

	return &DefaultRBACPermissionChecker{
		config: config,
		logger: logger,
	}
}

// CheckFieldPermission checks if a user has permission to access a specific field
func (r *DefaultRBACPermissionChecker) CheckFieldPermission(ctx context.Context, user User, resourceType, fieldName string) error {
	if !r.config.Enabled {
		return nil // RBAC disabled, allow all
	}

	// Admin bypass check
	if r.config.AdminBypass && user.IsAdmin {
		r.logger.Debug("Admin bypass enabled, allowing access",
			"user", user.Name, "resource", resourceType, "field", fieldName)
		return nil
	}

	// Check explicit denials first
	if r.isExplicitlyDenied(user, resourceType, fieldName) {
		r.logger.Info("Access denied - explicit denial",
			"user", user.Name, "resource", resourceType, "field", fieldName)
		return fmt.Errorf("access denied to field %s.%s", resourceType, fieldName)
	}

	// Check permissions in order: user-specific, group-based, role-based
	if r.hasUserSpecificPermission(user, resourceType, fieldName) {
		return nil
	}

	if r.hasGroupBasedPermission(user, resourceType, fieldName) {
		return nil
	}

	if r.hasRoleBasedPermission(user, resourceType, fieldName) {
		return nil
	}

	// Apply default policy
	if r.config.DefaultPolicy == "allow" {
		r.logger.Debug("Default policy allow, granting access",
			"user", user.Name, "resource", resourceType, "field", fieldName)
		return nil
	}

	r.logger.Info("Access denied - no matching permissions",
		"user", user.Name, "resource", resourceType, "field", fieldName)
	return fmt.Errorf("access denied to field %s.%s", resourceType, fieldName)
}

// CheckResourcePermission checks if a user has permission to access a resource
func (r *DefaultRBACPermissionChecker) CheckResourcePermission(ctx context.Context, user User, resourceType, action string) error {
	if !r.config.Enabled {
		return nil
	}

	if r.config.AdminBypass && user.IsAdmin {
		return nil
	}

	// Check role-based permissions
	if rolePerms, exists := r.config.RoleBasedPermissions[user.Role]; exists {
		if actions, exists := rolePerms.Resources[resourceType]; exists {
			for _, allowedAction := range actions {
				if allowedAction == action || allowedAction == "*" {
					return nil
				}
			}
		}
	}

	// Check group-based permissions
	for _, group := range user.Groups {
		if groupPerms, exists := r.config.GroupBasedPermissions[group]; exists {
			if actions, exists := groupPerms.Resources[resourceType]; exists {
				for _, allowedAction := range actions {
					if allowedAction == action || allowedAction == "*" {
						return nil
					}
				}
			}
		}
	}

	if r.config.DefaultPolicy == "allow" {
		return nil
	}

	return fmt.Errorf("access denied to %s %s", action, resourceType)
}

// GetUserPermissions returns all permissions for a user
func (r *DefaultRBACPermissionChecker) GetUserPermissions(ctx context.Context, user User) ([]string, error) {
	permissions := []string{}

	// Add role-based permissions
	if rolePerms, exists := r.config.RoleBasedPermissions[user.Role]; exists {
		for resource, actions := range rolePerms.Resources {
			for _, action := range actions {
				permissions = append(permissions, fmt.Sprintf("%s:%s", resource, action))
			}
		}
		for field, actions := range rolePerms.Fields {
			for _, action := range actions {
				permissions = append(permissions, fmt.Sprintf("field:%s:%s", field, action))
			}
		}
	}

	// Add group-based permissions
	for _, group := range user.Groups {
		if groupPerms, exists := r.config.GroupBasedPermissions[group]; exists {
			for resource, actions := range groupPerms.Resources {
				for _, action := range actions {
					permissions = append(permissions, fmt.Sprintf("%s:%s", resource, action))
				}
			}
		}
	}

	// Add user-specific permissions
	if userPerms, exists := r.config.UserSpecificPermissions[user.Name]; exists {
		permissions = append(permissions, userPerms.AdditionalPermissions...)
	}

	return permissions, nil
}

// Helper methods for permission checking

func (r *DefaultRBACPermissionChecker) isExplicitlyDenied(user User, resourceType, fieldName string) bool {
	// Check role-based denials
	if rolePerms, exists := r.config.RoleBasedPermissions[user.Role]; exists {
		fieldPath := fmt.Sprintf("%s.%s", resourceType, fieldName)
		for _, deniedField := range rolePerms.DeniedFields {
			if deniedField == fieldPath {
				return true
			}
		}
	}

	// Check user-specific denials
	if userPerms, exists := r.config.UserSpecificPermissions[user.Name]; exists {
		fieldPath := fmt.Sprintf("%s.%s", resourceType, fieldName)
		for _, deniedPerm := range userPerms.DeniedPermissions {
			if deniedPerm == fieldPath {
				return true
			}
		}
	}

	return false
}

func (r *DefaultRBACPermissionChecker) hasUserSpecificPermission(user User, resourceType, fieldName string) bool {
	userPerms, exists := r.config.UserSpecificPermissions[user.Name]
	if !exists {
		return false
	}

	fieldPath := fmt.Sprintf("%s.%s", resourceType, fieldName)
	for _, perm := range userPerms.AdditionalPermissions {
		if perm == fieldPath || perm == fmt.Sprintf("%s.*", resourceType) {
			return true
		}
	}

	return false
}

func (r *DefaultRBACPermissionChecker) hasGroupBasedPermission(user User, resourceType, fieldName string) bool {
	for _, group := range user.Groups {
		groupPerms, exists := r.config.GroupBasedPermissions[group]
		if !exists {
			continue
		}

		fieldPath := fmt.Sprintf("%s.%s", resourceType, fieldName)
		if actions, exists := groupPerms.Fields[fieldPath]; exists {
			if len(actions) > 0 { // If any actions are allowed
				return true
			}
		}

		// Check wildcard permissions
		if actions, exists := groupPerms.Fields[fmt.Sprintf("%s.*", resourceType)]; exists {
			if len(actions) > 0 {
				return true
			}
		}
	}

	return false
}

func (r *DefaultRBACPermissionChecker) hasRoleBasedPermission(user User, resourceType, fieldName string) bool {
	rolePerms, exists := r.config.RoleBasedPermissions[user.Role]
	if !exists {
		return false
	}

	fieldPath := fmt.Sprintf("%s.%s", resourceType, fieldName)

	// Check specific field permissions
	if actions, exists := rolePerms.Fields[fieldPath]; exists {
		return len(actions) > 0
	}

	// Check wildcard permissions
	if actions, exists := rolePerms.Fields[fmt.Sprintf("%s.*", resourceType)]; exists {
		return len(actions) > 0
	}

	return false
}

// Context keys for storing security information
type userContextKey struct{}
type securityContextKey struct{}

// SecurityContext holds security-related information in the GraphQL context
type SecurityContext struct {
	User        User
	Permissions []string
	RequestTime time.Time
	ClientIP    string
}

// WithUser adds user information to the context
func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

// GetUser extracts user information from the context
func GetUser(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)
	return user, ok
}

// WithSecurityContext adds security context to the GraphQL context
func WithSecurityContext(ctx context.Context, secCtx SecurityContext) context.Context {
	return context.WithValue(ctx, securityContextKey{}, secCtx)
}

// GetSecurityContext extracts security context from the GraphQL context
func GetSecurityContext(ctx context.Context) (SecurityContext, bool) {
	secCtx, ok := ctx.Value(securityContextKey{}).(SecurityContext)
	return secCtx, ok
}

// ExtractUserFromPluginContext extracts user information from Grafana plugin context
func ExtractUserFromPluginContext(pluginCtx backend.PluginContext) User {
	user := User{
		Name:  pluginCtx.User.Name,
		Login: pluginCtx.User.Login,
		Email: pluginCtx.User.Email,
		Role:  string(pluginCtx.User.Role),
	}

	// Map Grafana roles to our role system
	switch pluginCtx.User.Role {
	case "Admin":
		user.IsAdmin = true
		user.Role = "admin"
	case "Editor":
		user.IsEditor = true
		user.Role = "editor"
	case "Viewer":
		user.IsViewer = true
		user.Role = "viewer"
	default:
		user.Role = "viewer" // Default to viewer
		user.IsViewer = true
	}

	return user
}

// GetDefaultRBACConfig returns a sensible default RBAC configuration
func GetDefaultRBACConfig() RBACConfig {
	return RBACConfig{
		Enabled:       true,
		DefaultPolicy: "deny",
		AdminBypass:   true,
		RoleBasedPermissions: map[string]RolePermissions{
			"admin": {
				Resources: map[string][]string{
					"*": {"*"}, // Admin can do everything
				},
				Fields: map[string][]string{
					"*.*": {"read", "write"},
				},
			},
			"editor": {
				Resources: map[string][]string{
					"playlist":  {"read", "write"},
					"dashboard": {"read", "write"},
					"issue":     {"read", "write"},
				},
				Fields: map[string][]string{
					"playlist.*":  {"read", "write"},
					"dashboard.*": {"read", "write"},
					"issue.*":     {"read", "write"},
				},
				DeniedFields: []string{
					"user.password",
					"*.sensitive",
				},
			},
			"viewer": {
				Resources: map[string][]string{
					"playlist":  {"read"},
					"dashboard": {"read"},
					"issue":     {"read"},
				},
				Fields: map[string][]string{
					"playlist.*":  {"read"},
					"dashboard.*": {"read"},
					"issue.*":     {"read"},
				},
				DeniedFields: []string{
					"user.password",
					"*.sensitive",
					"*.secret",
				},
			},
		},
		GroupBasedPermissions:   make(map[string]GroupPermissions),
		UserSpecificPermissions: make(map[string]UserPermissions),
	}
}
