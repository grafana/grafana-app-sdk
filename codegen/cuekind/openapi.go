package cuekind

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"sigs.k8s.io/yaml"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/internal/openapiutil"
)

func (p *Parser) loadManifestOpenAPI(manifest *codegen.SimpleManifest) error {
	for _, version := range manifest.Versions() {
		if inline := version.Properties().InlineOpenAPI; inline != nil {
			doc := maps.Clone(inline)
			doc["openapi"] = "3.0.3"
			data, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			routes, err := parseOpenAPIRoutes(data, manifest.Properties().FullGroup, version.Name())
			if err != nil {
				return fmt.Errorf("inline OpenAPI (version %s): %w", version.Name(), err)
			}
			manifest.AllVersions[version.Name()].ImportedRoutes = routes
		}
		name := version.Properties().ImportOpenAPIFile
		if p.files == nil {
			if name != "" {
				return fmt.Errorf("version %s: importOpenAPIFile requires a filesystem; use LoadCue", version.Name())
			}
			continue
		}
		if name == "" {
			for _, ext := range []string{"json", "yaml", "yml"} {
				candidate := "openapi." + version.Name() + "." + ext
				_, err := fs.Stat(p.files, candidate)
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err != nil {
					return fmt.Errorf("OpenAPI %s: %w", candidate, err)
				}
				if name != "" {
					return fmt.Errorf("version %s: multiple OpenAPI files found (%s, %s); set importOpenAPIFile explicitly", version.Name(), name, candidate)
				}
				name = candidate
			}
		}
		if name == "" {
			continue
		}
		data, err := fs.ReadFile(p.files, name)
		if err != nil {
			return fmt.Errorf("OpenAPI %s: %w", name, err)
		}
		routes, err := parseOpenAPIRoutes(data, manifest.Properties().FullGroup, version.Name())
		if err != nil {
			return fmt.Errorf("OpenAPI %s (version %s): %w", name, version.Name(), err)
		}
		target := &manifest.AllVersions[version.Name()].ImportedRoutes
		target.Cluster, err = openapiutil.MergePaths(target.Cluster, routes.Cluster)
		if err != nil {
			return err
		}
		target.Namespaced, err = openapiutil.MergePaths(target.Namespaced, routes.Namespaced)
		if err != nil {
			return err
		}
		if target.Schemas == nil {
			target.Schemas = routes.Schemas
		} else {
			maps.Copy(target.Schemas, routes.Schemas)
		}
	}
	return nil
}

func parseOpenAPIRoutes(data []byte, group, version string) (app.ManifestVersionRoutes, error) { //nolint:staticcheck // ManifestVersionRoutes is deprecated; imported routes still use it until manifests read openapi
	routes := app.ManifestVersionRoutes{} //nolint:staticcheck // ManifestVersionRoutes is deprecated; imported routes still use it until manifests read openapi
	var raw map[string]any
	if err := yaml.UnmarshalStrict(data, &raw); err != nil {
		return routes, err
	}
	apiVersion, ok := raw["openapi"].(string)
	if !ok || !strings.HasPrefix(apiVersion, "3.0.") {
		return routes, fmt.Errorf("expected an OpenAPI 3.0 document, got %q", apiVersion)
	}
	if paths, ok := raw["paths"].(map[string]any); ok {
		for name, item := range paths {
			if strings.HasPrefix(name, "x-") {
				continue
			}
			if !strings.HasPrefix(name, "/") {
				return routes, fmt.Errorf("path %q must start with /", name)
			}
			if _, ok := item.(map[string]any); !ok {
				return routes, fmt.Errorf("path %q must be an object", name)
			}
		}
	}
	// The manifest retains components.schemas, but has no other component maps.
	// Inline references to parameters, responses, etc. while preserving schema refs.
	expanded, err := expandOpenAPIRefs(raw, raw, nil, openAPIObject)
	if err != nil {
		return routes, err
	}
	data, err = json.Marshal(expanded)
	if err != nil {
		return routes, err
	}
	var doc spec3.OpenAPI
	if err := json.Unmarshal(data, &doc); err != nil {
		return routes, err
	}
	if doc.Components != nil {
		for name, schema := range doc.Components.Schemas {
			if schema == nil {
				return routes, fmt.Errorf("schema %q must be an object", name)
			}
			if routes.Schemas == nil {
				routes.Schemas = make(map[string]spec.Schema)
			}
			routes.Schemas[name] = *schema
		}
	}
	if doc.Paths == nil {
		return routes, nil
	}
	for _, source := range slices.Sorted(maps.Keys(doc.Paths.Paths)) {
		path := doc.Paths.Paths[source]
		if path == nil {
			return routes, fmt.Errorf("path %q must be an object", source)
		}
		target, namespaced, err := openAPIRoutePath(source, group, version)
		if err != nil {
			return routes, err
		}
		if routes.Cluster == nil {
			routes.Cluster = make(map[string]spec3.PathProps)
			routes.Namespaced = make(map[string]spec3.PathProps)
		}
		destination := routes.Cluster
		if namespaced {
			destination = routes.Namespaced
		}
		if _, exists := destination[target]; exists {
			return routes, fmt.Errorf("multiple paths resolve to manifest route %q", target)
		}
		if err := inheritPathParameters(&path.PathProps); err != nil {
			return routes, fmt.Errorf("path %q: %w", source, err)
		}
		addImplicitPathParameters(source, &path.PathProps)
		destination[target] = path.PathProps
	}
	return routes, nil
}

// Materialize shared parameters before merging documents. The legacy route
// installer reads operation parameters only, and retaining shared parameters
// would also apply one document's defaults to another document's operations.
func inheritPathParameters(path *spec3.PathProps) error {
	for _, parameter := range path.Parameters {
		if parameter == nil {
			return errors.New("parameter must be an object")
		}
	}
	for _, operation := range []*spec3.Operation{path.Get, path.Put, path.Post, path.Delete, path.Options, path.Head, path.Patch, path.Trace} {
		if operation == nil {
			continue
		}
		for _, parameter := range operation.Parameters {
			if parameter == nil {
				return errors.New("parameter must be an object")
			}
		}
		for _, parameter := range path.Parameters {
			if !slices.ContainsFunc(operation.Parameters, func(existing *spec3.Parameter) bool {
				return existing.Name == parameter.Name && existing.In == parameter.In
			}) {
				operation.Parameters = append(operation.Parameters, parameter)
			}
		}
	}
	path.Parameters = nil
	return nil
}

// Use the source path before namespace and resource prefixes are removed.
// Explicit parameters at either OpenAPI level take precedence over defaults.
func addImplicitPathParameters(source string, path *spec3.PathProps) {
	for _, name := range []string{"namespace", "name"} {
		if !strings.Contains(source, "{"+name+"}") {
			continue
		}
		matches := func(parameter *spec3.Parameter) bool {
			return parameter != nil && parameter.In == "path" && parameter.Name == name
		}
		if slices.ContainsFunc(path.Parameters, matches) {
			continue
		}
		for _, operation := range []*spec3.Operation{path.Get, path.Put, path.Post, path.Delete, path.Options, path.Head, path.Patch, path.Trace} {
			if operation == nil || slices.ContainsFunc(operation.Parameters, matches) {
				continue
			}
			operation.Parameters = append(operation.Parameters, &spec3.Parameter{
				ParameterProps: spec3.ParameterProps{
					Name: name, In: "path", Required: true,
					Schema: &spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"string"}}},
				},
			})
		}
	}
}

func openAPIRoutePath(source, group, version string) (string, bool, error) {
	target := source
	if strings.HasPrefix(target, "/apis/") {
		prefix := "/apis/" + group + "/" + version + "/"
		if !strings.HasPrefix(target, prefix) {
			return "", false, fmt.Errorf("path %q does not belong to %s/%s", source, group, version)
		}
		target = "/" + strings.TrimPrefix(target, prefix)
	}
	const namespacePrefix = "/namespaces/{namespace}/"
	namespaced := strings.HasPrefix(target, namespacePrefix)
	if namespaced {
		target = "/" + strings.TrimPrefix(target, namespacePrefix)
	}
	if target == "/" || !strings.HasPrefix(target, "/") {
		return "", false, fmt.Errorf("path %q must identify a custom route", source)
	}
	return target, namespaced, nil
}

type openAPIRefContext uint8

const (
	openAPIObject openAPIRefContext = iota
	openAPINamedObjects
	openAPIExtendedMap
	openAPIExamples
	openAPIExample
)

// Names in maps such as responses and properties are user-defined. A response
// named "default" is not a schema default, nor is a property named "example"
// literal example data. Example Objects, in turn, contain literal data in value.
func (context openAPIRefContext) child(key string) openAPIRefContext {
	switch context {
	case openAPINamedObjects, openAPIExtendedMap:
		return openAPIObject
	case openAPIExamples:
		return openAPIExample
	default:
		switch key {
		case "examples":
			return openAPIExamples
		case "paths", "responses":
			return openAPIExtendedMap
		case "schemas", "properties", "parameters", "requestBodies", "headers", "links", "callbacks", "securitySchemes", "content", "encoding":
			return openAPINamedObjects
		default:
			return openAPIObject
		}
	}
}

func (context openAPIRefContext) literal(key string) bool {
	if context == openAPIExtendedMap {
		return strings.HasPrefix(key, "x-")
	}
	if context == openAPINamedObjects || context == openAPIExamples {
		return false
	}
	return key == "example" || key == "default" || key == "enum" || strings.HasPrefix(key, "x-") ||
		(context == openAPIExample && key == "value")
}

func expandOpenAPIRefs(value any, root map[string]any, stack []string, context openAPIRefContext) (any, error) {
	switch value := value.(type) {
	case map[string]any:
		if ref, ok := value["$ref"].(string); ok && (context == openAPIObject || context == openAPIExample) {
			if !strings.HasPrefix(ref, "#/") {
				return nil, fmt.Errorf("reference %q must be local to the OpenAPI document", ref)
			}
			var target any = root
			for segment := range strings.SplitSeq(strings.TrimPrefix(ref, "#/"), "/") {
				object, ok := target.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("unresolved reference %q", ref)
				}
				segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
				target, ok = object[segment]
				if !ok {
					return nil, fmt.Errorf("unresolved reference %q", ref)
				}
			}
			if strings.HasPrefix(ref, "#/components/schemas/") {
				return value, nil
			}
			if slices.Contains(stack, ref) {
				return nil, fmt.Errorf("cyclic non-schema reference %q", ref)
			}
			return expandOpenAPIRefs(target, root, append(stack, ref), context)
		}
		result := make(map[string]any, len(value))
		for key, item := range value {
			// These values are instance data, not OpenAPI references.
			if context.literal(key) {
				result[key] = item
				continue
			}
			expanded, err := expandOpenAPIRefs(item, root, stack, context.child(key))
			if err != nil {
				return nil, err
			}
			result[key] = expanded
		}
		return result, nil
	case []any:
		result := make([]any, len(value))
		for idx, item := range value {
			expanded, err := expandOpenAPIRefs(item, root, stack, openAPIObject)
			if err != nil {
				return nil, err
			}
			result[idx] = expanded
		}
		return result, nil
	default:
		return value, nil
	}
}
