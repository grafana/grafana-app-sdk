package jennies

import (
	"bytes"
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

type ResourceClientJenny struct {
	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	// When GroupByKind is false, subresource types (such as spec and status) are prefixed with the kind name,
	// i.e. generating FooSpec instead of Spec for kind.Name() = "Foo" and Depth=1
	GroupByKind bool
}

func (*ResourceClientJenny) JennyName() string {
	return "ResourceClientJenny"
}

func (r *ResourceClientJenny) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
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
		for cpath, methods := range kind.Routes {
			for method, route := range methods {
				if route.Name == "" {
					route.Name = defaultRouteName(method, cpath)
				}
				crmd, err := getCustomRouteInfo(route)
				if err != nil {
					return nil, err
				}
				crmd.Path = cpath
				crmd.Method = method
				md.CustomRoutes = append(md.CustomRoutes, crmd)
			}
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

type GroupVersionClientJenny struct{}

func (*GroupVersionClientJenny) JennyName() string {
	return "GroupVersionClientJenny"
}

func (r *GroupVersionClientJenny) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0)
	for _, version := range appManifest.Versions() {
		md := templates.GoGroupVersionClientMetadata{
			PackageName:      ToPackageName(version.Name()),
			NamespacedRoutes: make([]templates.GoClientCustomRoute, 0),
			ClusterRoutes:    make([]templates.GoClientCustomRoute, 0),
		}

		for cpath, methods := range version.Routes().Namespaced {
			for method, route := range methods {
				if route.Name == "" {
					route.Name = defaultRouteName(method, cpath)
				}
				crmd, err := getCustomRouteInfo(route)
				if err != nil {
					return nil, err
				}
				crmd.Path = cpath
				crmd.Method = method
				md.NamespacedRoutes = append(md.NamespacedRoutes, crmd)
			}
		}

		for cpath, methods := range version.Routes().Cluster {
			for method, route := range methods {
				if route.Name == "" {
					route.Name = defaultRouteName(method, cpath)
				}
				crmd, err := getCustomRouteInfo(route)
				if err != nil {
					return nil, err
				}
				crmd.Path = cpath
				crmd.Method = method
				md.ClusterRoutes = append(md.ClusterRoutes, crmd)
			}
		}

		if len(md.NamespacedRoutes) == 0 && len(md.ClusterRoutes) == 0 {
			continue
		}

		b := bytes.Buffer{}
		err := templates.WriteGroupVersionClient(md, &b)
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
