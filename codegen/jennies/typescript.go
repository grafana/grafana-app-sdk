package jennies

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"

	"cuelang.org/go/cue"

	"github.com/grafana/codejen"
	"github.com/grafana/cog"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/resource"
)

type TypeScriptResourceTypes struct {
	GenerateOnlyCurrent bool
}

func (*TypeScriptResourceTypes) JennyName() string { return "TypeScriptResourceTypes" }

func (t *TypeScriptResourceTypes) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0, 1)
	if t.GenerateOnlyCurrent {
		for _, kind := range codegen.PreferredVersionKinds(appManifest) {
			if !kind.Codegen.TS.Enabled {
				return nil, nil
			}
			b, err := t.generateObjectFile(&kind, strings.ToLower(kind.MachineName)+"_")
			if err != nil {
				return nil, err
			}
			files = append(files, codejen.File{
				RelativePath: fmt.Sprintf("%s/%s_object_gen.ts", kind.MachineName, kind.MachineName),
				Data:         b,
				From:         []codejen.NamedJenny{t},
			})
		}
	} else {
		for version, kind := range codegen.VersionedKinds(appManifest) {
			if !kind.Codegen.TS.Enabled {
				continue
			}
			b, err := t.generateObjectFile(&kind, "")
			if err != nil {
				return nil, err
			}
			files = append(files, codejen.File{
				RelativePath: fmt.Sprintf("%s/%s/%s_object_gen.ts", kind.MachineName, version.Name(), kind.MachineName),
				Data:         b,
				From:         []codejen.NamedJenny{t},
			})
		}
	}
	return files, nil
}

func (*TypeScriptResourceTypes) generateObjectFile(vk *codegen.VersionedKind, tsTypePrefix string) ([]byte, error) {
	metadata := templates.ResourceTSTemplateMetadata{
		TypeName:     exportField(vk.Kind),
		Subresources: make([]templates.SubresourceMetadata, 0),
		FilePrefix:   tsTypePrefix,
	}

	it, err := vk.Schema.Fields()
	if err != nil {
		return nil, err
	}
	for it.Next() {
		if it.Selector().String() == "spec" || it.Selector().String() == "metadata" { //nolint:goconst
			continue
		}
		metadata.Subresources = append(metadata.Subresources, templates.SubresourceMetadata{
			TypeName: exportField(it.Selector().String()),
			JSONName: it.Selector().String(),
		})
	}

	tsBytes := &bytes.Buffer{}
	err = templates.WriteResourceTSType(metadata, tsBytes)
	if err != nil {
		return nil, err
	}
	return tsBytes.Bytes(), nil
}

// TypeScriptTypes is a one-to-many jenny that generates one or more TypeScript types for a kind.
// Each type is a specific version of the kind where codegen.frontend is true.
// If GenerateOnlyCurrent is true, then all other versions of the kind will be ignored and only
// the kind.Propertoes().Current version will be used for TypeScript type generation
// (this will impact the generated file path).
type TypeScriptTypes struct {
	// GenerateOnlyCurrent should be set to true if you only want to generate code for the kind.Properties().Current version.
	// This will affect the package and path(s) of the generated file(s).
	GenerateOnlyCurrent bool

	// Depth represents the tree depth for creating go types from fields. A Depth of 0 will return one go type
	// (plus any definitions used by that type), a Depth of 1 will return a file with a go type for each top-level field
	// (plus any definitions encompassed by each type), etc. Note that types are _not_ generated for fields above the Depth
	// level--i.e. a Depth of 1 will generate go types for each field within the KindVersion.Schema, but not a type for the
	// Schema itself. Because Depth results in recursive calls, the highest value is bound to a max of GoTypesMaxDepth.
	Depth int

	// NamingDepth determines how types are named in relation to Depth. If Depth <= NamingDepth, the go types are named
	// using the field name of the type. Otherwise, Names used are prefixed by field names between Depth and NamingDepth.
	// Typically, a value of 0 is "safest" for NamingDepth, as it prevents overlapping names for types.
	// However, if you know that your fields have unique names up to a certain depth, you may configure this to be higher.
	NamingDepth int
}

var _ codejen.OneToMany[codegen.AppManifest] = &TypeScriptTypes{}

func (TypeScriptTypes) JennyName() string {
	return "TypeScriptTypes"
}

func (j TypeScriptTypes) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0, 1)
	if j.GenerateOnlyCurrent {
		for version, kind := range codegen.PreferredVersionKinds(appManifest) {
			if !kind.Codegen.TS.Enabled {
				return nil, nil
			}

			generated, err := j.generateFiles(version.Name(), &kind, kind.Kind, "", strings.ToLower(kind.MachineName)+"_")
			if err != nil {
				return nil, err
			}
			files = append(files, generated...)
		}
	} else {
		for version, kind := range codegen.VersionedKinds(appManifest) {
			if !kind.Codegen.TS.Enabled {
				continue
			}

			generated, err := j.generateFiles(version.Name(), &kind, kind.Kind, fmt.Sprintf("%s/%s", kind.MachineName, version.Name()), "")
			if err != nil {
				return nil, err
			}
			files = append(files, generated...)
		}
	}
	return files, nil
}

func (j TypeScriptTypes) generateFiles(version string, kind *codegen.VersionedKind, name, pathPrefix, prefix string) (codejen.Files, error) {
	if j.Depth > 0 {
		return j.generateFilesAtDepth(kind.Schema, version, kind, 0, pathPrefix, prefix)
	}

	tsBytes, err := generateTypescriptBytes(kind.Schema, ToPackageName(version), exportField(sanitizeLabelString(name)), cog.TypescriptConfig{
		ImportsMap:        kind.Codegen.TS.Config.ImportsMap,
		EnumsAsUnionTypes: kind.Codegen.TS.Config.EnumsAsUnionTypes,
	})
	if err != nil {
		return nil, err
	}
	return codejen.Files{codejen.File{
		Data:         tsBytes,
		RelativePath: fmt.Sprintf(path.Join(pathPrefix, "%stypes.gen.ts"), prefix),
		From:         []codejen.NamedJenny{j},
	}}, nil
}

func (j TypeScriptTypes) generateFilesAtDepth(v cue.Value, version string, vk *codegen.VersionedKind, currDepth int, pathPrefix string, prefix string) (codejen.Files, error) {
	if currDepth == j.Depth {
		selectors := TrimPathPrefix(v.Path(), vk.Schema.Path()).Selectors()
		fieldName := make([]string, 0, len(selectors))
		for _, s := range selectors {
			fieldName = append(fieldName, s.String())
		}
		tsBytes, err := generateTypescriptBytes(v, ToPackageName(version), exportField(strings.Join(fieldName, "")), cog.TypescriptConfig{
			ImportsMap:        vk.Codegen.TS.Config.ImportsMap,
			EnumsAsUnionTypes: vk.Codegen.TS.Config.EnumsAsUnionTypes,
		})
		if err != nil {
			return nil, err
		}
		return codejen.Files{codejen.File{
			Data:         tsBytes,
			RelativePath: fmt.Sprintf(path.Join(pathPrefix, "%stypes.%s.gen.ts"), prefix, strings.Join(fieldName, "_")),
			From:         []codejen.NamedJenny{j},
		}}, nil
	}

	it, err := v.Fields()
	if err != nil {
		return nil, err
	}

	files := make(codejen.Files, 0, 1)
	for it.Next() {
		f, err := j.generateFilesAtDepth(it.Value(), version, vk, currDepth+1, pathPrefix, prefix)
		if err != nil {
			return nil, err
		}
		files = append(files, f...)
	}
	return files, nil
}

func generateTypescriptBytes(v cue.Value, packageName string, name string, tsConfig cog.TypescriptConfig) ([]byte, error) {
	files, err := cog.TypesFromSchema().
		CUEValue(packageName, v, cog.ForceEnvelope(name)).
		Typescript(tsConfig).
		Run(context.Background())
	if err != nil {
		return nil, err
	}

	if len(files) != 1 {
		return nil, fmt.Errorf("expected one file to be generated, got %d", len(files))
	}

	return files[0].Data, nil
}

// TypeScriptRTKAPI generates one RTK Query API per app manifest version, covering every kind with TypeScript
// codegen enabled and all custom routes, matching the shape of the generated clients in grafana/grafana's
// @grafana/api-clients package. The APIs share the createBaseQuery module written by TypeScriptBaseQuery.
type TypeScriptRTKAPI struct {
	GenerateOnlyCurrent bool
}

func (*TypeScriptRTKAPI) JennyName() string { return "TypeScriptRTKAPI" }

func (t *TypeScriptRTKAPI) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0)
	for _, version := range appManifest.Versions() {
		if t.GenerateOnlyCurrent && version.Name() != appManifest.Properties().PreferredVersion {
			continue
		}
		generated, err := t.generateVersion(appManifest, version)
		if err != nil {
			return nil, err
		}
		files = append(files, generated...)
	}
	return files, nil
}

//nolint:funlen,gocognit
func (t *TypeScriptRTKAPI) generateVersion(appManifest codegen.AppManifest, version codegen.Version) (codejen.Files, error) {
	group := appManifest.Properties().FullGroup
	dir := path.Join(appManifest.Properties().Group, version.Name())
	kindImport := func(kind codegen.VersionedKind) string {
		return fmt.Sprintf("../../%s/%s/%s_object_gen", kind.MachineName, version.Name(), kind.MachineName)
	}
	metadata := templates.TSRTKAPITemplateMetadata{
		Group:           group,
		Version:         version.Name(),
		ReducerPath:     rtkReducerPath(group, version.Name()),
		BaseQueryImport: "../../createBaseQuery.gen",
	}
	if t.GenerateOnlyCurrent {
		dir = appManifest.Properties().Group
		metadata.BaseQueryImport = "../createBaseQuery.gen"
		kindImport = func(kind codegen.VersionedKind) string {
			return fmt.Sprintf("../%s/%s_%s_object_gen", kind.MachineName, strings.ToLower(kind.MachineName), kind.MachineName)
		}
	}

	files := make(codejen.Files, 0)
	for _, kind := range version.Kinds() {
		if !kind.Codegen.TS.Enabled {
			continue
		}
		lowerKind := strings.ToLower(kind.Kind[:1]) + kind.Kind[1:]
		k := templates.TSRTKKind{
			TypeName:     exportField(kind.Kind),
			Kind:         kind.Kind,
			MachineName:  kind.MachineName,
			Plural:       kind.PluralMachineName,
			ArgName:      lowerKind,
			ObjectImport: kindImport(kind),
			Subresources: make([]templates.SubresourceMetadata, 0),
		}
		if kind.Scope == string(resource.ClusterScope) {
			k.URLPrefix = "${CLUSTER_URL}"
		}
		it, err := kind.Schema.Fields()
		if err != nil {
			return nil, err
		}
		for it.Next() {
			if it.Selector().String() == "spec" || it.Selector().String() == "metadata" {
				continue
			}
			k.Subresources = append(k.Subresources, templates.SubresourceMetadata{
				TypeName: exportField(it.Selector().String()),
				JSONName: it.Selector().String(),
			})
		}
		routes, routeFiles, err := t.routes(kind.Routes, strings.ToLower(kind.MachineName)+"_", "", dir, version.Name(), kind.Codegen.TS.Config, &metadata)
		if err != nil {
			return nil, fmt.Errorf("kind %s: %w", kind.Kind, err)
		}
		k.Routes = routes
		files = append(files, routeFiles...)
		metadata.Kinds = append(metadata.Kinds, k)
	}
	if len(metadata.Kinds) == 0 {
		return nil, nil
	}

	for _, scoped := range []struct {
		routes map[string]map[string]codegen.CustomRoute
		prefix string
	}{
		{version.Routes().Namespaced, ""},
		{version.Routes().Cluster, "${CLUSTER_URL}"},
	} {
		routes, routeFiles, err := t.routes(scoped.routes, "", scoped.prefix, dir, version.Name(), codegen.KindCodegenTSConfig{}, &metadata)
		if err != nil {
			return nil, fmt.Errorf("version %s: %w", version.Name(), err)
		}
		metadata.Routes = append(metadata.Routes, routes...)
		files = append(files, routeFiles...)
	}

	b := &bytes.Buffer{}
	if err := templates.WriteTSRTKAPI(metadata, b); err != nil {
		return nil, err
	}
	files = append(files, codejen.File{RelativePath: path.Join(dir, "api_gen.ts"), Data: b.Bytes(), From: []codejen.NamedJenny{t}})
	return files, nil
}

// routes builds route metadata for a route map, generating TypeScript types for each route's query, body and
// response schema into dir. Type imports are appended to metadata.TypeImports.
//
//nolint:funlen
func (t *TypeScriptRTKAPI) routes(routeMap map[string]map[string]codegen.CustomRoute, filePrefix, urlPrefix, dir, version string,
	tsConfig codegen.KindCodegenTSConfig, metadata *templates.TSRTKAPITemplateMetadata) ([]templates.TSRTKRoute, codejen.Files, error) {
	files := make(codejen.Files, 0)
	routes := make([]templates.TSRTKRoute, 0)
	paths := make([]string, 0, len(routeMap))
	for p := range routeMap {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	cfg := cog.TypescriptConfig{ImportsMap: tsConfig.ImportsMap, EnumsAsUnionTypes: tsConfig.EnumsAsUnionTypes}
	for _, routePath := range paths {
		methods := make([]string, 0, len(routeMap[routePath]))
		for m := range routeMap[routePath] {
			methods = append(methods, m)
		}
		slices.Sort(methods)
		for _, method := range methods {
			route := routeMap[routePath][method]
			if route.Name == "" {
				route.Name = defaultRouteName(method, routePath)
			}
			if !route.Response.Exists() {
				return nil, nil, fmt.Errorf("custom route %s %s: response is required", method, routePath)
			}
			typeName := exportField(route.Name)
			fileBase := filePrefix + strings.ToLower(route.Name)
			r := templates.TSRTKRoute{
				Name:      route.Name,
				TypeName:  typeName,
				Method:    strings.ToUpper(method),
				Path:      pathParamRegex.ReplaceAllString(routePath, "${queryArg.$1}"),
				URLPrefix: urlPrefix,
				IsGet:     strings.ToUpper(method) == http.MethodGet,
				IsQuery:   strings.ToUpper(method) == http.MethodGet,
			}
			// Response
			responseName := typeName + "Response"
			if route.ResponseMetadata.TypeMeta {
				responseName = typeName + "ResponseBody"
			}
			tsBytes, err := generateTypescriptBytes(route.Response, ToPackageName(version), responseName, cfg)
			if err != nil {
				return nil, nil, fmt.Errorf("custom route %s: response: %w", route.Name, err)
			}
			file := fileBase + "_response.gen"
			files = append(files, codejen.File{RelativePath: path.Join(dir, file+".ts"), Data: tsBytes, From: []codejen.NamedJenny{t}})
			metadata.TypeImports = append(metadata.TypeImports, templates.TSRTKTypeImport{TypeName: responseName, File: file})
			r.ResponseType = responseName
			if route.ResponseMetadata.TypeMeta {
				r.ResponseType = responseName + " & { apiVersion?: string; kind?: string"
				if route.ResponseMetadata.ObjectMeta {
					r.ResponseType += "; metadata?: Record<string, unknown>"
				} else if route.ResponseMetadata.ListMeta {
					r.ResponseType += "; metadata?: ListMeta"
				}
				r.ResponseType += " }"
			}
			// Query params
			if route.Request.Query.Exists() {
				paramsName := typeName + "RequestParams"
				tsBytes, err := generateTypescriptBytes(route.Request.Query, ToPackageName(version), paramsName, cfg)
				if err != nil {
					return nil, nil, fmt.Errorf("custom route %s: query: %w", route.Name, err)
				}
				file := fileBase + "_request_params.gen"
				files = append(files, codejen.File{RelativePath: path.Join(dir, file+".ts"), Data: tsBytes, From: []codejen.NamedJenny{t}})
				metadata.TypeImports = append(metadata.TypeImports, templates.TSRTKTypeImport{TypeName: paramsName, File: file})
				r.ParamsType = paramsName
				it, err := route.Request.Query.Fields(cue.Optional(true))
				if err != nil {
					return nil, nil, err
				}
				for it.Next() {
					r.Params = append(r.Params, it.Selector().Unquoted())
				}
			}
			// Body
			if route.Request.Body.Exists() {
				bodyName := typeName + "RequestBody"
				tsBytes, err := generateTypescriptBytes(route.Request.Body, ToPackageName(version), bodyName, cfg)
				if err != nil {
					return nil, nil, fmt.Errorf("custom route %s: body: %w", route.Name, err)
				}
				file := fileBase + "_request_body.gen"
				files = append(files, codejen.File{RelativePath: path.Join(dir, file+".ts"), Data: tsBytes, From: []codejen.NamedJenny{t}})
				metadata.TypeImports = append(metadata.TypeImports, templates.TSRTKTypeImport{TypeName: bodyName, File: file})
				r.BodyType = bodyName
				r.HasBody = true
			}
			r.HasArgs = r.HasBody || len(r.Params) > 0 || strings.Contains(r.Path, "${queryArg.") || filePrefix != ""
			routes = append(routes, r)
		}
	}
	return routes, files, nil
}

var pathParamRegex = regexp.MustCompile(`\{([A-Za-z0-9_]+)}`)

// rtkReducerPath mirrors the reducer path convention in grafana/grafana's @grafana/api-clients:
// the group without its ".grafana.app" suffix in lowerCamel, then "API", then the version,
// e.g. "notificationsAlertingAPIv0alpha1".
func rtkReducerPath(group, version string) string {
	label := strings.TrimSuffix(group, ".grafana.app")
	var sb strings.Builder
	for i, part := range strings.Split(label, ".") {
		if part == "" {
			continue
		}
		if i == 0 {
			_, _ = sb.WriteString(part)
			continue
		}
		_, _ = sb.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return sb.String() + "API" + version
}

// TypeScriptBaseQuery writes the single shared createBaseQuery.gen.ts module the generated RTK Query APIs import.
// It is a no-op when no kind has TypeScript codegen enabled.
type TypeScriptBaseQuery struct{}

func (*TypeScriptBaseQuery) JennyName() string { return "TypeScriptBaseQuery" }

func (t *TypeScriptBaseQuery) Generate(manifests ...codegen.AppManifest) (*codejen.File, error) {
	enabled := false
	for _, m := range manifests {
		for _, kind := range codegen.VersionedKinds(m) {
			if kind.Codegen.TS.Enabled {
				enabled = true
			}
		}
	}
	if !enabled {
		return nil, nil
	}
	b := &bytes.Buffer{}
	if err := templates.WriteTSBaseQuery(b); err != nil {
		return nil, err
	}
	return &codejen.File{RelativePath: "createBaseQuery.gen.ts", Data: b.Bytes(), From: []codejen.NamedJenny{t}}, nil
}
