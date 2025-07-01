package subgraph

import (
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/graphql-go/graphql"
)

// ResourceGraphQLHandler provides resource-specific GraphQL handling
// Each app can implement this interface to provide custom type generation and conversion
type ResourceGraphQLHandler interface {
	// GetResourceKind returns the resource kind this handler supports
	GetResourceKind() resource.Kind

	// GetGraphQLFields returns additional GraphQL fields for this resource type
	// These fields will be added to the base metadata + spec structure
	GetGraphQLFields() graphql.Fields

	// ConvertResourceToGraphQL converts a resource.Object to GraphQL format
	// Returns a map of field names to values that will be merged with base fields
	ConvertResourceToGraphQL(obj resource.Object) map[string]interface{}
}

// ResourceHandlerRegistry manages resource-specific GraphQL handlers
type ResourceHandlerRegistry struct {
	handlers map[string]ResourceGraphQLHandler
}

// NewResourceHandlerRegistry creates a new registry
func NewResourceHandlerRegistry() *ResourceHandlerRegistry {
	return &ResourceHandlerRegistry{
		handlers: make(map[string]ResourceGraphQLHandler),
	}
}

// RegisterHandler registers a resource handler
func (r *ResourceHandlerRegistry) RegisterHandler(handler ResourceGraphQLHandler) {
	kind := handler.GetResourceKind()
	key := kind.Group() + "/" + kind.Version() + "/" + kind.Kind()
	r.handlers[key] = handler
}

// GetHandler retrieves a handler for the given resource kind
func (r *ResourceHandlerRegistry) GetHandler(kind resource.Kind) ResourceGraphQLHandler {
	key := kind.Group() + "/" + kind.Version() + "/" + kind.Kind()
	return r.handlers[key]
}

// GetHandlerByKindName retrieves a handler by kind name (case-insensitive)
func (r *ResourceHandlerRegistry) GetHandlerByKindName(kindName string) ResourceGraphQLHandler {
	for _, handler := range r.handlers {
		if handler.GetResourceKind().Kind() == kindName {
			return handler
		}
	}
	return nil
}

// GetAllHandlers returns all registered handlers
func (r *ResourceHandlerRegistry) GetAllHandlers() []ResourceGraphQLHandler {
	handlers := make([]ResourceGraphQLHandler, 0, len(r.handlers))
	for _, handler := range r.handlers {
		handlers = append(handlers, handler)
	}
	return handlers
}
