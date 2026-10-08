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
		prefix := "/" + kind.Resource() + "/{name}"
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
