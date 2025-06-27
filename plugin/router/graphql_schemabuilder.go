package router

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"

	"github.com/grafana/grafana-app-sdk/resource"
)

// SchemaBuilder builds GraphQL schemas from CUE definitions and resource Kinds
type SchemaBuilder struct {
	cueContext     *cue.Context
	typeRegistry   map[string]graphql.Type
	inputRegistry  map[string]graphql.Input
	objectRegistry map[string]*graphql.Object
}

// NewSchemaBuilder creates a new schema builder
func NewSchemaBuilder() *SchemaBuilder {
	return &SchemaBuilder{
		cueContext:     cuecontext.New(),
		typeRegistry:   make(map[string]graphql.Type),
		inputRegistry:  make(map[string]graphql.Input),
		objectRegistry: make(map[string]*graphql.Object),
	}
}

// BuildSchemaFromKinds builds a GraphQL schema from a collection of resource kinds
func (sb *SchemaBuilder) BuildSchemaFromKinds(collections map[string]resource.KindCollection) (graphql.Schema, error) {
	queryFields := graphql.Fields{}
	mutationFields := graphql.Fields{}

	// First pass: register all object types
	for _, collection := range collections {
		for _, kind := range collection.Kinds() {
			if err := sb.registerKindTypes(kind); err != nil {
				return graphql.Schema{}, fmt.Errorf("failed to register types for %s: %w", kind.Kind(), err)
			}
		}
	}

	// Second pass: build query and mutation fields
	for _, collection := range collections {
		for _, kind := range collection.Kinds() {
			kindName := kind.Kind()
			objectType := sb.objectRegistry[kindName]

			if objectType == nil {
				return graphql.Schema{}, fmt.Errorf("object type not found for kind %s", kindName)
			}

			// Add query fields
			queryFields[sb.camelCase(kindName)] = sb.buildGetField(kind, objectType)
			queryFields[sb.camelCase(kind.Plural())] = sb.buildListField(kind, objectType)

			// Add mutation fields
			mutationFields["create"+kindName] = sb.buildCreateField(kind, objectType)
			mutationFields["update"+kindName] = sb.buildUpdateField(kind, objectType)
			mutationFields["delete"+kindName] = sb.buildDeleteField(kind)

			// Add custom query fields for relationships and complex queries
			if err := sb.addRelationshipFields(kind, objectType, queryFields); err != nil {
				return graphql.Schema{}, fmt.Errorf("failed to add relationship fields for %s: %w", kindName, err)
			}
		}
	}

	// Create the schema
	return graphql.NewSchema(graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: queryFields,
		}),
		Mutation: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Mutation",
			Fields: mutationFields,
		}),
	})
}

// registerKindTypes registers GraphQL types for a resource kind
func (sb *SchemaBuilder) registerKindTypes(kind resource.Schema) error {
	// Create the main object type
	objectType, err := sb.buildObjectTypeFromKind(kind)
	if err != nil {
		return fmt.Errorf("failed to build object type: %w", err)
	}

	sb.objectRegistry[kind.Kind()] = objectType
	sb.typeRegistry[kind.Kind()] = objectType

	// Create input types for mutations
	inputType, err := sb.buildInputTypeFromKind(kind)
	if err != nil {
		return fmt.Errorf("failed to build input type: %w", err)
	}

	sb.inputRegistry[kind.Kind()+"Input"] = inputType

	return nil
}

// buildObjectTypeFromKind builds a GraphQL object type from a resource kind
func (sb *SchemaBuilder) buildObjectTypeFromKind(kind resource.Schema) (*graphql.Object, error) {
	obj := kind.ZeroValue()
	objType := reflect.TypeOf(obj).Elem()

	fields := graphql.Fields{}

	// Add metadata fields
	fields["metadata"] = &graphql.Field{
		Type: sb.getOrCreateObjectMetaType(),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			if obj, ok := p.Source.(resource.Object); ok {
				return sb.extractMetadata(obj), nil
			}
			return nil, nil
		},
	}

	// Add spec field
	if specField, ok := objType.FieldByName("Spec"); ok {
		specType, err := sb.buildTypeFromReflectType(specField.Type, kind.Kind()+"Spec")
		if err != nil {
			return nil, fmt.Errorf("failed to build spec type: %w", err)
		}

		fields["spec"] = &graphql.Field{
			Type: specType,
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				if obj, ok := p.Source.(resource.Object); ok {
					return obj.GetSpec(), nil
				}
				return nil, nil
			},
		}
	}

	// Add status field if it exists
	if statusField, ok := objType.FieldByName("Status"); ok {
		statusType, err := sb.buildTypeFromReflectType(statusField.Type, kind.Kind()+"Status")
		if err != nil {
			return nil, fmt.Errorf("failed to build status type: %w", err)
		}

		fields["status"] = &graphql.Field{
			Type: statusType,
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				if obj, ok := p.Source.(resource.Object); ok {
					if subresources := obj.GetSubresources(); subresources != nil {
						return subresources["status"], nil
					}
				}
				return nil, nil
			},
		}
	}

	return graphql.NewObject(graphql.ObjectConfig{
		Name:   kind.Kind(),
		Fields: fields,
	}), nil
}

// buildTypeFromReflectType builds a GraphQL type from a Go reflect.Type
func (sb *SchemaBuilder) buildTypeFromReflectType(goType reflect.Type, typeName string) (graphql.Type, error) {
	// Check if we already have this type
	if existingType, exists := sb.typeRegistry[typeName]; exists {
		return existingType, nil
	}

	switch goType.Kind() {
	case reflect.String:
		return graphql.String, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return graphql.Int, nil
	case reflect.Float32, reflect.Float64:
		return graphql.Float, nil
	case reflect.Bool:
		return graphql.Boolean, nil
	case reflect.Slice:
		elementType, err := sb.buildTypeFromReflectType(goType.Elem(), typeName+"Item")
		if err != nil {
			return nil, err
		}
		return graphql.NewList(elementType), nil
	case reflect.Ptr:
		return sb.buildTypeFromReflectType(goType.Elem(), typeName)
	case reflect.Map:
		// For maps, we'll use a JSON scalar
		return sb.getOrCreateJSONScalar(), nil
	case reflect.Struct:
		return sb.buildObjectTypeFromStruct(goType, typeName)
	case reflect.Interface:
		// For interfaces, use JSON scalar
		return sb.getOrCreateJSONScalar(), nil
	default:
		// Fallback to JSON scalar
		return sb.getOrCreateJSONScalar(), nil
	}
}

// buildObjectTypeFromStruct builds a GraphQL object type from a Go struct
func (sb *SchemaBuilder) buildObjectTypeFromStruct(structType reflect.Type, typeName string) (graphql.Type, error) {
	fields := graphql.Fields{}

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		
		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get field name from JSON tag or use field name
		fieldName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" && parts[0] != "-" {
				fieldName = parts[0]
			}
			// Skip fields marked as "-"
			if parts[0] == "-" {
				continue
			}
		}

		fieldType, err := sb.buildTypeFromReflectType(field.Type, typeName+field.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to build type for field %s: %w", field.Name, err)
		}

		fields[fieldName] = &graphql.Field{
			Type: fieldType,
			Resolve: func(fieldName string) graphql.FieldResolveFn {
				return func(p graphql.ResolveParams) (interface{}, error) {
					return sb.extractFieldValue(p.Source, fieldName), nil
				}
			}(fieldName),
		}
	}

	objectType := graphql.NewObject(graphql.ObjectConfig{
		Name:   typeName,
		Fields: fields,
	})

	sb.objectRegistry[typeName] = objectType
	sb.typeRegistry[typeName] = objectType

	return objectType, nil
}

// buildInputTypeFromKind builds a GraphQL input type from a resource kind
func (sb *SchemaBuilder) buildInputTypeFromKind(kind resource.Schema) (graphql.Input, error) {
	obj := kind.ZeroValue()
	objType := reflect.TypeOf(obj).Elem()

	fields := graphql.InputObjectConfigFieldMap{}

	// Add metadata input
	fields["metadata"] = &graphql.InputObjectFieldConfig{
		Type: sb.getOrCreateObjectMetaInputType(),
	}

	// Add spec input
	if specField, ok := objType.FieldByName("Spec"); ok {
		specInputType, err := sb.buildInputTypeFromReflectType(specField.Type, kind.Kind()+"SpecInput")
		if err != nil {
			return nil, fmt.Errorf("failed to build spec input type: %w", err)
		}

		fields["spec"] = &graphql.InputObjectFieldConfig{
			Type: specInputType,
		}
	}

	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   kind.Kind() + "Input",
		Fields: fields,
	}), nil
}

// buildInputTypeFromReflectType builds a GraphQL input type from a Go reflect.Type
func (sb *SchemaBuilder) buildInputTypeFromReflectType(goType reflect.Type, typeName string) (graphql.Input, error) {
	// Check if we already have this input type
	if existingType, exists := sb.inputRegistry[typeName]; exists {
		return existingType, nil
	}

	switch goType.Kind() {
	case reflect.String:
		return graphql.String, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return graphql.Int, nil
	case reflect.Float32, reflect.Float64:
		return graphql.Float, nil
	case reflect.Bool:
		return graphql.Boolean, nil
	case reflect.Slice:
		elementType, err := sb.buildInputTypeFromReflectType(goType.Elem(), typeName+"Item")
		if err != nil {
			return nil, err
		}
		return graphql.NewList(elementType), nil
	case reflect.Ptr:
		return sb.buildInputTypeFromReflectType(goType.Elem(), typeName)
	case reflect.Map:
		return sb.getOrCreateJSONScalar(), nil
	case reflect.Struct:
		return sb.buildInputObjectFromStruct(goType, typeName)
	case reflect.Interface:
		return sb.getOrCreateJSONScalar(), nil
	default:
		return sb.getOrCreateJSONScalar(), nil
	}
}

// buildInputObjectFromStruct builds a GraphQL input object from a Go struct
func (sb *SchemaBuilder) buildInputObjectFromStruct(structType reflect.Type, typeName string) (graphql.Input, error) {
	fields := graphql.InputObjectConfigFieldMap{}

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		
		if !field.IsExported() {
			continue
		}

		fieldName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" && parts[0] != "-" {
				fieldName = parts[0]
			}
			if parts[0] == "-" {
				continue
			}
		}

		fieldType, err := sb.buildInputTypeFromReflectType(field.Type, typeName+field.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to build input type for field %s: %w", field.Name, err)
		}

		fields[fieldName] = &graphql.InputObjectFieldConfig{
			Type: fieldType,
		}
	}

	inputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   typeName,
		Fields: fields,
	})

	sb.inputRegistry[typeName] = inputType

	return inputType, nil
}

// Helper methods for building common fields

func (sb *SchemaBuilder) buildGetField(kind resource.Schema, objectType *graphql.Object) *graphql.Field {
	return &graphql.Field{
		Type: objectType,
		Args: graphql.FieldConfigArgument{
			"name": &graphql.ArgumentConfig{
				Type: graphql.NewNonNull(graphql.String),
			},
			"namespace": &graphql.ArgumentConfig{
				Type: graphql.String,
			},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			// This will be resolved by the GraphQLRouter
			return nil, fmt.Errorf("resolver not implemented - should be set by GraphQLRouter")
		},
	}
}

func (sb *SchemaBuilder) buildListField(kind resource.Schema, objectType *graphql.Object) *graphql.Field {
	return &graphql.Field{
		Type: graphql.NewList(objectType),
		Args: graphql.FieldConfigArgument{
			"namespace": &graphql.ArgumentConfig{
				Type: graphql.String,
			},
			"labelSelector": &graphql.ArgumentConfig{
				Type: graphql.String,
			},
			"limit": &graphql.ArgumentConfig{
				Type: graphql.Int,
			},
			"offset": &graphql.ArgumentConfig{
				Type: graphql.Int,
			},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return nil, fmt.Errorf("resolver not implemented - should be set by GraphQLRouter")
		},
	}
}

func (sb *SchemaBuilder) buildCreateField(kind resource.Schema, objectType *graphql.Object) *graphql.Field {
	inputType := sb.inputRegistry[kind.Kind()+"Input"]
	return &graphql.Field{
		Type: objectType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{
				Type: graphql.NewNonNull(inputType),
			},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return nil, fmt.Errorf("resolver not implemented - should be set by GraphQLRouter")
		},
	}
}

func (sb *SchemaBuilder) buildUpdateField(kind resource.Schema, objectType *graphql.Object) *graphql.Field {
	inputType := sb.inputRegistry[kind.Kind()+"Input"]
	return &graphql.Field{
		Type: objectType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{
				Type: graphql.NewNonNull(inputType),
			},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return nil, fmt.Errorf("resolver not implemented - should be set by GraphQLRouter")
		},
	}
}

func (sb *SchemaBuilder) buildDeleteField(kind resource.Schema) *graphql.Field {
	return &graphql.Field{
		Type: graphql.Boolean,
		Args: graphql.FieldConfigArgument{
			"name": &graphql.ArgumentConfig{
				Type: graphql.NewNonNull(graphql.String),
			},
			"namespace": &graphql.ArgumentConfig{
				Type: graphql.String,
			},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return nil, fmt.Errorf("resolver not implemented - should be set by GraphQLRouter")
		},
	}
}

// addRelationshipFields adds fields for relationships between kinds
func (sb *SchemaBuilder) addRelationshipFields(kind resource.Schema, objectType *graphql.Object, queryFields graphql.Fields) error {
	// Add aggregation fields for investigations
	if kind.Kind() == "Investigation" {
		queryFields["investigationsByUser"] = &graphql.Field{
			Type: graphql.NewList(objectType),
			Args: graphql.FieldConfigArgument{
				"userId": &graphql.ArgumentConfig{
					Type: graphql.NewNonNull(graphql.String),
				},
				"limit": &graphql.ArgumentConfig{
					Type: graphql.Int,
				},
			},
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				return nil, fmt.Errorf("resolver not implemented - should be set by GraphQLRouter")
			},
		}
	}

	return nil
}

// Utility methods

func (sb *SchemaBuilder) getOrCreateObjectMetaType() *graphql.Object {
	if metaType, exists := sb.objectRegistry["ObjectMeta"]; exists {
		return metaType
	}

	metaType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ObjectMeta",
		Fields: graphql.Fields{
			"name":              &graphql.Field{Type: graphql.String},
			"namespace":         &graphql.Field{Type: graphql.String},
			"uid":               &graphql.Field{Type: graphql.String},
			"resourceVersion":   &graphql.Field{Type: graphql.String},
			"generation":        &graphql.Field{Type: graphql.Int},
			"labels":            &graphql.Field{Type: graphql.NewList(graphql.String)},
			"annotations":       &graphql.Field{Type: graphql.NewList(graphql.String)},
			"creationTimestamp": &graphql.Field{Type: graphql.String},
		},
	})

	sb.objectRegistry["ObjectMeta"] = metaType
	sb.typeRegistry["ObjectMeta"] = metaType
	return metaType
}

func (sb *SchemaBuilder) getOrCreateObjectMetaInputType() *graphql.InputObject {
	if inputType, exists := sb.inputRegistry["ObjectMetaInput"]; exists {
		if inputObj, ok := inputType.(*graphql.InputObject); ok {
			return inputObj
		}
	}

	inputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ObjectMetaInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"name":      &graphql.InputObjectFieldConfig{Type: graphql.String},
			"namespace": &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})

	sb.inputRegistry["ObjectMetaInput"] = inputType
	return inputType
}

func (sb *SchemaBuilder) getOrCreateJSONScalar() *graphql.Scalar {
	if jsonType, exists := sb.typeRegistry["JSON"]; exists {
		if scalar, ok := jsonType.(*graphql.Scalar); ok {
			return scalar
		}
	}

	jsonScalar := graphql.NewScalar(graphql.ScalarConfig{
		Name: "JSON",
		Description: "The `JSON` scalar type represents JSON values as specified by " +
			"[ECMA-404](http://www.ecma-international.org/publications/files/ECMA-ST/ECMA-404.pdf).",
		Serialize: func(value interface{}) interface{} {
			return value
		},
		ParseValue: func(value interface{}) interface{} {
			return value
		},
		ParseLiteral: func(valueAST ast.Value) interface{} {
			return valueAST.GetValue()
		},
	})

	sb.typeRegistry["JSON"] = jsonScalar
	return jsonScalar
}

func (sb *SchemaBuilder) extractMetadata(obj resource.Object) map[string]interface{} {
	return map[string]interface{}{
		"name":              obj.GetName(),
		"namespace":         obj.GetNamespace(),
		"uid":               string(obj.GetUID()),
		"resourceVersion":   obj.GetResourceVersion(),
		"generation":        obj.GetGeneration(),
		"labels":            obj.GetLabels(),
		"annotations":       obj.GetAnnotations(),
		"creationTimestamp": obj.GetCreationTimestamp().Format("2006-01-02T15:04:05Z"),
	}
}

func (sb *SchemaBuilder) extractFieldValue(source interface{}, fieldName string) interface{} {
	if source == nil {
		return nil
	}

	// Try to extract field value using reflection or JSON marshaling
	if sourceMap, ok := source.(map[string]interface{}); ok {
		return sourceMap[fieldName]
	}

	// Use reflection as fallback
	v := reflect.ValueOf(source)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() == reflect.Struct {
		field := v.FieldByName(fieldName)
		if field.IsValid() && field.CanInterface() {
			return field.Interface()
		}
	}

	// Last resort: marshal to JSON and extract field
	jsonData, err := json.Marshal(source)
	if err != nil {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil
	}

	return data[fieldName]
}

func (sb *SchemaBuilder) camelCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
} 