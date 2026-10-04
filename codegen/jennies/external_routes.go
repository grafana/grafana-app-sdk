package jennies

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
)

// Keep external documents optional without extending the Version interface for
// parsers that do not support them.
type externalRouteVersion interface {
	ExternalRoutes() app.ManifestVersionRoutes
}

func externalRoutes(version codegen.Version) app.ManifestVersionRoutes {
	if source, ok := version.(externalRouteVersion); ok {
		return source.ExternalRoutes()
	}
	return app.ManifestVersionRoutes{}
}

// Resource paths belong to the kind's routes, whose paths are relative to an
// individual object. Other paths remain version-level custom routes.
func externalKindRoutes(kind app.ManifestVersionKind, source app.ManifestVersionRoutes) map[string]spec3.PathProps {
	paths := source.Namespaced
	if kind.Scope == "Cluster" {
		paths = source.Cluster
	}
	prefix := "/" + kind.Resource() + "/{name}"
	result := make(map[string]spec3.PathProps)
	for path, props := range paths {
		if strings.HasPrefix(path, prefix+"/") {
			result[strings.TrimPrefix(path, prefix)] = props
		}
	}
	return result
}

//nolint:staticcheck // Preserve deprecated route fields for manifest compatibility.
func mergeExternalVersionRoutes(version *app.ManifestVersion, source app.ManifestVersionRoutes) error {
	// Work on copies so repeated generation sees the same imported document.
	source.Cluster = maps.Clone(source.Cluster)
	source.Namespaced = maps.Clone(source.Namespaced)
	for idx := range version.Kinds {
		kind := &version.Kinds[idx]
		paths := externalKindRoutes(*kind, source)
		if len(paths) == 0 {
			continue
		}
		var err error
		kind.Routes, err = mergeExternalPaths(kind.Routes, paths)
		if err != nil {
			return err
		}
		// Kind route references are resolved against the kind's schema document.
		// As with CUE kind routes, these schemas follow IncludeSchemas.
		if kind.Schema != nil {
			schemas := maps.Clone(kind.Schema.AsOpenAPI3SchemasMap())
			for name, schema := range source.Schemas {
				schemas[name] = schema
			}
			kind.Schema, err = app.VersionSchemaFromMap(schemas, kind.Kind)
			if err != nil {
				return err
			}
		}
		remaining := source.Namespaced
		if kind.Scope == "Cluster" {
			remaining = source.Cluster
		}
		for path := range paths {
			delete(remaining, "/"+kind.Resource()+"/{name}"+path)
		}
	}
	return mergeExternalRoutes(&version.Routes, source)
}

func mergeExternalRoutes(target *app.ManifestVersionRoutes, source app.ManifestVersionRoutes) error {
	var err error
	target.Cluster, err = mergeExternalPaths(target.Cluster, source.Cluster)
	if err != nil {
		return err
	}
	target.Namespaced, err = mergeExternalPaths(target.Namespaced, source.Namespaced)
	if err != nil {
		return err
	}
	if len(source.Schemas) > 0 {
		if target.Schemas == nil {
			target.Schemas = make(map[string]spec.Schema)
		}
		maps.Copy(target.Schemas, source.Schemas)
	}
	return nil
}

// Merge at the path-field level: an imported operation replaces the same CUE
// operation, while methods absent from the document are retained.
func mergeExternalPaths(target, source map[string]spec3.PathProps) (map[string]spec3.PathProps, error) {
	if len(source) == 0 {
		return target, nil
	}
	if target == nil {
		target = make(map[string]spec3.PathProps)
	}
	for path, props := range source {
		base, err := json.Marshal(target[path])
		if err != nil {
			return nil, err
		}
		override, err := json.Marshal(props)
		if err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(base, &fields); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(override, &fields); err != nil {
			return nil, err
		}
		merged, err := json.Marshal(fields)
		if err != nil {
			return nil, err
		}
		var result spec3.PathProps
		if err := json.Unmarshal(merged, &result); err != nil {
			return nil, err
		}
		target[path] = result
	}
	return target, nil
}

// Imported operations have no generated request/response types. Exclude them
// from Go associations, including operations that override a CUE definition.
//
//nolint:staticcheck // Preserve deprecated route fields for manifest compatibility.
func manifestTypeVersions(manifest codegen.AppManifest, data *app.ManifestData) ([]app.ManifestVersion, map[string]bool) {
	versions := make([]app.ManifestVersion, len(data.Versions))
	copy(versions, data.Versions)
	imported := make(map[string]bool)
	for idx, version := range manifest.Versions() {
		routes := externalRoutes(version)
		if len(routes.Cluster)+len(routes.Namespaced)+len(routes.Schemas) == 0 {
			continue
		}
		imported[version.Name()] = true
		versions[idx].Kinds = slices.Clone(versions[idx].Kinds)
		for kindIdx := range versions[idx].Kinds {
			kind := &versions[idx].Kinds[kindIdx]
			kind.Routes = withoutExternalOperations(kind.Routes, externalKindRoutes(*kind, routes))
		}
		versions[idx].Routes.Cluster = withoutExternalOperations(versions[idx].Routes.Cluster, routes.Cluster)
		versions[idx].Routes.Namespaced = withoutExternalOperations(versions[idx].Routes.Namespaced, routes.Namespaced)
	}
	return versions, imported
}

func withoutExternalOperations(paths, external map[string]spec3.PathProps) map[string]spec3.PathProps {
	result := maps.Clone(paths)
	for path, source := range external {
		props := result[path]
		targets := []**spec3.Operation{&props.Get, &props.Put, &props.Post, &props.Delete, &props.Options, &props.Head, &props.Patch, &props.Trace}
		operations := []*spec3.Operation{source.Get, source.Put, source.Post, source.Delete, source.Options, source.Head, source.Patch, source.Trace}
		for idx, op := range operations {
			if op != nil {
				*targets[idx] = nil
			}
		}
		if len(getRouteNames(&props)) == 0 && props.Trace == nil {
			delete(result, path)
			continue
		}
		result[path] = props
	}
	return result
}
