package jennies

import (
	"maps"
	"slices"
	"strings"

	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
)

// buildVersionOpenAPI collects custom routes at paths relative to the version root.
//
//nolint:staticcheck // Preserve deprecated route fields for manifest compatibility.
func buildVersionOpenAPI(version *app.ManifestVersion, kindRouteSchemas map[string]spec.SchemaProps) {
	paths := make(map[string]spec3.PathProps)
	schemas := make(map[string]spec.Schema)
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
	add("", version.Routes.Cluster)
	add("/namespaces/{namespace}", version.Routes.Namespaced)
	for _, kind := range version.Kinds {
		prefix := "/" + kind.Resource() + "/{name}"
		if kind.Scope != "Cluster" {
			prefix = "/namespaces/{namespace}" + prefix
		}
		add(prefix, kind.Routes)
	}
	for name, props := range kindRouteSchemas {
		schemas[name] = spec.Schema{SchemaProps: props}
	}
	maps.Copy(schemas, version.Routes.Schemas)
	version.OpenAPI = app.ManifestVersionOpenAPI{
		Paths:      paths,
		Components: app.ManifestVersionOpenAPIComponents{Schemas: schemas},
	}
}
