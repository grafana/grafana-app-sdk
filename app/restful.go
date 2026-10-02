package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/emicklei/go-restful/v3"
)

type RestfulRoutesProvider interface {
	ProvideRoutes(version string) (*RestfulRoutes, error)
}

type RestfulRoutes struct {
	// Routes hosted directly under /apis/{group}/{version}.
	Cluster *restful.WebService

	// Routes hosted under /apis/{group}/{version}/namespaces/{namespace}.
	Namespaced *restful.WebService

	// Sub-resource routes keyed by manifest Kind name (for example, "Widget").
	// The manifest supplies the resource plural and scope.
	Kinds map[string]*restful.WebService
}

// WebService combines routes under the full API prefix for the requested served
// manifest version. Source service paths are relative to their cluster, namespace, or kind
// prefix. The source services are not modified.
//
// Use route-level filters on the source services: go-restful does not expose
// service-level filters for copying. Private per-route transport settings are
// likewise not copied; the resulting service uses go-restful's defaults.
func (r *RestfulRoutes) WebService(version string, manifest *ManifestData) (*restful.WebService, error) {
	if r == nil {
		return nil, fmt.Errorf("routes and manifest must not be nil")
	}
	if version == "" || strings.Contains(version, "/") {
		return nil, fmt.Errorf("a valid API version is required")
	}
	if manifest == nil {
		return nil, fmt.Errorf("manifest must not be nil")
	}

	var selected *ManifestVersion
	for i := range manifest.Versions {
		if manifest.Versions[i].Name == version && manifest.Versions[i].Served {
			selected = &manifest.Versions[i]
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("manifest version %q is not declared or not served", version)
	}

	ws := new(restful.WebService)
	seen := make(map[string]bool)
	if err := appendRestfulRoutes(ws, r.Cluster, "", seen); err != nil {
		return nil, err
	}
	if err := appendRestfulRoutes(ws, r.Namespaced, "/namespaces/{namespace}", seen); err != nil {
		return nil, err
	}
	matchedKinds := make(map[string]bool)
	for _, kind := range selected.Kinds {
		rws, ok := r.Kinds[kind.Kind]
		if !ok {
			continue
		}
		matchedKinds[kind.Kind] = true
		prefix := ""
		switch kind.Scope {
		case "Namespaced":
			prefix = "/namespaces/{namespace}"
		case "Cluster": // no prefix
		default:
			return nil, fmt.Errorf("kind %q has unsupported scope %q", kind.Kind, kind.Scope)
		}
		// Rebuild each child route under the parent. Changing a WebService's
		// Path alone does not rebase its already-built routes.
		if err := appendRestfulRoutes(ws, rws, prefix+"/"+kind.Resource()+"/{name}", seen); err != nil {
			return nil, err
		}
	}
	for kind := range r.Kinds {
		if !matchedKinds[kind] {
			return nil, fmt.Errorf("route kind %q is not declared in manifest version %q", kind, version)
		}
	}
	return ws, nil
}

func appendRestfulRoutes(dst, src *restful.WebService, prefix string, seen map[string]bool) error {
	if src == nil {
		return nil
	}
	for _, route := range src.Routes() {
		// Do not path.Clean route patterns: they can contain regular expressions.
		path := prefix + "/" + strings.TrimPrefix(route.Path, "/")
		key := route.Method + " " + dst.RootPath() + path
		if seen[key] {
			return fmt.Errorf("duplicate route %s", key)
		}
		seen[key] = true
		builder := dst.Method(route.Method).Path(path).To(route.Function).
			Consumes(route.Consumes...).Produces(route.Produces...).
			Doc(route.Doc).Notes(route.Notes).Operation(route.Operation).
			Writes(route.WriteSamples...)
		if route.ReadSample != nil {
			builder.Reads(route.ReadSample)
		}
		if len(route.WriteSamples) == 0 && route.WriteSample != nil {
			builder.Writes(route.WriteSample)
		}
		parameters := append(slices.Clone(route.ParameterDocs), src.PathParameters()...)
		for _, name := range []string{"namespace", "name"} {
			if strings.Contains(prefix, "{"+name+"}") && !slices.ContainsFunc(parameters, func(p *restful.Parameter) bool {
				return p.Data().Name == name && p.Data().Kind == restful.PathParameterKind
			}) {
				parameters = append(parameters, restful.PathParameter(name, name).DataType("string"))
			}
		}
		for _, parameter := range parameters {
			builder.Param(parameter)
		}
		for _, filter := range route.Filters {
			builder.Filter(filter)
		}
		for _, condition := range route.If {
			builder.If(condition)
		}
		for code, response := range route.ResponseErrors {
			builder.ReturnsWithHeaders(code, response.Message, response.Model, response.Headers)
		}
		if route.DefaultResponse != nil {
			builder.DefaultReturns(route.DefaultResponse.Message, route.DefaultResponse.Model)
		}
		for key, value := range route.Metadata {
			builder.Metadata(key, value)
		}
		for key, value := range route.Extensions {
			builder.AddExtension(key, value)
		}
		if route.Deprecated {
			builder.Deprecate()
		}
		dst.Route(builder)
	}
	return nil
}
