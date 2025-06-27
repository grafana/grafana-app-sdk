package main

import (
	"context"
	"fmt"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/plugin/router"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-app-sdk/simple"

	investigationsv0alpha1 "github.com/grafana/grafana/apps/investigations/pkg/apis/investigations/v0alpha1"
)

// InvestigationStore implements Store interface for investigations
type InvestigationStore struct {
	investigations map[string]*investigationsv0alpha1.Investigation
}

func NewInvestigationStore() *InvestigationStore {
	return &InvestigationStore{
		investigations: make(map[string]*investigationsv0alpha1.Investigation),
	}
}

func (s *InvestigationStore) Add(ctx context.Context, obj resource.Object) (resource.Object, error) {
	investigation := obj.(*investigationsv0alpha1.Investigation)
	key := investigation.GetName()
	s.investigations[key] = investigation
	return investigation, nil
}

func (s *InvestigationStore) Get(ctx context.Context, kind string, identifier resource.Identifier) (resource.Object, error) {
	investigation, exists := s.investigations[identifier.Name]
	if !exists {
		return nil, fmt.Errorf("investigation %s not found", identifier.Name)
	}
	return investigation, nil
}

func (s *InvestigationStore) List(ctx context.Context, kind string, options resource.StoreListOptions) (resource.ListObject, error) {
	investigations := make([]resource.Object, 0, len(s.investigations))
	for _, investigation := range s.investigations {
		investigations = append(investigations, investigation)
	}

	list := &investigationsv0alpha1.InvestigationList{
		Items: make([]investigationsv0alpha1.Investigation, len(investigations)),
	}
	
	for i, obj := range investigations {
		list.Items[i] = *obj.(*investigationsv0alpha1.Investigation)
	}

	return list, nil
}

func (s *InvestigationStore) Update(ctx context.Context, obj resource.Object) (resource.Object, error) {
	investigation := obj.(*investigationsv0alpha1.Investigation)
	key := investigation.GetName()
	s.investigations[key] = investigation
	return investigation, nil
}

func (s *InvestigationStore) Delete(ctx context.Context, kind string, identifier resource.Identifier) error {
	delete(s.investigations, identifier.Name)
	return nil
}

// GraphQLInvestigationApp extends the base investigations app with GraphQL support
type GraphQLInvestigationApp struct {
	baseApp         app.App
	graphqlProvider *router.AppGraphQLProvider
	store           *InvestigationStore
}

// NewGraphQLInvestigationApp creates a new investigations app with GraphQL support
func NewGraphQLInvestigationApp(cfg app.Config) (*GraphQLInvestigationApp, error) {
	// Create base app
	simpleConfig := simple.AppConfig{
		Name:       "investigation",
		KubeConfig: cfg.KubeConfig,
		ManagedKinds: []simple.AppManagedKind{
			{
				Kind: investigationsv0alpha1.InvestigationKind(),
			},
			{
				Kind: investigationsv0alpha1.InvestigationIndexKind(),
			},
		},
	}

	baseApp, err := simple.NewApp(simpleConfig)
	if err != nil {
		return nil, err
	}

	if err := baseApp.ValidateManifest(cfg.ManifestData); err != nil {
		return nil, err
	}

	// Create GraphQL provider
	graphqlProvider := router.NewAppGraphQLProvider("investigations")
	store := NewInvestigationStore()

	// Add some sample data
	sampleInvestigation := &investigationsv0alpha1.Investigation{}
	sampleInvestigation.SetName("sample-investigation")
	sampleInvestigation.SetNamespace("default")
	store.Add(context.Background(), sampleInvestigation)

	app := &GraphQLInvestigationApp{
		baseApp:         baseApp,
		graphqlProvider: graphqlProvider,
		store:           store,
	}

	// Set up GraphQL schema
	if err := app.setupGraphQL(); err != nil {
		return nil, err
	}

	return app, nil
}

// setupGraphQL configures the GraphQL schema for this app
func (app *GraphQLInvestigationApp) setupGraphQL() error {
	// Create resource collection from the managed kinds
	kinds := []resource.Schema{
		investigationsv0alpha1.InvestigationKind(),
		investigationsv0alpha1.InvestigationIndexKind(),
	}

	collection := resource.NewKindCollection(kinds...)
	
	// Add the resource collection to the GraphQL provider
	return app.graphqlProvider.AddResourceCollection("investigations", collection, app.store)
}

// GetGraphQLProvider returns the GraphQL provider for registration with the server
func (app *GraphQLInvestigationApp) GetGraphQLProvider() *router.AppGraphQLProvider {
	return app.graphqlProvider
}

// App interface methods (delegate to base app)
func (app *GraphQLInvestigationApp) Start(ctx context.Context) error {
	return app.baseApp.Start(ctx)
}

func (app *GraphQLInvestigationApp) Stop(ctx context.Context) error {
	return app.baseApp.Stop(ctx)
}

func (app *GraphQLInvestigationApp) IsReady(ctx context.Context) bool {
	return app.baseApp.IsReady(ctx)
}

func (app *GraphQLInvestigationApp) GetMetrics(ctx context.Context) ([]byte, error) {
	return app.baseApp.GetMetrics(ctx)
}

func (app *GraphQLInvestigationApp) GetProbes(ctx context.Context) ([]byte, error) {
	return app.baseApp.GetProbes(ctx)
}

// Example usage showing how this app would be registered with the main Grafana server
func main() {
	fmt.Println("Example: GraphQL Investigations App")
	fmt.Println("===================================")
	
	// This would typically be called during app loading in the main Grafana server
	cfg := app.Config{
		// Configuration would be provided by the app loading system
	}
	
	// Create the GraphQL-enabled investigations app
	investigationsApp, err := NewGraphQLInvestigationApp(cfg)
	if err != nil {
		fmt.Printf("Failed to create investigations app: %v\n", err)
		return
	}
	
	// Get the GraphQL provider for registration
	provider := investigationsApp.GetGraphQLProvider()
	
	fmt.Printf("Created GraphQL provider for app: %s\n", provider.GetAppName())
	
	// In the real implementation, this would be registered with the main Grafana server's
	// GraphQL registry via something like:
	// apiServerService.RegisterGraphQLApp("investigations", provider)
	
	schema, err := provider.GetGraphQLSchema()
	if err != nil {
		fmt.Printf("Failed to get schema: %v\n", err)
		return
	}
	
	fmt.Printf("GraphQL schema ready with query type: %s\n", schema.QueryType().Name())
	if schema.MutationType() != nil {
		fmt.Printf("GraphQL schema ready with mutation type: %s\n", schema.MutationType().Name())
	}
	
	fmt.Println("\nExample GraphQL queries for this app:")
	fmt.Println("query { investigations_investigation(name: \"sample-investigation\") { metadata { name } } }")
	fmt.Println("query { investigations_investigations { metadata { name } } }")
	fmt.Println("\nExample GraphQL mutations:")
	fmt.Println("mutation { investigations_createInvestigation(input: { metadata: { name: \"new-investigation\" } }) { metadata { name } } }")
} 