package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/grafana/grafana-app-sdk/plugin/router"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/graphql-go/graphql"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GraphQLInvestigationApp demonstrates GraphQL integration with the Investigations app
type GraphQLInvestigationApp struct {
	graphqlRouter *router.GraphQLRouter
	registry      *router.GraphQLRegistry
	store         *InvestigationStore
}

// InvestigationStore implements the Store interface for investigations
type InvestigationStore struct {
	investigations map[string]*Investigation
	indexes        map[string]*InvestigationIndex
}

// Investigation represents an investigation resource (simplified structure)
type Investigation struct {
	resource.Object
	metav1.ObjectMeta `json:"metadata"`
	Spec              InvestigationSpec `json:"spec"`
}

type InvestigationSpec struct {
	Title                 string              `json:"title"`
	CreatedByProfile      Person              `json:"createdByProfile"`
	HasCustomName         bool                `json:"hasCustomName"`
	IsFavorite            bool                `json:"isFavorite"`
	OverviewNote          string              `json:"overviewNote"`
	OverviewNoteUpdatedAt string              `json:"overviewNoteUpdatedAt"`
	Collectables          []Collectable       `json:"collectables"`
	ViewMode              ViewMode            `json:"viewMode"`
}

type Person struct {
	UID         string `json:"uid"`
	Name        string `json:"name"`
	GravatarURL string `json:"gravatarUrl"`
}

type Collectable struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Title     string `json:"title"`
	Origin    string `json:"origin"`
	Type      string `json:"type"`
	Queries   []string `json:"queries"`
	TimeRange TimeRange `json:"timeRange"`
	Datasource DatasourceRef `json:"datasource"`
	URL       string `json:"url"`
	LogoPath  *string `json:"logoPath,omitempty"`
	Note      string `json:"note"`
	NoteUpdatedAt string `json:"noteUpdatedAt"`
	FieldConfig string `json:"fieldConfig"`
}

type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
	Raw  struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"raw"`
}

type DatasourceRef struct {
	UID string `json:"uid"`
}

type ViewMode struct {
	Mode         string `json:"mode"`
	ShowComments bool   `json:"showComments"`
	ShowTooltips bool   `json:"showTooltips"`
}

// InvestigationIndex represents an investigation index resource
type InvestigationIndex struct {
	resource.Object
	metav1.ObjectMeta `json:"metadata"`
	Spec              InvestigationIndexSpec `json:"spec"`
}

type InvestigationIndexSpec struct {
	Title                    string                   `json:"title"`
	Owner                    Person                   `json:"owner"`
	InvestigationSummaries  []InvestigationSummary   `json:"investigationSummaries"`
}

type InvestigationSummary struct {
	Title                 string              `json:"title"`
	CreatedByProfile      Person              `json:"createdByProfile"`
	HasCustomName         bool                `json:"hasCustomName"`
	IsFavorite            bool                `json:"isFavorite"`
	OverviewNote          string              `json:"overviewNote"`
	OverviewNoteUpdatedAt string              `json:"overviewNoteUpdatedAt"`
	ViewMode              ViewMode            `json:"viewMode"`
	CollectableSummaries  []CollectableSummary `json:"collectableSummaries"`
}

type CollectableSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// Store interface implementations
func (s *InvestigationStore) Add(ctx context.Context, obj resource.Object) (resource.Object, error) {
	switch obj.GetStaticMetadata().Kind {
	case "Investigation":
		inv := obj.(*Investigation)
		inv.ObjectMeta.Name = fmt.Sprintf("investigation-%d", time.Now().Unix())
		inv.ObjectMeta.UID = fmt.Sprintf("uid-%d", time.Now().Unix())
		inv.ObjectMeta.CreationTimestamp = metav1.Now()
		s.investigations[inv.ObjectMeta.Name] = inv
		return inv, nil
	case "InvestigationIndex":
		idx := obj.(*InvestigationIndex)
		idx.ObjectMeta.Name = fmt.Sprintf("index-%d", time.Now().Unix())
		idx.ObjectMeta.UID = fmt.Sprintf("uid-%d", time.Now().Unix())
		idx.ObjectMeta.CreationTimestamp = metav1.Now()
		s.indexes[idx.ObjectMeta.Name] = idx
		return idx, nil
	}
	return nil, fmt.Errorf("unsupported kind: %s", obj.GetStaticMetadata().Kind)
}

func (s *InvestigationStore) Get(ctx context.Context, kind string, identifier resource.Identifier) (resource.Object, error) {
	switch kind {
	case "Investigation":
		if inv, exists := s.investigations[identifier.Name]; exists {
			return inv, nil
		}
		return nil, fmt.Errorf("investigation not found: %s", identifier.Name)
	case "InvestigationIndex":
		if idx, exists := s.indexes[identifier.Name]; exists {
			return idx, nil
		}
		return nil, fmt.Errorf("investigation index not found: %s", identifier.Name)
	}
	return nil, fmt.Errorf("unsupported kind: %s", kind)
}

func (s *InvestigationStore) List(ctx context.Context, kind string, options resource.StoreListOptions) (resource.ListObject, error) {
	switch kind {
	case "Investigation":
		items := make([]resource.Object, 0, len(s.investigations))
		for _, inv := range s.investigations {
			// Apply namespace filter if specified
			if options.Namespace != "" && inv.GetNamespace() != options.Namespace {
				continue
			}
			items = append(items, inv)
		}
		return &resource.SimpleListObject{Items: items}, nil
	case "InvestigationIndex":
		items := make([]resource.Object, 0, len(s.indexes))
		for _, idx := range s.indexes {
			// Apply namespace filter if specified
			if options.Namespace != "" && idx.GetNamespace() != options.Namespace {
				continue
			}
			items = append(items, idx)
		}
		return &resource.SimpleListObject{Items: items}, nil
	}
	return nil, fmt.Errorf("unsupported kind: %s", kind)
}

func (s *InvestigationStore) Update(ctx context.Context, obj resource.Object) (resource.Object, error) {
	switch obj.GetStaticMetadata().Kind {
	case "Investigation":
		inv := obj.(*Investigation)
		if _, exists := s.investigations[inv.ObjectMeta.Name]; !exists {
			return nil, fmt.Errorf("investigation not found: %s", inv.ObjectMeta.Name)
		}
		inv.ObjectMeta.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
		s.investigations[inv.ObjectMeta.Name] = inv
		return inv, nil
	case "InvestigationIndex":
		idx := obj.(*InvestigationIndex)
		if _, exists := s.indexes[idx.ObjectMeta.Name]; !exists {
			return nil, fmt.Errorf("investigation index not found: %s", idx.ObjectMeta.Name)
		}
		idx.ObjectMeta.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
		s.indexes[idx.ObjectMeta.Name] = idx
		return idx, nil
	}
	return nil, fmt.Errorf("unsupported kind: %s", obj.GetStaticMetadata().Kind)
}

func (s *InvestigationStore) Delete(ctx context.Context, kind string, identifier resource.Identifier) error {
	switch kind {
	case "Investigation":
		if _, exists := s.investigations[identifier.Name]; !exists {
			return fmt.Errorf("investigation not found: %s", identifier.Name)
		}
		delete(s.investigations, identifier.Name)
		return nil
	case "InvestigationIndex":
		if _, exists := s.indexes[identifier.Name]; !exists {
			return fmt.Errorf("investigation index not found: %s", identifier.Name)
		}
		delete(s.indexes, identifier.Name)
		return nil
	}
	return fmt.Errorf("unsupported kind: %s", kind)
}

// Implement resource.Object interface for Investigation
func (i *Investigation) GetStaticMetadata() resource.StaticMetadata {
	return resource.StaticMetadata{
		Name:      i.ObjectMeta.Name,
		Namespace: i.ObjectMeta.Namespace,
		Group:     "investigations.grafana.app",
		Version:   "v0alpha1",
		Kind:      "Investigation",
	}
}

func (i *Investigation) SetStaticMetadata(metadata resource.StaticMetadata) {
	i.ObjectMeta.Name = metadata.Name
	i.ObjectMeta.Namespace = metadata.Namespace
}

func (i *Investigation) GetName() string { return i.ObjectMeta.Name }
func (i *Investigation) GetNamespace() string { return i.ObjectMeta.Namespace }
func (i *Investigation) GetUID() string { return string(i.ObjectMeta.UID) }
func (i *Investigation) GetResourceVersion() string { return i.ObjectMeta.ResourceVersion }
func (i *Investigation) GetCreationTimestamp() time.Time { return i.ObjectMeta.CreationTimestamp.Time }
func (i *Investigation) GetDeletionTimestamp() *time.Time {
	if i.ObjectMeta.DeletionTimestamp != nil {
		t := i.ObjectMeta.DeletionTimestamp.Time
		return &t
	}
	return nil
}
func (i *Investigation) GetLabels() map[string]string { return i.ObjectMeta.Labels }
func (i *Investigation) SetLabels(labels map[string]string) { i.ObjectMeta.Labels = labels }
func (i *Investigation) GetAnnotations() map[string]string { return i.ObjectMeta.Annotations }
func (i *Investigation) SetAnnotations(annotations map[string]string) { i.ObjectMeta.Annotations = annotations }
func (i *Investigation) GetSpec() map[string]interface{} {
	b, _ := json.Marshal(i.Spec)
	var spec map[string]interface{}
	json.Unmarshal(b, &spec)
	return spec
}
func (i *Investigation) SetSpec(spec map[string]interface{}) error {
	b, _ := json.Marshal(spec)
	return json.Unmarshal(b, &i.Spec)
}
func (i *Investigation) GetSubresources() map[string]interface{} { return nil }
func (i *Investigation) SetSubresource(name string, value interface{}) error { return nil }
func (i *Investigation) Copy() resource.Object {
	b, _ := json.Marshal(i)
	var copy Investigation
	json.Unmarshal(b, &copy)
	return &copy
}

// Implement resource.Object interface for InvestigationIndex
func (idx *InvestigationIndex) GetStaticMetadata() resource.StaticMetadata {
	return resource.StaticMetadata{
		Name:      idx.ObjectMeta.Name,
		Namespace: idx.ObjectMeta.Namespace,
		Group:     "investigations.grafana.app",
		Version:   "v0alpha1",
		Kind:      "InvestigationIndex",
	}
}

func (idx *InvestigationIndex) SetStaticMetadata(metadata resource.StaticMetadata) {
	idx.ObjectMeta.Name = metadata.Name
	idx.ObjectMeta.Namespace = metadata.Namespace
}

func (idx *InvestigationIndex) GetName() string { return idx.ObjectMeta.Name }
func (idx *InvestigationIndex) GetNamespace() string { return idx.ObjectMeta.Namespace }
func (idx *InvestigationIndex) GetUID() string { return string(idx.ObjectMeta.UID) }
func (idx *InvestigationIndex) GetResourceVersion() string { return idx.ObjectMeta.ResourceVersion }
func (idx *InvestigationIndex) GetCreationTimestamp() time.Time { return idx.ObjectMeta.CreationTimestamp.Time }
func (idx *InvestigationIndex) GetDeletionTimestamp() *time.Time {
	if idx.ObjectMeta.DeletionTimestamp != nil {
		t := idx.ObjectMeta.DeletionTimestamp.Time
		return &t
	}
	return nil
}
func (idx *InvestigationIndex) GetLabels() map[string]string { return idx.ObjectMeta.Labels }
func (idx *InvestigationIndex) SetLabels(labels map[string]string) { idx.ObjectMeta.Labels = labels }
func (idx *InvestigationIndex) GetAnnotations() map[string]string { return idx.ObjectMeta.Annotations }
func (idx *InvestigationIndex) SetAnnotations(annotations map[string]string) { idx.ObjectMeta.Annotations = annotations }
func (idx *InvestigationIndex) GetSpec() map[string]interface{} {
	b, _ := json.Marshal(idx.Spec)
	var spec map[string]interface{}
	json.Unmarshal(b, &spec)
	return spec
}
func (idx *InvestigationIndex) SetSpec(spec map[string]interface{}) error {
	b, _ := json.Marshal(spec)
	return json.Unmarshal(b, &idx.Spec)
}
func (idx *InvestigationIndex) GetSubresources() map[string]interface{} { return nil }
func (idx *InvestigationIndex) SetSubresource(name string, value interface{}) error { return nil }
func (idx *InvestigationIndex) Copy() resource.Object {
	b, _ := json.Marshal(idx)
	var copy InvestigationIndex
	json.Unmarshal(b, &copy)
	return &copy
}

// Mock schema definitions for the investigation kinds
type InvestigationSchema struct{}
func (s *InvestigationSchema) Kind() string { return "Investigation" }
func (s *InvestigationSchema) Plural() string { return "Investigations" }
func (s *InvestigationSchema) Group() string { return "investigations.grafana.app" }
func (s *InvestigationSchema) Version() string { return "v0alpha1" }
func (s *InvestigationSchema) ZeroValue() resource.Object { return &Investigation{} }

type InvestigationIndexSchema struct{}
func (s *InvestigationIndexSchema) Kind() string { return "InvestigationIndex" }
func (s *InvestigationIndexSchema) Plural() string { return "InvestigationIndexes" }
func (s *InvestigationIndexSchema) Group() string { return "investigations.grafana.app" }
func (s *InvestigationIndexSchema) Version() string { return "v0alpha1" }
func (s *InvestigationIndexSchema) ZeroValue() resource.Object { return &InvestigationIndex{} }

// Mock resource collection
type InvestigationCollection struct{}
func (c *InvestigationCollection) Kinds() []resource.Schema {
	return []resource.Schema{
		&InvestigationSchema{},
		&InvestigationIndexSchema{},
	}
}

// NewGraphQLInvestigationApp creates a new GraphQL-enabled investigations app
func NewGraphQLInvestigationApp() (*GraphQLInvestigationApp, error) {
	// Create store with sample data
	store := &InvestigationStore{
		investigations: make(map[string]*Investigation),
		indexes:        make(map[string]*InvestigationIndex),
	}
	
	// Add sample data
	sampleInvestigation := &Investigation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample-investigation",
			Namespace: "default",
			UID:       "sample-inv-uid",
			Labels:    map[string]string{"owner": "alice"},
		},
		Spec: InvestigationSpec{
			Title: "Sample Investigation",
			CreatedByProfile: Person{
				UID:         "alice-uid",
				Name:        "Alice Smith",
				GravatarURL: "https://gravatar.com/avatar/alice",
			},
			HasCustomName:         true,
			IsFavorite:           true,
			OverviewNote:         "This is a sample investigation for GraphQL testing",
			OverviewNoteUpdatedAt: time.Now().Format(time.RFC3339),
			Collectables: []Collectable{
				{
					ID:        "col-1",
					CreatedAt: time.Now().Format(time.RFC3339),
					Title:     "CPU Usage Query",
					Origin:    "prometheus",
					Type:      "query",
					Queries:   []string{"cpu_usage_rate"},
					TimeRange: TimeRange{
						From: "now-1h",
						To:   "now",
						Raw: struct {
							From string `json:"from"`
							To   string `json:"to"`
						}{From: "now-1h", To: "now"},
					},
					Datasource: DatasourceRef{UID: "prometheus-uid"},
					URL:        "http://prometheus.local/graph",
					Note:       "Monitoring CPU usage patterns",
					NoteUpdatedAt: time.Now().Format(time.RFC3339),
					FieldConfig: "{}",
				},
			},
			ViewMode: ViewMode{
				Mode:         "full",
				ShowComments: true,
				ShowTooltips: true,
			},
		},
	}
	store.investigations["sample-investigation"] = sampleInvestigation
	
	sampleIndex := &InvestigationIndex{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alice-favorites",
			Namespace: "default",
			UID:       "sample-idx-uid",
		},
		Spec: InvestigationIndexSpec{
			Title: "Alice's Favorite Investigations",
			Owner: Person{
				UID:         "alice-uid",
				Name:        "Alice Smith",
				GravatarURL: "https://gravatar.com/avatar/alice",
			},
			InvestigationSummaries: []InvestigationSummary{
				{
					Title: "Sample Investigation",
					CreatedByProfile: Person{
						UID:         "alice-uid",
						Name:        "Alice Smith",
						GravatarURL: "https://gravatar.com/avatar/alice",
					},
					HasCustomName:         true,
					IsFavorite:           true,
					OverviewNote:         "This is a sample investigation for GraphQL testing",
					OverviewNoteUpdatedAt: time.Now().Format(time.RFC3339),
					ViewMode: ViewMode{
						Mode:         "full",
						ShowComments: true,
						ShowTooltips: true,
					},
					CollectableSummaries: []CollectableSummary{
						{
							ID:    "col-1",
							Title: "CPU Usage Query",
							Type:  "query",
						},
					},
				},
			},
		},
	}
	store.indexes["alice-favorites"] = sampleIndex
	
	// Create GraphQL router
	graphqlRouter, err := router.NewGraphQLRouter()
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL router: %w", err)
	}
	
	// Create resource collection and add to GraphQL router
	collection := &InvestigationCollection{}
	err = graphqlRouter.AddResourceGroup("investigations", collection, store)
	if err != nil {
		return nil, fmt.Errorf("failed to add resource group: %w", err)
	}
	
	// Create registry for aggregating multiple apps
	registry := router.NewGraphQLRegistry()
	err = registry.RegisterApp("investigations", graphqlRouter)
	if err != nil {
		return nil, fmt.Errorf("failed to register app: %w", err)
	}
	
	return &GraphQLInvestigationApp{
		graphqlRouter: graphqlRouter,
		registry:      registry,
		store:         store,
	}, nil
}

// GetUnifiedRouter returns the router that handles the unified /graphql endpoint
func (app *GraphQLInvestigationApp) GetUnifiedRouter() *router.JSONRouter {
	return app.registry.GetUnifiedRouter()
}

// GetAppSpecificRouter returns the app-specific GraphQL router
func (app *GraphQLInvestigationApp) GetAppSpecificRouter() *router.GraphQLRouter {
	return app.graphqlRouter
}

// Example usage demonstrating the GraphQL capabilities
func main() {
	app, err := NewGraphQLInvestigationApp()
	if err != nil {
		log.Fatalf("Failed to create GraphQL app: %v", err)
	}
	
	// Get the unified router that aggregates all apps
	unifiedRouter := app.GetUnifiedRouter()
	
	// Set up HTTP server
	http.Handle("/graphql", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a mock JSONRequest from HTTP request
		jsonReq := router.JSONRequest{
			Method: r.Method,
			URL:    *r.URL,
			Body:   r.Body,
			Headers: r.Header,
		}
		
		// Call the unified GraphQL handler
		ctx := context.Background()
		response, err := unifiedRouter.Handle("/graphql", func(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
			// This would normally be handled by the router's internal handler
			// For demo purposes, we'll show how to manually call it
			return nil, fmt.Errorf("demo only - use the actual router.Handle method")
		}, "POST")(ctx, jsonReq)
		
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	
	// Start server
	fmt.Println("GraphQL server starting on :8080")
	fmt.Println("Example queries:")
	fmt.Println()
	fmt.Println("1. List all investigations:")
	fmt.Println(`{
  investigations {
    metadata {
      name
      namespace
      uid
    }
    spec {
      title
      createdByProfile {
        name
        uid
      }
      isFavorite
      collectables {
        title
        type
        queries
      }
    }
  }
}`)
	fmt.Println()
	fmt.Println("2. Get a specific investigation:")
	fmt.Println(`{
  investigation(name: "sample-investigation") {
    metadata {
      name
      uid
    }
    spec {
      title
      overviewNote
      collectables {
        title
        origin
        timeRange {
          from
          to
        }
      }
    }
  }
}`)
	fmt.Println()
	fmt.Println("3. Get investigations by user (using custom relationship resolver):")
	fmt.Println(`{
  investigationsByUser(userUid: "alice-uid") {
    metadata {
      name
    }
    spec {
      title
      isFavorite
    }
  }
}`)
	fmt.Println()
	fmt.Println("4. Create a new investigation:")
	fmt.Println(`mutation {
  createInvestigation(input: {
    metadata: {
      name: "new-investigation"
      namespace: "default"
    }
    spec: {
      title: "New Investigation"
      createdByProfile: {
        uid: "bob-uid"
        name: "Bob Johnson"
        gravatarUrl: "https://gravatar.com/avatar/bob"
      }
      hasCustomName: true
      isFavorite: false
      overviewNote: "Created via GraphQL"
      collectables: []
      viewMode: {
        mode: "compact"
        showComments: false
        showTooltips: true
      }
    }
  }) {
    metadata {
      name
      uid
    }
    spec {
      title
    }
  }
}`)
	
	log.Fatal(http.ListenAndServe(":8080", nil))
} 