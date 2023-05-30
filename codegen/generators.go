package codegen

import (
	"github.com/grafana/codejen"
	"github.com/grafana/kindsys"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

const (
	CustomTargetResource = "resource"
	CustomTargetModel    = "model"
)

// CRDGenerator returns a Generator which will create a CRD file
func CRDGenerator(outputEncoder CRDOutputEncoder, outputExtension string) Generator {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(&crdGenerator{
		outputExtension: outputExtension,
		outputEncoder:   outputEncoder,
	})
	return g
}

// ResourceGenerator returns a Generator which will produce Go and Cue files for using a schema for storage
func ResourceGenerator() Generator {
	g := codejen.JennyListWithNamer[kindsys.Custom](namerFunc)
	g.Append(&resourceGoTypesGenerator{},
		&resourceObjectGenerator{},
		&schemaGenerator{},
		&lineageGenerator{},
		&cueGenerator{})
	return g
}

// ModelsGenerator returns a Generator which will produce Go and CUE files for API contract models
func ModelsGenerator() Generator {
	g := codejen.JennyListWithNamer[kindsys.Custom](namerFunc)
	g.Append(&modelGoTypesGenerator{},
		&modelsFunctionsGenerator{},
		&cueGenerator{})
	return g
}

// BackendPluginGenerator returns a Generator which will produce boilerplate backend plugin code
func BackendPluginGenerator(projectRepo, generatedAPIPath string) Generator {
	pluginSecurePkgFiles, _ := templates.GetBackendPluginSecurePackageFiles()

	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(&routerHandlerCodeGenerator{
		projectRepo:    projectRepo,
		apiCodegenPath: generatedAPIPath,
	},
		&simpleFileReturnGenerator{
			file: codejen.File{
				RelativePath: "plugin/secure/data.go",
				Data:         pluginSecurePkgFiles["data.go"],
			},
		},
		&simpleFileReturnGenerator{
			file: codejen.File{
				RelativePath: "plugin/secure/middleware.go",
				Data:         pluginSecurePkgFiles["middleware.go"],
			},
		},
		&simpleFileReturnGenerator{
			file: codejen.File{
				RelativePath: "plugin/secure/retriever.go",
				Data:         pluginSecurePkgFiles["retriever.go"],
			},
		},
		&routerCodeGenerator{
			projectRepo: projectRepo,
		},
		&backendPluginMainGenerator{
			projectRepo:    projectRepo,
			apiCodegenPath: generatedAPIPath,
		})
	return g
}

// TypeScriptModelsGenerator returns a Generator which generates TypeScript model code
func TypeScriptModelsGenerator() Generator {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(TSTypesJenny{})
	return g
}

// OperatorGenerator returns a Generator which will build out watcher boilerplate for each resource,
// and a main func to run an operator for the watchers.
func OperatorGenerator(projectRepo, codegenPath string) Generator {
	g := codejen.JennyListWithNamer(namerFunc)
	g.Append(&watcherJenny{
		projectRepo: projectRepo,
		codegenPath: codegenPath,
	}, &operatorKubeConfigJenny{}, &operatorMainJenny{
		projectRepo: projectRepo,
		codegenPath: codegenPath,
	})
	return g
}
