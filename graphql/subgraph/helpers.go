package subgraph

import (
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/graphql-go/graphql"
)

// SimpleResourceHandler provides a basic implementation of ResourceGraphQLHandler
// that only requires minimal configuration. Use this for simple resources that
// don't need complex GraphQL field mapping.
type SimpleResourceHandler struct {
	kind        resource.Kind
	extraFields graphql.Fields
	converter   func(obj resource.Object) map[string]interface{}
}

// NewSimpleResourceHandler creates a simple resource handler with minimal configuration
func NewSimpleResourceHandler(kind resource.Kind) *SimpleResourceHandler {
	return &SimpleResourceHandler{
		kind: kind,
	}
}

// WithExtraFields adds additional GraphQL fields to the base resource type
func (h *SimpleResourceHandler) WithExtraFields(fields graphql.Fields) *SimpleResourceHandler {
	h.extraFields = fields
	return h
}

// WithConverter sets a custom converter function for resource-to-GraphQL conversion
func (h *SimpleResourceHandler) WithConverter(converter func(obj resource.Object) map[string]interface{}) *SimpleResourceHandler {
	h.converter = converter
	return h
}

// GetResourceKind implements ResourceGraphQLHandler
func (h *SimpleResourceHandler) GetResourceKind() resource.Kind {
	return h.kind
}

// GetGraphQLFields implements ResourceGraphQLHandler
func (h *SimpleResourceHandler) GetGraphQLFields() graphql.Fields {
	if h.extraFields != nil {
		return h.extraFields
	}
	return graphql.Fields{}
}

// ConvertResourceToGraphQL implements ResourceGraphQLHandler
func (h *SimpleResourceHandler) ConvertResourceToGraphQL(obj resource.Object) map[string]interface{} {
	if h.converter != nil {
		return h.converter(obj)
	}
	return map[string]interface{}{}
}

// CreateSubgraphWithHandlers is a convenience function for creating a subgraph
// with multiple resource handlers. This is useful when you want to register
// handlers for multiple resource types in a single call.
func CreateSubgraphWithHandlers(config SubgraphProviderConfig, handlers ...ResourceGraphQLHandler) (GraphQLSubgraph, error) {
	if config.ResourceHandlers == nil {
		config.ResourceHandlers = NewResourceHandlerRegistry()
	}

	for _, handler := range handlers {
		config.ResourceHandlers.RegisterHandler(handler)
	}

	return CreateSubgraphFromConfig(config)
}

// AddHandlersToExistingRegistry adds multiple handlers to an existing registry
func AddHandlersToExistingRegistry(registry *ResourceHandlerRegistry, handlers ...ResourceGraphQLHandler) {
	for _, handler := range handlers {
		registry.RegisterHandler(handler)
	}
}
