package security

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func TestDefaultRBACPermissionChecker(t *testing.T) {
	logger := logging.NewSLogLogger(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Set to ERROR to reduce test output
	}))
	config := GetDefaultRBACConfig()
	checker := NewDefaultRBACPermissionChecker(config, logger)

	tests := []struct {
		name         string
		user         User
		resourceType string
		fieldName    string
		expectError  bool
		description  string
	}{
		{
			name: "admin_can_access_all_fields",
			user: User{
				Name:    "admin-user",
				Role:    "admin",
				IsAdmin: true,
			},
			resourceType: "dashboard",
			fieldName:    "secret",
			expectError:  false,
			description:  "Admin users should bypass all restrictions",
		},
		{
			name: "editor_can_access_dashboard_fields",
			user: User{
				Name:     "editor-user",
				Role:     "editor",
				IsEditor: true,
			},
			resourceType: "dashboard",
			fieldName:    "title",
			expectError:  false,
			description:  "Editor should access dashboard fields",
		},
		{
			name: "editor_denied_user_password",
			user: User{
				Name:     "editor-user",
				Role:     "editor",
				IsEditor: true,
			},
			resourceType: "user",
			fieldName:    "password",
			expectError:  true,
			description:  "Editor should be denied access to user password",
		},
		{
			name: "viewer_can_read_dashboard",
			user: User{
				Name:     "viewer-user",
				Role:     "viewer",
				IsViewer: true,
			},
			resourceType: "dashboard",
			fieldName:    "title",
			expectError:  false,
			description:  "Viewer should be able to read dashboard fields",
		},
		{
			name: "viewer_denied_write_operations",
			user: User{
				Name:     "viewer-user",
				Role:     "viewer",
				IsViewer: true,
			},
			resourceType: "unknown",
			fieldName:    "write_field",
			expectError:  true,
			description:  "Viewer should be denied access to unknown resources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := checker.CheckFieldPermission(ctx, tt.user, tt.resourceType, tt.fieldName)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for %s but got none. %s", tt.name, tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for %s but got: %v. %s", tt.name, err, tt.description)
			}
		})
	}
}

func TestExtractUserFromPluginContext(t *testing.T) {
	tests := []struct {
		name         string
		pluginCtx    backend.PluginContext
		expectedUser User
	}{
		{
			name: "admin_user_extraction",
			pluginCtx: backend.PluginContext{
				User: &backend.User{
					Name:  "admin",
					Login: "admin",
					Email: "admin@example.com",
					Role:  "Admin",
				},
			},
			expectedUser: User{
				Name:    "admin",
				Login:   "admin",
				Email:   "admin@example.com",
				Role:    "admin",
				IsAdmin: true,
			},
		},
		{
			name: "editor_user_extraction",
			pluginCtx: backend.PluginContext{
				User: &backend.User{
					Name:  "editor",
					Login: "editor",
					Email: "editor@example.com",
					Role:  "Editor",
				},
			},
			expectedUser: User{
				Name:     "editor",
				Login:    "editor",
				Email:    "editor@example.com",
				Role:     "editor",
				IsEditor: true,
			},
		},
		{
			name: "viewer_user_extraction",
			pluginCtx: backend.PluginContext{
				User: &backend.User{
					Name:  "viewer",
					Login: "viewer",
					Email: "viewer@example.com",
					Role:  "Viewer",
				},
			},
			expectedUser: User{
				Name:     "viewer",
				Login:    "viewer",
				Email:    "viewer@example.com",
				Role:     "viewer",
				IsViewer: true,
			},
		},
		{
			name: "unknown_role_defaults_to_viewer",
			pluginCtx: backend.PluginContext{
				User: &backend.User{
					Name:  "unknown",
					Login: "unknown",
					Email: "unknown@example.com",
					Role:  "CustomRole",
				},
			},
			expectedUser: User{
				Name:     "unknown",
				Login:    "unknown",
				Email:    "unknown@example.com",
				Role:     "viewer",
				IsViewer: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := ExtractUserFromPluginContext(tt.pluginCtx)

			if user.Name != tt.expectedUser.Name {
				t.Errorf("Expected name %s, got %s", tt.expectedUser.Name, user.Name)
			}
			if user.Role != tt.expectedUser.Role {
				t.Errorf("Expected role %s, got %s", tt.expectedUser.Role, user.Role)
			}
			if user.IsAdmin != tt.expectedUser.IsAdmin {
				t.Errorf("Expected IsAdmin %v, got %v", tt.expectedUser.IsAdmin, user.IsAdmin)
			}
			if user.IsEditor != tt.expectedUser.IsEditor {
				t.Errorf("Expected IsEditor %v, got %v", tt.expectedUser.IsEditor, user.IsEditor)
			}
			if user.IsViewer != tt.expectedUser.IsViewer {
				t.Errorf("Expected IsViewer %v, got %v", tt.expectedUser.IsViewer, user.IsViewer)
			}
		})
	}
}

func TestCustomPermissionConfiguration(t *testing.T) {
	logger := logging.NewSLogLogger(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Set to ERROR to reduce test output
	}))

	// Create a custom RBAC configuration
	customConfig := RBACConfig{
		Enabled:       true,
		DefaultPolicy: "deny",
		AdminBypass:   true,
		RoleBasedPermissions: map[string]RolePermissions{
			"custom_role": {
				Resources: map[string][]string{
					"special_resource": {"read", "write"},
				},
				Fields: map[string][]string{
					"special_resource.special_field": {"read"},
				},
			},
		},
		UserSpecificPermissions: map[string]UserPermissions{
			"special_user": {
				AdditionalPermissions: []string{
					"secret_resource.secret_field",
				},
			},
		},
		GroupBasedPermissions: map[string]GroupPermissions{
			"dev_team": {
				Resources: map[string][]string{
					"debug_info": {"read"},
				},
				Fields: map[string][]string{
					"debug_info.logs": {"read"},
				},
			},
		},
	}

	checker := NewDefaultRBACPermissionChecker(customConfig, logger)

	tests := []struct {
		name         string
		user         User
		resourceType string
		fieldName    string
		expectError  bool
	}{
		{
			name: "custom_role_access_allowed",
			user: User{
				Name: "custom_user",
				Role: "custom_role",
			},
			resourceType: "special_resource",
			fieldName:    "special_field",
			expectError:  false,
		},
		{
			name: "user_specific_permission",
			user: User{
				Name: "special_user",
				Role: "viewer",
			},
			resourceType: "secret_resource",
			fieldName:    "secret_field",
			expectError:  false,
		},
		{
			name: "group_based_permission",
			user: User{
				Name:   "dev_user",
				Role:   "viewer",
				Groups: []string{"dev_team"},
			},
			resourceType: "debug_info",
			fieldName:    "logs",
			expectError:  false,
		},
		{
			name: "denied_by_default_policy",
			user: User{
				Name: "regular_user",
				Role: "viewer",
			},
			resourceType: "unknown_resource",
			fieldName:    "unknown_field",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := checker.CheckFieldPermission(ctx, tt.user, tt.resourceType, tt.fieldName)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for %s but got none", tt.name)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for %s but got: %v", tt.name, err)
			}
		})
	}
}

func TestSecurityContextHandling(t *testing.T) {
	user := User{
		Name:     "test_user",
		Role:     "editor",
		IsEditor: true,
	}

	ctx := context.Background()

	// Test adding user to context
	ctx = WithUser(ctx, user)

	// Test retrieving user from context
	retrievedUser, found := GetUser(ctx)
	if !found {
		t.Error("Expected to find user in context")
	}

	if retrievedUser.Name != user.Name {
		t.Errorf("Expected user name %s, got %s", user.Name, retrievedUser.Name)
	}

	// Test security context
	secCtx := SecurityContext{
		User:        user,
		Permissions: []string{"dashboard.read", "playlist.write"},
	}

	ctx = WithSecurityContext(ctx, secCtx)

	retrievedSecCtx, found := GetSecurityContext(ctx)
	if !found {
		t.Error("Expected to find security context")
	}

	if retrievedSecCtx.User.Name != user.Name {
		t.Errorf("Expected security context user name %s, got %s", user.Name, retrievedSecCtx.User.Name)
	}

	if len(retrievedSecCtx.Permissions) != 2 {
		t.Errorf("Expected 2 permissions, got %d", len(retrievedSecCtx.Permissions))
	}
}

func TestGetUserPermissions(t *testing.T) {
	logger := logging.NewSLogLogger(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Set to ERROR to reduce test output
	}))
	config := GetDefaultRBACConfig()
	checker := NewDefaultRBACPermissionChecker(config, logger)

	tests := []struct {
		name             string
		user             User
		expectedMinPerms int // Minimum number of permissions expected
	}{
		{
			name: "admin_has_many_permissions",
			user: User{
				Name:    "admin",
				Role:    "admin",
				IsAdmin: true,
			},
			expectedMinPerms: 1, // Admin should have at least some permissions
		},
		{
			name: "editor_has_moderate_permissions",
			user: User{
				Name:     "editor",
				Role:     "editor",
				IsEditor: true,
			},
			expectedMinPerms: 1,
		},
		{
			name: "viewer_has_limited_permissions",
			user: User{
				Name:     "viewer",
				Role:     "viewer",
				IsViewer: true,
			},
			expectedMinPerms: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			permissions, err := checker.GetUserPermissions(ctx, tt.user)

			if err != nil {
				t.Errorf("Unexpected error getting permissions for %s: %v", tt.name, err)
			}

			if len(permissions) < tt.expectedMinPerms {
				t.Errorf("Expected at least %d permissions for %s, got %d",
					tt.expectedMinPerms, tt.name, len(permissions))
			}

			t.Logf("User %s has permissions: %v", tt.user.Name, permissions)
		})
	}
}

// BenchmarkRBACPermissionCheck benchmarks the performance of permission checking
func BenchmarkRBACPermissionCheck(b *testing.B) {
	logger := logging.NewSLogLogger(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Set to ERROR to reduce test output
	}))
	config := GetDefaultRBACConfig()
	checker := NewDefaultRBACPermissionChecker(config, logger)

	user := User{
		Name:     "bench_user",
		Role:     "editor",
		IsEditor: true,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = checker.CheckFieldPermission(ctx, user, "dashboard", "title")
	}
}
