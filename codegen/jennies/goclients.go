package jennies

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
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
	GroupByKind bool
}

func (*ClientJenny) JennyName() string {
	return "ClientJenny"
}

func (r *ClientJenny) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0)

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
	files := make(codejen.Files, 0)
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
			CustomRoutes: make([]templates.GoClientCustomRoute, 0),
		}

		md.CustomRoutes, err = getCustomRoutes(kind.Routes)
		if err != nil {
			return nil, fmt.Errorf("getting custom routes for kind %s: %w", kind.Kind, err)
		}
		slices.SortFunc(md.CustomRoutes, func(a, b templates.GoClientCustomRoute) int {
			return strings.Compare(a.TypeName, b.TypeName)
		})

		b := bytes.Buffer{}
		err = templates.WriteGoResourceClient(md, &b)
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		formatted, err = imports.Process("", formatted, &imports.Options{
			Comments: true,
		})
		if err != nil {
			return nil, err
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
	files := make(codejen.Files, 0)
	for _, version := range appManifest.Versions() {
		md := templates.GoCustomRouteClientMetadata{
			PackageName:      ToPackageName(version.Name()),
			NamespacedRoutes: make([]templates.GoClientCustomRoute, 0),
			ClusterRoutes:    make([]templates.GoClientCustomRoute, 0),
			Group:            appManifest.Properties().FullGroup,
			Version:          version.Name(),
		}

		var err error
		md.NamespacedRoutes, err = getCustomRoutes(version.Routes().Namespaced)
		if err != nil {
			return nil, err
		}

		md.ClusterRoutes, err = getCustomRoutes(version.Routes().Cluster)
		if err != nil {
			return nil, err
		}

		if len(md.NamespacedRoutes) == 0 && len(md.ClusterRoutes) == 0 {
			continue
		}

		b := bytes.Buffer{}
		err = templates.WriteGoCustomRouteClient(md, &b)
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		formatted, err = imports.Process("", formatted, &imports.Options{
			Comments: true,
		})
		if err != nil {
			return nil, err
		}
		files = append(files, codejen.File{
			RelativePath: filepath.Join(ToPackageName(appManifest.Properties().Group), ToPackageName(version.Name()), "client_gen.go"),
			Data:         formatted,
			From:         []codejen.NamedJenny{r},
		})
	}
	return files, nil
}

func getCustomRouteInfo(customRoute codegen.CustomRoute) (templates.GoClientCustomRoute, error) {
	md := templates.GoClientCustomRoute{
		TypeName:  toExportedFieldName(customRoute.Name),
		HasParams: customRoute.Request.Query.Exists(),
		HasBody:   customRoute.Request.Body.Exists(),
	}
	if md.HasParams {
		md.ParamValues = make([]templates.GoCustomRouteParamValues, 0)
		it, err := customRoute.Request.Query.Fields()
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

func getCustomRoutes(routes map[string]map[string]codegen.CustomRoute) ([]templates.GoClientCustomRoute, error) {
	var errs error
	var md []templates.GoClientCustomRoute
	for cpath, methods := range routes {
		for method, route := range methods {
			if route.Name == "" {
				route.Name = defaultRouteName(method, cpath)
			}
			crmd, err := getCustomRouteInfo(route)
			if err != nil {
				errs = errors.Join(errs, err)
				continue
			}
			crmd.Path = cpath
			crmd.Method = method
			md = append(md, crmd)
		}
	}
	return md, errs
}
