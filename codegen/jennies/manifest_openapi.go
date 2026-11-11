package jennies

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
)

// routeSchemaSet collects the route schemas of a version into the single
// components.schemas map, tracking which kind or version defined each name.
type routeSchemaSet struct {
	schemas map[string]spec.Schema
	owners  map[string]string
}

func newRouteSchemaSet() *routeSchemaSet {
	return &routeSchemaSet{
		schemas: make(map[string]spec.Schema),
		owners:  make(map[string]string),
	}
}

// add returns an error if name is already defined by another owner with a different schema.
func (s *routeSchemaSet) add(owner, name string, schema spec.Schema) error {
	if existing, ok := s.schemas[name]; ok && s.owners[name] != owner {
		existingJSON, err := json.Marshal(existing)
		if err != nil {
			return err
		}
		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			return err
		}
		if !bytes.Equal(existingJSON, schemaJSON) {
			return fmt.Errorf("custom route schema %q is defined differently by %s and %s; move the shared type to the inline openapi.components.schemas section", name, s.owners[name], owner)
		}
		return nil
	}
	s.schemas[name] = schema
	s.owners[name] = owner
	return nil
}

// buildVersionOpenAPI collects custom routes at paths relative to the version root.
func buildVersionOpenAPI(version *app.ManifestVersion, kindRouteSchemas *routeSchemaSet) error {
	paths := make(map[string]spec3.PathProps)
	add := func(prefix string, routes map[string]spec3.PathProps) {
		for path, props := range routes {
			fullPath := prefix + "/" + strings.TrimPrefix(path, "/")
			// Legacy routes are relative to their scope, so their implicit path
			// parameters must be declared when exposing version-relative paths.
			props.Parameters = slices.Clone(props.Parameters)
			for _, name := range []string{"namespace", "name"} {
				if !strings.Contains(fullPath, "{"+name+"}") || slices.ContainsFunc(props.Parameters, func(p *spec3.Parameter) bool {
					return p != nil && p.In == "path" && p.Name == name
				}) {
					continue
				}
				props.Parameters = append(props.Parameters, &spec3.Parameter{ParameterProps: spec3.ParameterProps{
					Name: name, In: "path", Required: true,
					Schema: &spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"string"}}},
				}})
			}
			paths[fullPath] = props
		}
	}
	routes := version.Routes //nolint:staticcheck // Routes is deprecated; this is where it is translated to OpenAPI paths
	add("", routes.Cluster)
	add("/namespaces/{namespace}", routes.Namespaced)
	for _, kind := range version.Kinds {
		prefix := "/" + kind.Resource + "/{name}"
		if kind.Scope != "Cluster" {
			prefix = "/namespaces/{namespace}" + prefix
		}
		add(prefix, kind.Routes)
	}
	for name, schema := range routes.Schemas {
		if err := kindRouteSchemas.add("version "+version.Name+" routes", name, schema); err != nil {
			return err
		}
	}
	version.OpenAPI = app.ManifestVersionOpenAPI{
		Paths:      paths,
		Components: app.ManifestVersionOpenAPIComponents{Schemas: kindRouteSchemas.schemas},
	}
	return nil
}

// kindResourceSchemas returns the components that describe the resource of a kind, as the standard
// Kubernetes calls for it return it, which a route can reference with ResponseKind. The component
// named for the kind is the resource, with its apiVersion, kind and metadata. What it references of
// the kind's schema, such as its spec, is a component named for the kind and the schema, such as
// Foo.spec, so that the kinds of a version do not share the names of their fields.
func kindResourceSchemas(vk codegen.VersionedKind) (map[string]spec.Schema, error) {
	raw := make(map[string]any)
	if err := kindOpenAPISchemas(vk, raw); err != nil {
		return nil, fmt.Errorf("schema of kind %s: %w", vk.Kind, err)
	}
	schemas := make(map[string]spec.Schema, len(raw))
	for name, value := range raw {
		b, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var schema spec.Schema
		if err := json.Unmarshal(b, &schema); err != nil {
			return nil, fmt.Errorf("schema %s of kind %s: %w", name, vk.Kind, err)
		}
		schema.SchemaProps = prefixReferences(schema.SchemaProps, vk.Kind+".", map[string]spec.SchemaProps{})
		if name != vk.Kind {
			schemas[vk.Kind+"."+name] = schema
			continue
		}
		schema.Type = []string{"object"}
		schema.Properties[keyAPIVersion] = apiVersionPropSchema
		schema.Properties[keyKind] = kindPropSchema
		schema.Properties[keyMetadata] = objectMetaPropSchema
		schema.Required = append([]string{keyAPIVersion, keyKind, keyMetadata}, schema.Required...)
		schemas[vk.Kind] = schema
	}
	return schemas, nil
}

// routeResponseKinds are the kinds of a version that its routes return with ResponseKind, by name.
func routeResponseKinds(version codegen.Version) (map[string]codegen.VersionedKind, error) {
	names := map[string]struct{}{}
	visit := func(routes map[string]map[string]codegen.CustomRoute) {
		for _, methods := range routes {
			for _, route := range methods {
				if route.ResponseKind != "" {
					names[route.ResponseKind] = struct{}{}
				}
			}
		}
	}
	for _, kind := range version.Kinds() {
		visit(kind.Routes)
	}
	visit(version.Routes().Namespaced)
	visit(version.Routes().Cluster)
	kinds := make(map[string]codegen.VersionedKind, len(names))
	for name := range names {
		i := slices.IndexFunc(version.Kinds(), func(k codegen.VersionedKind) bool { return k.Kind == name })
		if i < 0 {
			return nil, fmt.Errorf("a route has responseKind %q, which is not a kind of version %s", name, version.Name())
		}
		kinds[name] = version.Kinds()[i]
	}
	return kinds, nil
}
