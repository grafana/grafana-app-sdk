package jennies

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/grafana/codejen"
	"golang.org/x/tools/imports"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type ClientJenny struct {
	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	// When GroupByKind is false, subresource types (such as spec and status) are prefixed with the kind name,
	// i.e. generating FooSpec instead of Spec for kind.Name() = "Foo" and Depth=1
	GroupByKind        bool
	SkipImportsProcess bool
	// ProjectRepo is the Go module of the project and GoGenPath the path in it that code is generated
	// in, which are where the client imports the types of kinds from when generating by kind.
	ProjectRepo string
	GoGenPath   string
}

func (*ClientJenny) JennyName() string {
	return "ClientJenny"
}

func (r *ClientJenny) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0, 1)

	groupVersionFiles, err := r.generateCustomRouteClients(appManifest)
	if err != nil {
		return nil, err
	}
	files = append(files, groupVersionFiles...)

	resourceFiles, err := r.generateResourceClients(appManifest)
	if err != nil {
		return nil, err
	}
	files = append(files, resourceFiles...)

	return files, nil
}

func (r *ClientJenny) generateResourceClients(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0, 1)
	for version, kind := range codegen.VersionedKinds(appManifest) {
		if !kind.Codegen.Go.Enabled {
			continue
		}
		prefix := exportField(kind.Kind)
		if r.GroupByKind {
			prefix = ""
		}
		subresources := make([]templates.GoResourceClientSubresource, 0)
		it, err := kind.Schema.Fields()
		if err != nil {
			return nil, err
		}
		for it.Next() {
			sr := it.Selector().String()
			if sr == "metadata" || sr == "spec" { //nolint:goconst
				continue
			}
			subresources = append(subresources, templates.GoResourceClientSubresource{
				FieldName:   exportField(sr),
				Subresource: sr,
			})
		}
		// Sort for consistent output in the template
		slices.SortFunc(subresources, func(a, b templates.GoResourceClientSubresource) int {
			return strings.Compare(a.FieldName, b.FieldName)
		})
		md := templates.GoResourceClientMetadata{
			PackageName:  ToPackageName(version.Name()),
			KindName:     exportField(kind.Kind),
			KindPrefix:   prefix,
			Subresources: subresources,
		}

		md.CustomRoutes, err = getCustomRoutes(kind.Routes, r.responseType(appManifest, version, kind.Kind))
		if err != nil {
			return nil, fmt.Errorf("getting custom routes for kind %s: %w", kind.Kind, err)
		}
		slices.SortFunc(md.CustomRoutes, func(a, b templates.GoClientCustomRoute) int {
			return strings.Compare(a.TypeName, b.TypeName)
		})
		md.Imports = responseImports(md.CustomRoutes)

		b := bytes.Buffer{}
		err = templates.WriteGoResourceClient(md, &b)
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		if !r.SkipImportsProcess {
			formatted, err = imports.Process("", formatted, &imports.Options{
				Comments: true,
			})
			if err != nil {
				return nil, err
			}
		}
		files = append(files, codejen.File{
			RelativePath: filepath.Join(getGeneratedPathForKind(r.GroupByKind, appManifest.Properties().Group, kind, version.Name()), fmt.Sprintf("%s_client_gen.go", kind.MachineName)),
			Data:         formatted,
			From:         []codejen.NamedJenny{r},
		})
	}
	return files, nil
}

func (r *ClientJenny) generateCustomRouteClients(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0, 1)
	for _, version := range appManifest.Versions() {
		md := templates.GoCustomRouteClientMetadata{
			PackageName: ToPackageName(version.Name()),
			Group:       appManifest.Properties().FullGroup,
			Version:     version.Name(),
		}

		var err error
		md.NamespacedRoutes, err = getCustomRoutes(version.Routes().Namespaced, r.responseType(appManifest, version, ""))
		if err != nil {
			return nil, err
		}

		md.ClusterRoutes, err = getCustomRoutes(version.Routes().Cluster, r.responseType(appManifest, version, ""))
		if err != nil {
			return nil, err
		}

		if len(md.NamespacedRoutes) == 0 && len(md.ClusterRoutes) == 0 {
			continue
		}
		md.Imports = responseImports(md.NamespacedRoutes, md.ClusterRoutes)

		b := bytes.Buffer{}
		err = templates.WriteGoCustomRouteClient(md, &b)
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		if !r.SkipImportsProcess {
			formatted, err = imports.Process("", formatted, &imports.Options{
				Comments: true,
			})
			if err != nil {
				return nil, err
			}
		}
		files = append(files, codejen.File{
			RelativePath: filepath.Join(ToPackageName(appManifest.Properties().Group), ToPackageName(version.Name()), "client_gen.go"),
			Data:         formatted,
			From:         []codejen.NamedJenny{r},
		})
	}
	return files, nil
}

// responseTypeOf is the Go type that the client method of a custom route returns, and the package
// it is imported from if that is not the package of the client.
type responseTypeOf func(codegen.CustomRoute) (string, *templates.GoImport, error)

// responseType is the Go type that the client method of a custom route returns, which is the
// response type generated for the route, or else the type generated for the kind it returns. The
// type of a kind is in the package of its group and version, unless generating by kind, when it is
// in the package of the kind and version, which is imported unless the client is in it too.
func (r *ClientJenny) responseType(appManifest codegen.AppManifest, version codegen.Version, owner string) responseTypeOf {
	return func(route codegen.CustomRoute) (string, *templates.GoImport, error) {
		if route.ResponseKind == "" {
			return toExportedFieldName(route.Name) + "Response", nil, nil
		}
		i := slices.IndexFunc(version.Kinds(), func(k codegen.VersionedKind) bool { return k.Kind == route.ResponseKind })
		if i < 0 {
			return "", nil, fmt.Errorf("custom route %s returns kind %s, which is not a kind of version %s", route.Name, route.ResponseKind, version.Name())
		}
		kind := version.Kinds()[i]
		if !kind.Codegen.Go.Enabled {
			return "", nil, fmt.Errorf("custom route %s returns kind %s, which does not generate Go code", route.Name, route.ResponseKind)
		}
		if !r.GroupByKind || route.ResponseKind == owner {
			return exportField(route.ResponseKind), nil, nil
		}
		if r.ProjectRepo == "" {
			return "", nil, fmt.Errorf("custom route %s returns kind %s, whose package is imported from the project's Go module, which is not set", route.Name, route.ResponseKind)
		}
		imp := &templates.GoImport{
			Alias: strings.ToLower(ToPackageName(kind.MachineName) + ToPackageName(version.Name())),
			Path:  path.Join(r.ProjectRepo, r.GoGenPath, filepath.ToSlash(GetGeneratedGoTypePath(true, appManifest.Properties().Group, version.Name(), kind.MachineName))),
		}
		return imp.Alias + "." + exportField(route.ResponseKind), imp, nil
	}
}

// responseImports are the packages the response types of routes are imported from, without repeats.
func responseImports(routes ...[]templates.GoClientCustomRoute) []templates.GoImport {
	var found []templates.GoImport
	for _, rs := range routes {
		for _, route := range rs {
			if route.ResponseImport != nil && !slices.Contains(found, *route.ResponseImport) {
				found = append(found, *route.ResponseImport)
			}
		}
	}
	slices.SortFunc(found, func(a, b templates.GoImport) int { return strings.Compare(a.Path, b.Path) })
	return found
}

func getCustomRouteInfo(customRoute codegen.CustomRoute, responseType responseTypeOf) (templates.GoClientCustomRoute, error) {
	respType, respImport, err := responseType(customRoute)
	if err != nil {
		return templates.GoClientCustomRoute{}, err
	}
	md := templates.GoClientCustomRoute{
		TypeName:       toExportedFieldName(customRoute.Name),
		ResponseType:   respType,
		ResponseImport: respImport,
		HasParams:      customRoute.Request.QuerySchema().Exists(),
		HasBody:        customRoute.Request.Body.Exists(),
	}
	if md.HasParams {
		md.ParamValues = make([]templates.GoCustomRouteParamValues, 0)
		it, err := customRoute.Request.QuerySchema().Fields()
		if err != nil {
			return md, err
		}
		for it.Next() {
			md.ParamValues = append(md.ParamValues, templates.GoCustomRouteParamValues{
				Key:       it.Selector().String(),
				FieldName: exportField(it.Selector().String()),
			})
		}
	}
	return md, nil
}

func getCustomRoutes(routeMap map[string]map[string]codegen.CustomRoute, responseType responseTypeOf) ([]templates.GoClientCustomRoute, error) {
	var errs error
	routes := make([]templates.GoClientCustomRoute, 0)
	for cpath, methods := range routeMap {
		for method, route := range methods {
			if route.Name == "" {
				route.Name = defaultRouteName(method, cpath)
			}
			crmd, err := getCustomRouteInfo(route, responseType)
			if err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			crmd.Path = cpath
			crmd.Method = method
			routes = append(routes, crmd)
		}
	}
	return routes, errs
}
