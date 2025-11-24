package cuekind

import (
	"strings"

	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/jennies"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

// CRDGenerator returns a Generator which will create a CRD file
func CRDGenerator(outputEncoder jennies.CRDOutputEncoder, outputExtension string) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(jennies.CRDGenerator(outputEncoder, outputExtension))
	return g
}

// ResourceGenerator returns a collection of jennies which generate backend resource code from kinds.
// The `versioned` parameter governs whether to generate all versions where codegen.backend == true,
// or just generate code for the current version.
// If `groupKinds` is true, kinds within the same group will exist in the same package.
// When combined with `versioned`, each version package will contain all kinds in the group
// which have a schema for that version.
func ResourceGenerator(groupKinds bool) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(
		&jennies.GoTypes{
			Depth:                1,
			AddKubernetesCodegen: true,
			GroupByKind:          !groupKinds,
			AnyAsInterface:       true, // This is for compatibility with kube openAPI generator, which has issues with map[string]any
		},
		&jennies.ResourceObjectGenerator{
			SubresourceTypesArePrefixed: groupKinds,
			GroupByKind:                 !groupKinds,
		},
		&jennies.SchemaGenerator{
			GroupByKind: !groupKinds,
		},
		&jennies.CodecGenerator{
			GroupByKind: !groupKinds,
		},
		&jennies.Constants{
			GroupByKind: !groupKinds,
		},
	)
	return g
}

// BackendPluginGenerator returns a Generator which will produce boilerplate backend plugin code
func BackendPluginGenerator(projectRepo, generatedAPIPath string, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	pluginSecurePkgFiles, _ := templates.GetBackendPluginSecurePackageFiles()

	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(
		jennies.RouterHandlerCodeGenerator(projectRepo, generatedAPIPath, !groupKinds),
		jennies.StaticManyToOneGenerator[codegen.Kind](codejen.File{
			RelativePath: "plugin/secure/data.go",
			Data:         pluginSecurePkgFiles["data.go"],
		}),
		jennies.StaticManyToOneGenerator[codegen.Kind](codejen.File{
			RelativePath: "plugin/secure/middleware.go",
			Data:         pluginSecurePkgFiles["middleware.go"],
		}),
		jennies.StaticManyToOneGenerator[codegen.Kind](codejen.File{
			RelativePath: "plugin/secure/retriever.go",
			Data:         pluginSecurePkgFiles["retriever.go"],
		}),
		jennies.RouterCodeGenerator(projectRepo),
		jennies.BackendPluginMainGenerator(projectRepo, generatedAPIPath, !groupKinds),
	)
	return g
}

// TypeScriptResourceGenerator returns a Generator which generates TypeScript resource code.
func TypeScriptResourceGenerator() *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(&jennies.TypeScriptTypes{
		Depth: 1,
	}, &jennies.TypeScriptResourceTypes{})
	return g
}

// OperatorGenerator returns a Generator which will build out watcher boilerplate for each resource,
// and a main func to run an operator for the watchers.
func OperatorGenerator(projectRepo, codegenPath string, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer[codegen.Kind](namerFunc)
	g.Append(
		&jennies.OperatorKubeConfigJenny{},
		jennies.OperatorMainJenny(projectRepo, codegenPath, !groupKinds),
		&jennies.OperatorConfigJenny{},
	)
	return g
}

func AppGenerator(projectRepo, codegenPath string, manifestGoFilePath string, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	parts := strings.Split(projectRepo, "/")
	if len(parts) == 0 {
		parts = []string{""}
	}
	g := codejen.JennyListWithNamer[codegen.Kind](namerFunc)
	g.Append(
		jennies.WatcherJenny(projectRepo, codegenPath, !groupKinds),
		&jennies.AppGenerator{
			GroupByKind:         !groupKinds,
			ProjectRepo:         projectRepo,
			ProjectName:         parts[len(parts)-1],
			CodegenPath:         codegenPath,
			ManifestPackagePath: manifestGoFilePath,
		},
	)
	return g
}

func PostResourceGenerationGenerator(projectRepo, goGenPath string, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer[codegen.Kind](namerFunc)
	g.Append(&jennies.OpenAPI{
		GoModName:   projectRepo,
		GoGenPath:   goGenPath,
		GroupByKind: !groupKinds,
	})
	return g
}

func ManifestGenerator(encoder jennies.ManifestOutputEncoder, extension string, includeSchemas bool, crdCompatible bool) *codejen.JennyList[codegen.AppManifest] {
	g := codejen.JennyListWithNamer[codegen.AppManifest](namerFuncManifest)
	g.Append(&jennies.ManifestGenerator{
		Encoder:        encoder,
		FileExtension:  extension,
		IncludeSchemas: includeSchemas,
		CRDCompatible:  crdCompatible,
	})
	return g
}

func ManifestGoGenerator(pkg string, includeSchemas bool, projectRepo, goGenPath string, manifestGoFilePath string, groupKinds bool) *codejen.JennyList[codegen.AppManifest] {
	g := codejen.JennyListWithNamer[codegen.AppManifest](namerFuncManifest)
	g.Append(&jennies.ManifestGoGenerator{
		Package:         pkg,
		IncludeSchemas:  includeSchemas,
		ProjectRepo:     projectRepo,
		CodegenPath:     goGenPath,
		GroupByKind:     !groupKinds,
		DestinationPath: manifestGoFilePath,
	},
		&jennies.CustomRouteGoTypesJenny{
			AddKubernetesCodegen: true,
			GroupByKind:          !groupKinds,
			AnyAsInterface:       true, // This is for compatibility with kube openAPI generator, which has issues with map[string]any
		},
		&jennies.ResourceClientJenny{
			GroupByKind: !groupKinds,
		})
	return g
}

func namerFunc(k codegen.Kind) string {
	if k == nil {
		return "nil"
	}
	return k.Properties().Kind
}

func namerFuncManifest(m codegen.AppManifest) string {
	if m == nil {
		return "nil"
	}
	return m.Name()
}
