package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/grafana/grafana-app-sdk/graphql/security"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/graphql-go/graphql"
)

// Demo shows how to integrate RBAC with GraphQL Federation
func main() {
	fmt.Println("GraphQL RBAC Security Demo")
	fmt.Println("==========================")

	// 1. Set up logging
	logger := logging.NewSLogLogger(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// 2. Create a sample GraphQL schema (simulating your existing federation schema)
	schema := createDemoSchema()

	// 3. Set up RBAC security
	securityConfig := security.GetDefaultSecurityConfig()
	securityConfig.Logging.LogSuccessfulAccess = true // Enable verbose logging for demo

	// 4. Create secure gateway
	gateway, err := security.NewSecureGraphQLGateway(&schema, logger, securityConfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to create secure gateway: %v", err))
	}

	// 5. Test different user scenarios
	testScenarios := []TestScenario{
		{
			Name: "Admin User - Full Access",
			User: createUser("admin", "Admin"),
			Query: `{
				dashboard(id: "123") {
					id
					title
					sensitiveData
				}
				user {
					name
					password
				}
			}`,
			ExpectedResult: "[OK] Should have full access",
		},
		{
			Name: "Editor User - Limited Access",
			User: createUser("editor", "Editor"),
			Query: `{
				dashboard(id: "123") {
					id
					title
					sensitiveData
				}
				user {
					name
					password
				}
			}`,
			ExpectedResult: "[WARN] Should access dashboard but not user password",
		},
		{
			Name: "Viewer User - Read Only",
			User: createUser("viewer", "Viewer"),
			Query: `{
				dashboard(id: "123") {
					id
					title
				}
				playlist {
					name
					items
				}
			}`,
			ExpectedResult: "[OK] Should have read access to dashboard and playlist",
		},
		{
			Name: "Viewer User - Denied Sensitive Data",
			User: createUser("viewer", "Viewer"),
			Query: `{
				dashboard(id: "123") {
					sensitiveData
				}
			}`,
			ExpectedResult: "[DENY] Should be denied access to sensitive data",
		},
	}

	// 6. Run test scenarios
	for i, scenario := range testScenarios {
		fmt.Printf("\n--- Test %d: %s ---\n", i+1, scenario.Name)
		fmt.Printf("Query: %s\n", strings.ReplaceAll(scenario.Query, "\n", " "))
		fmt.Printf("Expected: %s\n", scenario.ExpectedResult)

		runTestScenario(gateway, scenario, logger)
	}

	// 7. Start HTTP server for interactive testing
	fmt.Println("\nStarting HTTP server for interactive testing...")
	fmt.Println("Visit: http://localhost:8080")
	fmt.Println("Use different users by setting user role in the web interface")

	http.HandleFunc("/graphql", createHTTPHandler(gateway, logger))
	http.HandleFunc("/", serveGraphiQL)

	fmt.Println("Server running on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.Error("Server failed", "error", err)
	}
}

// TestScenario represents a test case
type TestScenario struct {
	Name           string
	User           backend.User
	Query          string
	ExpectedResult string
}

// createUser creates a test user with the specified role
func createUser(name, role string) backend.User {
	return backend.User{
		Name:  name,
		Login: name,
		Email: fmt.Sprintf("%s@example.com", name),
		Role:  role,
	}
}

// runTestScenario executes a single test scenario
func runTestScenario(gateway *security.SecureGraphQLGateway, scenario TestScenario, logger logging.Logger) {
	ctx := context.Background()
	pluginCtx := backend.PluginContext{
		User: &scenario.User,
	}

	result, err := gateway.HandleGraphQLRequest(ctx, scenario.Query, nil, pluginCtx)
	if err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		return
	}

	// Check for errors in the GraphQL result
	if len(result.Errors) > 0 {
		fmt.Printf("[ERRORS] GraphQL Errors:\n")
		for _, gqlErr := range result.Errors {
			if strings.Contains(gqlErr.Message, "access denied") {
				fmt.Printf("  [DENIED] Access Denied: %s\n", gqlErr.Message)
			} else {
				fmt.Printf("  [ERROR] %s\n", gqlErr.Message)
			}
		}
	}

	// Show successful data
	if result.Data != nil {
		fmt.Printf("[SUCCESS] Successful Response:\n")
		prettyJSON, _ := json.MarshalIndent(result.Data, "  ", "  ")
		fmt.Printf("  %s\n", string(prettyJSON))
	}
}

// createDemoSchema creates a sample GraphQL schema for testing
func createDemoSchema() graphql.Schema {
	// Define types
	dashboardType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Dashboard",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return "123", nil
				},
			},
			"title": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return "Sample Dashboard", nil
				},
			},
			"sensitiveData": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return "SECRET_API_KEY_12345", nil
				},
			},
		},
	})

	userType := graphql.NewObject(graphql.ObjectConfig{
		Name: "User",
		Fields: graphql.Fields{
			"name": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return "John Doe", nil
				},
			},
			"password": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return "super_secret_password", nil
				},
			},
		},
	})

	playlistType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Playlist",
		Fields: graphql.Fields{
			"name": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return "My Playlist", nil
				},
			},
			"items": &graphql.Field{
				Type: graphql.NewList(graphql.String),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return []string{"item1", "item2", "item3"}, nil
				},
			},
		},
	})

	// Define query type
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"dashboard": &graphql.Field{
				Type: dashboardType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type: graphql.String,
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return map[string]interface{}{}, nil // Return empty map, individual fields will resolve
				},
			},
			"user": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return map[string]interface{}{}, nil
				},
			},
			"playlist": &graphql.Field{
				Type: playlistType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return map[string]interface{}{}, nil
				},
			},
		},
	})

	// Create schema
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query: queryType,
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to create GraphQL schema: %v", err))
	}

	return schema
}

// createHTTPHandler creates an HTTP handler for interactive GraphQL testing
func createHTTPHandler(gateway *security.SecureGraphQLGateway, logger logging.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Handle CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-Role")

		if r.Method == "OPTIONS" {
			return
		}

		// Parse GraphQL request
		var request struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Get user role from header (for demo purposes)
		userRole := r.Header.Get("X-User-Role")
		if userRole == "" {
			userRole = "Viewer" // Default to viewer
		}

		// Create plugin context
		pluginCtx := backend.PluginContext{
			User: &backend.User{
				Name:  "demo-user",
				Login: "demo",
				Email: "demo@example.com",
				Role:  userRole,
			},
		}

		// Execute GraphQL query
		ctx := context.Background()
		result, err := gateway.HandleGraphQLRequest(ctx, request.Query, request.Variables, pluginCtx)
		if err != nil {
			logger.Error("GraphQL execution failed", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Return result
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			logger.Error("Failed to encode response", "error", err)
		}
	}
}

// serveGraphiQL serves a simple GraphiQL interface for testing
func serveGraphiQL(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
    <title>GraphQL RBAC Demo</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .container { max-width: 800px; margin: 0 auto; }
        .role-selector { margin-bottom: 20px; }
        .query-examples { margin-bottom: 30px; }
        .example { background: #f5f5f5; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .example h4 { margin-top: 0; }
        .example code { background: white; padding: 10px; display: block; margin: 10px 0; border-radius: 3px; }
        .test-area { border: 1px solid #ddd; padding: 20px; border-radius: 5px; }
        textarea { width: 100%; height: 200px; font-family: monospace; }
        button { background: #007cba; color: white; padding: 10px 20px; border: none; border-radius: 3px; cursor: pointer; }
        button:hover { background: #005a87; }
        .result { margin-top: 20px; padding: 15px; background: #f9f9f9; border-radius: 5px; }
        .warning { background: #fff3cd; border: 1px solid #ffeaa7; padding: 10px; border-radius: 3px; margin: 10px 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>GraphQL RBAC Security Demo</h1>
        
        <div class="role-selector">
            <label>Select User Role: </label>
            <select id="userRole">
                <option value="Admin">Admin (Full Access)</option>
                <option value="Editor">Editor (Limited Access)</option>
                <option value="Viewer" selected>Viewer (Read Only)</option>
            </select>
        </div>

        <div class="query-examples">
                         <h3>Example Queries to Test</h3>
             
             <div class="example">
                 <h4>[SAFE] Query (All roles can access)</h4>
                 <code>{ dashboard(id: "123") { id title } }</code>
             </div>
             
             <div class="example">
                 <h4>[SENSITIVE] Query (Only Admin can access)</h4>
                 <code>{ dashboard(id: "123") { id title sensitiveData } }</code>
             </div>
             
             <div class="example">
                 <h4>[RESTRICTED] Query (Only Admin can access)</h4>
                 <code>{ user { name password } }</code>
             </div>
             
             <div class="example">
                 <h4>[COMPLEX] Query (Mixed permissions)</h4>
                <code>{ 
  dashboard(id: "123") { id title sensitiveData }
  playlist { name items }
  user { name password }
}</code>
            </div>
        </div>

        <div class="warning">
                         <strong>How to Test:</strong>
            <ul>
                <li>Select different user roles above</li>
                <li>Try the example queries</li>
                <li>Notice how sensitive fields are denied for non-admin users</li>
                <li>Check the browser console for detailed security logs</li>
            </ul>
        </div>

        <div class="test-area">
                         <h3>Test GraphQL Query</h3>
            <textarea id="queryInput" placeholder="Enter your GraphQL query here...">{ dashboard(id: "123") { id title } }</textarea>
            <br><br>
            <button onclick="executeQuery()">Execute Query</button>
            
            <div id="result" class="result" style="display: none;">
                <h4>Result:</h4>
                <pre id="resultContent"></pre>
            </div>
        </div>
    </div>

    <script>
        function executeQuery() {
            const query = document.getElementById('queryInput').value;
            const userRole = document.getElementById('userRole').value;
            
            fetch('/graphql', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-User-Role': userRole
                },
                body: JSON.stringify({ query: query })
            })
            .then(response => response.json())
            .then(data => {
                document.getElementById('resultContent').textContent = JSON.stringify(data, null, 2);
                document.getElementById('result').style.display = 'block';
                
                                 // Log security info to console
                 console.log('Security Test Results:', {
                    userRole: userRole,
                    query: query,
                    hasErrors: data.errors && data.errors.length > 0,
                    errors: data.errors,
                    data: data.data
                });
            })
            .catch(error => {
                document.getElementById('resultContent').textContent = 'Error: ' + error.message;
                document.getElementById('result').style.display = 'block';
            });
        }
        
        // Allow Enter key to execute query
        document.getElementById('queryInput').addEventListener('keydown', function(e) {
            if (e.ctrlKey && e.key === 'Enter') {
                executeQuery();
            }
        });
    </script>
</body>
</html>
	`))
}
