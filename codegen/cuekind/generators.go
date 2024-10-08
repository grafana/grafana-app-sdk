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
// If `versioned` is true, the paths to the generated files will include the version, and
// the package name will be the version, rather than the kind.
// If `groupKinds` is true, kinds within the same group will exist in the same package.
// When combined with `versioned`, each version package will contain all kinds in the group
// which have a schema for that version.
func ResourceGenerator(versioned bool, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(
		&jennies.GoTypes{
			GenerateOnlyCurrent:  !versioned,
			Depth:                1,
			AddKubernetesCodegen: true,
			GroupByKind:          !groupKinds,
		},
		&jennies.ResourceObjectGenerator{
			OnlyUseCurrentVersion:       !versioned,
			SubresourceTypesArePrefixed: groupKinds,
			GroupByKind:                 !groupKinds,
		},
		&jennies.SchemaGenerator{
			OnlyUseCurrentVersion: !versioned,
			GroupByKind:           !groupKinds,
		},
		&jennies.CodecGenerator{
			OnlyUseCurrentVersion: !versioned,
			GroupByKind:           !groupKinds,
		},
	)
	return g
}

// ModelsGenerator returns a Generator which will produce Go and CUE files for API contract models.
// The `versioned` parameter governs whether to generate all versions where codegen.backend == true,
// or just generate code for the current version.
// If `versioned` is true, the paths to the generated files will include the version, and
// the package name will be the version, rather than the kind.
func ModelsGenerator(versioned bool, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(
		&jennies.GoTypes{
			GenerateOnlyCurrent: !versioned,
			GroupByKind:         !groupKinds,
		},
	)
	return g
}

// BackendPluginGenerator returns a Generator which will produce boilerplate backend plugin code
func BackendPluginGenerator(projectRepo, generatedAPIPath string, versioned bool, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	pluginSecurePkgFiles, _ := templates.GetBackendPluginSecurePackageFiles()

	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(
		jennies.RouterHandlerCodeGenerator(projectRepo, generatedAPIPath, versioned, !groupKinds),
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
		jennies.BackendPluginMainGenerator(projectRepo, generatedAPIPath, versioned, !groupKinds),
	)
	return g
}

// TypeScriptModelsGenerator returns a Generator which generates TypeScript model code.
// The `versioned` parameter governs whether to generate all versions where codegen.frontend == true,
// or just generate code for the current version.
// If `versioned` is true, the paths to the generated files will include the version.
func TypeScriptModelsGenerator(versioned bool) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(&jennies.TypeScriptTypes{
		GenerateOnlyCurrent: !versioned,
	})
	return g
}

// TypeScriptResourceGenerator returns a Generator which generates TypeScript resource code.
// The `versioned` parameter governs whether to generate all versions where codegen.frontend == true,
// or just generate code for the current version.
// If `versioned` is true, the paths to the generated files will include the version.
func TypeScriptResourceGenerator(versioned bool) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(&jennies.TypeScriptTypes{
		GenerateOnlyCurrent: !versioned,
		Depth:               1,
	}, &jennies.TypeScriptResourceTypes{
		GenerateOnlyCurrent: !versioned,
	})
	return g
}

// OperatorGenerator returns a Generator which will build out watcher boilerplate for each resource,
// and a main func to run an operator for the watchers.
func OperatorGenerator(projectRepo, codegenPath string, versioned bool, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer[codegen.Kind](namerFunc)
	g.Append(
		&jennies.OperatorKubeConfigJenny{},
		jennies.OperatorMainJenny(projectRepo, codegenPath, versioned, !groupKinds),
		&jennies.OperatorConfigJenny{},
	)
	return g
}

func AppGenerator(projectRepo, codegenPath string, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	parts := strings.Split(projectRepo, "/")
	if len(parts) == 0 {
		parts = []string{""}
	}
	g := codejen.JennyListWithNamer[codegen.Kind](namerFunc)
	g.Append(
		jennies.WatcherJenny(projectRepo, codegenPath, true, !groupKinds),
		&jennies.AppGenerator{
			GroupByKind: !groupKinds,
			ProjectRepo: projectRepo,
			ProjectName: parts[len(parts)-1],
			CodegenPath: codegenPath,
		},
	)
	return g
}

func PostResourceGenerationGenerator(projectRepo, goGenPath string, versioned bool, groupKinds bool) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer[codegen.Kind](namerFunc)
	g.Append(&jennies.OpenAPI{
		GenerateOnlyCurrent: !versioned,
		GoModName:           projectRepo,
		GoGenPath:           goGenPath,
		GroupByKind:         !groupKinds,
	})
	return g
}

func ManifestGenerator(encoder jennies.ManifestOutputEncoder, extension string, appName string) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer[codegen.Kind](namerFunc)
	g.Append(&jennies.ManifestGenerator{
		AppName:       appName,
		Encoder:       encoder,
		FileExtension: extension,
	})
	return g
}

func ManifestGoGenerator(pkg string, appName string) *codejen.JennyList[codegen.Kind] {
	g := codejen.JennyListWithNamer[codegen.Kind](namerFunc)
	g.Append(&jennies.ManifestGoGenerator{
		Package: pkg,
		AppName: appName,
	})
	return g
}

func namerFunc(k codegen.Kind) string {
	if k == nil {
		return "nil"
	}
	return k.Properties().Kind
}
