package gateway

import (
	"testing"

	codecgen "github.com/grafana/grafana-app-sdk/graphql/codegen"
	"github.com/grafana/grafana-app-sdk/graphql/subgraph"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/graphql-go/graphql"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestMeshStyleGateway_Basic tests basic functionality of the MeshStyleGateway
func TestMeshStyleGateway_Basic(t *testing.T) {
	// Create a logger for testing (nil is fine for tests)
	var logger logging.Logger

	// Create a new MeshStyleGateway
	config := MeshGatewayConfig{
		Logger: logger,
	}
	gateway := NewMeshStyleGateway(config)

	// Verify initial state
	if gateway == nil {
		t.Fatal("NewMeshStyleGateway returned nil")
	}

	if len(gateway.GetSubgraphs()) != 0 {
		t.Errorf("Expected 0 subgraphs, got %d", len(gateway.GetSubgraphs()))
	}

	if len(gateway.GetRelationships()) != 0 {
		t.Errorf("Expected 0 relationships, got %d", len(gateway.GetRelationships()))
	}
}

// TestMeshStyleGateway_RegisterSubgraph tests subgraph registration
func TestMeshStyleGateway_RegisterSubgraph(t *testing.T) {
	var logger logging.Logger
	gateway := NewMeshStyleGateway(MeshGatewayConfig{Logger: logger})

	// Create a mock CUE-aware subgraph
	mockSubgraph := &mockCUEAwareSubgraph{
		schema: &graphql.Schema{},
		gv:     schema.GroupVersion{Group: "test.grafana.app", Version: "v1alpha1"},
		relationships: []codecgen.MeshRelationshipConfig{
			{
				SourceType:      "TestSource",
				SourceField:     "testField",
				TargetType:      "TestTarget",
				TargetService:   "testservice",
				TargetQuery:     "testQuery",
				TargetArguments: map[string]string{"name": "{source.name}"},
			},
		},
	}

	// Register the subgraph
	err := gateway.RegisterSubgraph(mockSubgraph.gv, mockSubgraph)
	if err != nil {
		t.Fatalf("Failed to register subgraph: %v", err)
	}

	// Verify registration
	subgraphs := gateway.GetSubgraphs()
	if len(subgraphs) != 1 {
		t.Errorf("Expected 1 subgraph, got %d", len(subgraphs))
	}

	expectedKey := "test.grafana.app/v1alpha1"
	if _, exists := subgraphs[expectedKey]; !exists {
		t.Errorf("Expected subgraph with key %s not found", expectedKey)
	}
}

// TestMeshStyleGateway_ParseAndConfigureRelationships tests relationship parsing
func TestMeshStyleGateway_ParseAndConfigureRelationships(t *testing.T) {
	var logger logging.Logger
	gateway := NewMeshStyleGateway(MeshGatewayConfig{Logger: logger})

	// Create mock subgraphs with relationships
	mockSubgraph1 := &mockCUEAwareSubgraph{
		schema: &graphql.Schema{},
		gv:     schema.GroupVersion{Group: "test1.grafana.app", Version: "v1alpha1"},
		relationships: []codecgen.MeshRelationshipConfig{
			{
				SourceType:    "Source1",
				SourceField:   "field1",
				TargetType:    "Target1",
				TargetService: "service1",
			},
		},
	}

	mockSubgraph2 := &mockCUEAwareSubgraph{
		schema: &graphql.Schema{},
		gv:     schema.GroupVersion{Group: "test2.grafana.app", Version: "v1alpha1"},
		relationships: []codecgen.MeshRelationshipConfig{
			{
				SourceType:    "Source2",
				SourceField:   "field2",
				TargetType:    "Target2",
				TargetService: "service2",
			},
		},
	}

	// Register subgraphs
	gateway.RegisterSubgraph(mockSubgraph1.gv, mockSubgraph1)
	gateway.RegisterSubgraph(mockSubgraph2.gv, mockSubgraph2)

	// Parse and configure relationships
	err := gateway.ParseAndConfigureRelationships()
	if err != nil {
		t.Fatalf("Failed to parse relationships: %v", err)
	}

	// Verify relationships were collected
	relationships := gateway.GetRelationships()
	if len(relationships) != 2 {
		t.Errorf("Expected 2 relationships, got %d", len(relationships))
	}

	// Verify specific relationships
	foundSource1 := false
	foundSource2 := false
	for _, rel := range relationships {
		if rel.SourceType == "Source1" {
			foundSource1 = true
		}
		if rel.SourceType == "Source2" {
			foundSource2 = true
		}
	}

	if !foundSource1 {
		t.Error("Expected Source1 relationship not found")
	}
	if !foundSource2 {
		t.Error("Expected Source2 relationship not found")
	}
}

// TestMeshStyleGateway_ComposeSchema tests schema composition
func TestMeshStyleGateway_ComposeSchema(t *testing.T) {
	var logger logging.Logger
	gateway := NewMeshStyleGateway(MeshGatewayConfig{Logger: logger})

	// Create a simple query field for testing
	testField := &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return "test", nil
		},
	}

	// Create a mock schema
	mockSchema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name: "Query",
			Fields: graphql.Fields{
				"testField": testField,
			},
		}),
	})
	if err != nil {
		t.Fatalf("Failed to create mock schema: %v", err)
	}

	// Create mock subgraph
	mockSubgraph := &mockCUEAwareSubgraph{
		schema:        &mockSchema,
		gv:            schema.GroupVersion{Group: "test.grafana.app", Version: "v1alpha1"},
		relationships: []codecgen.MeshRelationshipConfig{},
	}

	// Register subgraph
	gateway.RegisterSubgraph(mockSubgraph.gv, mockSubgraph)

	// Compose schema
	composedSchema, err := gateway.ComposeSchema()
	if err != nil {
		t.Fatalf("Failed to compose schema: %v", err)
	}

	// Verify schema was created
	if composedSchema == nil {
		t.Fatal("Composed schema is nil")
	}

	// Verify the schema has query fields
	if composedSchema.QueryType() == nil {
		t.Fatal("Composed schema has no query type")
	}

	// Verify field prefixing
	queryFields := composedSchema.QueryType().Fields()
	expectedFieldName := "test_grafana_app_v1alpha1_testField"
	if _, exists := queryFields[expectedFieldName]; !exists {
		t.Errorf("Expected prefixed field %s not found in composed schema", expectedFieldName)
		t.Logf("Available fields: %v", getFieldNames(queryFields))
	}
}

// mockCUEAwareSubgraph implements CUEAwareSubgraph for testing
type mockCUEAwareSubgraph struct {
	schema        *graphql.Schema
	gv            schema.GroupVersion
	relationships []codecgen.MeshRelationshipConfig
}

func (m *mockCUEAwareSubgraph) GetSchema() *graphql.Schema {
	return m.schema
}

func (m *mockCUEAwareSubgraph) GetResolvers() subgraph.ResolverMap {
	return make(subgraph.ResolverMap)
}

func (m *mockCUEAwareSubgraph) GetGroupVersion() schema.GroupVersion {
	return m.gv
}

func (m *mockCUEAwareSubgraph) GetKinds() []resource.Kind {
	return []resource.Kind{} // Empty for testing
}

func (m *mockCUEAwareSubgraph) GetRelationships() []codecgen.MeshRelationshipConfig {
	return m.relationships
}

// Helper function to get field names for debugging
func getFieldNames(fields graphql.FieldDefinitionMap) []string {
	var names []string
	for name := range fields {
		names = append(names, name)
	}
	return names
}
