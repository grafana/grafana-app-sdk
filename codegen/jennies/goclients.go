package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"

	"github.com/grafana/codejen"

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
	for _, version := range appManifest.Versions() {
		for _, kind := range version.Kinds() {
			if !kind.Codegen.Go.Enabled {
				continue
			}
			md := templates.GoResourceClientMetadata{
				PackageName:  ToPackageName(version.Name()),
				KindName:     exportField(kind.Kind),
				CustomRoutes: make([]templates.GoResourceClientCustomRoute, 0),
			}
			for cpath, methods := range kind.Routes {
				for method, route := range methods {
					if route.Name == "" {
						route.Name = defaultRouteName(method, cpath)
					}
					crmd, err := r.getCustomRouteInfo(route)
					if err != nil {
						return nil, err
					}
					crmd.Path = cpath
					crmd.Method = method
					md.CustomRoutes = append(md.CustomRoutes, crmd)
				}
			}

			b := bytes.Buffer{}
			err := templates.WriteGoResourceClient(md, &b)
			if err != nil {
				return nil, err
			}
			formatted, err := format.Source(b.Bytes())
			if err != nil {
				return nil, err
			}
			files = append(files, codejen.File{
				RelativePath: filepath.Join(getGeneratedPathForKind(r.GroupByKind, appManifest.Properties().Group, kind, version.Name()), fmt.Sprintf("%s_client_gen.go", kind.MachineName)),
				Data:         formatted,
				From:         []codejen.NamedJenny{r},
			})
		}
	}
	return files, nil
}

func (*ResourceClientJenny) getCustomRouteInfo(customRoute codegen.CustomRoute) (templates.GoResourceClientCustomRoute, error) {
	md := templates.GoResourceClientCustomRoute{
		TypeName:  toExportedFieldName(customRoute.Name),
		HasParams: customRoute.Request.Query.Exists(),
		HasBody:   customRoute.Request.Body.Exists(),
	}
	if md.HasParams {
		md.ParamValues = make([]templates.GoResourceClientParamValues, 0)
		it, err := customRoute.Request.Query.Fields()
		if err != nil {
			return md, err
		}
		for it.Next() {
			md.ParamValues = append(md.ParamValues, templates.GoResourceClientParamValues{
				Key:       it.Selector().String(),
				FieldName: exportField(it.Selector().String()),
			})
		}
	}
	return md, nil
}
