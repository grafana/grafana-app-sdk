package thema

import (
	"cuelang.org/go/cue"
	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/jennies"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
	themajennies "github.com/grafana/grafana-app-sdk/codegen/thema/jennies"
	"github.com/grafana/grafana-app-sdk/kindsys"
)

const (
	CustomTargetResource = "resource"
	CustomTargetModel    = "model"
)

func kindsysCustomToKind(decl kindsys.Custom) codegen.Kind {
	props := decl.Def().Properties
	k := &codegen.AnyKind{
		Props: codegen.KindProperties{
			Kind:              decl.Name(),
			MachineName:       props.MachineName,
			PluralName:        props.PluralName,
			PluralMachineName: props.PluralMachineName,
			Group:             props.Group,
			Current:           versionString(props.CurrentVersion),
			Codegen: codegen.KindCodegenProperties{
				Frontend: props.Codegen.Frontend,
				Backend:  props.Codegen.Backend,
			},
		},
		AllVersions: make([]codegen.KindVersion, 0),
	}
	if props.IsCRD {
		k.Props.APIResource = &codegen.APIResourceProperties{
			Group: props.CRD.Group,
			Scope: props.CRD.Scope,
		}
	}
	sch := decl.Lineage().First()
	for sch != nil {
		k.AllVersions = append(k.AllVersions, codegen.KindVersion{
			Version: versionString(sch.Version()),
			Schema:  sch.Underlying().LookupPath(cue.MakePath(cue.Hid("_#schema", "github.com/grafana/thema"))).Eval(),
			Codegen: k.Props.Codegen,
		})
		sch = sch.Successor()
	}
	return k
}

// CRDGenerator returns a Generator which will create a CRD file
func CRDGenerator(outputEncoder jennies.CRDOutputEncoder, outputExtension string) *codejen.JennyList[kindsys.Custom] {
	g := codejen.JennyListWithNamer(kindsysNamerFunc)
	g.Append(codejen.AdaptOneToOne(jennies.CRDGenerator(outputEncoder, outputExtension), kindsysCustomToKind))
	return g
}

func ResourceGenerator() *codejen.JennyList[kindsys.Custom] {
	g := codejen.JennyListWithNamer[kindsys.Custom](kindsysNamerFunc)
	g.Append(
		codejen.AdaptOneToMany[codegen.Kind, kindsys.Custom](&jennies.GoTypes{
			GenerateOnlyCurrent:  true,
			Depth:                1,
			GroupByKind:          true,
			AddKubernetesCodegen: true,
		}, kindsysCustomToKind),
		codejen.AdaptOneToMany[codegen.Kind, kindsys.Custom](&jennies.ResourceObjectGenerator{
			OnlyUseCurrentVersion:       true,
			GroupByKind:                 true,
			SubresourceTypesArePrefixed: false,
		}, kindsysCustomToKind),
		codejen.AdaptOneToMany[codegen.Kind, kindsys.Custom](&jennies.SchemaGenerator{
			OnlyUseCurrentVersion: true,
			GroupByKind:           true,
		}, kindsysCustomToKind),
		codejen.AdaptOneToMany[codegen.Kind, kindsys.Custom](&themajennies.CodecGenerator{
			OnlyUseCurrentVersion: true,
		}, kindsysCustomToKind),
		&themajennies.LineageGenerator{},
		&themajennies.CUEGenerator{},
	)
	return g
}

// ModelsGenerator returns a Generator which will produce Go and CUE files for API contract models
func ModelsGenerator() *codejen.JennyList[kindsys.Custom] {
	g := codejen.JennyListWithNamer[kindsys.Custom](kindsysNamerFunc)
	g.Append(
		codejen.AdaptOneToMany[codegen.Kind, kindsys.Custom](&jennies.GoTypes{
			GenerateOnlyCurrent: true,
		}, kindsysCustomToKind),
		codejen.AdaptOneToOne[codegen.Kind, kindsys.Custom](&themajennies.ModelsFunctionsGenerator{}, kindsysCustomToKind),
		&themajennies.CUEGenerator{},
	)
	return g
}

// BackendPluginGenerator returns a Generator which will produce boilerplate backend plugin code
func BackendPluginGenerator(projectRepo, generatedAPIPath string) *codejen.JennyList[kindsys.Custom] {
	pluginSecurePkgFiles, _ := templates.GetBackendPluginSecurePackageFiles()

	g := codejen.JennyListWithNamer(kindsysNamerFunc)
	g.Append(
		codejen.AdaptOneToOne(jennies.RouterHandlerCodeGenerator(projectRepo, generatedAPIPath, false, true), kindsysCustomToKind),
		jennies.StaticManyToOneGenerator[kindsys.Custom](codejen.File{
			RelativePath: "plugin/secure/data.go",
			Data:         pluginSecurePkgFiles["data.go"],
		}),
		jennies.StaticManyToOneGenerator[kindsys.Custom](codejen.File{
			RelativePath: "plugin/secure/middleware.go",
			Data:         pluginSecurePkgFiles["middleware.go"],
		}),
		jennies.StaticManyToOneGenerator[kindsys.Custom](codejen.File{
			RelativePath: "plugin/secure/retriever.go",
			Data:         pluginSecurePkgFiles["retriever.go"],
		}),
		codejen.AdaptManyToOne(jennies.RouterCodeGenerator(projectRepo), kindsysCustomToKind),
		codejen.AdaptManyToOne(jennies.BackendPluginMainGenerator(projectRepo, generatedAPIPath, false, true), kindsysCustomToKind),
	)
	return g
}

// TypeScriptModelsGenerator returns a Generator which generates TypeScript model code
func TypeScriptModelsGenerator() *codejen.JennyList[kindsys.Custom] {
	g := codejen.JennyListWithNamer(kindsysNamerFunc)
	g.Append(codejen.AdaptOneToMany[codegen.Kind, kindsys.Custom](&jennies.TypeScriptTypes{
		GenerateOnlyCurrent: true,
	}, kindsysCustomToKind))
	return g
}

// OperatorGenerator returns a Generator which will build out watcher boilerplate for each resource,
// and a main func to run an operator for the watchers.
func OperatorGenerator(projectRepo, codegenPath string) *codejen.JennyList[kindsys.Custom] {
	g := codejen.JennyListWithNamer[kindsys.Custom](kindsysNamerFunc)
	g.Append(
		codejen.AdaptOneToOne(jennies.WatcherJenny(projectRepo, codegenPath, false), kindsysCustomToKind),
		codejen.AdaptManyToOne[codegen.Kind, kindsys.Custom](&jennies.OperatorKubeConfigJenny{}, kindsysCustomToKind),
		codejen.AdaptManyToOne(jennies.OperatorMainJenny(projectRepo, codegenPath, false), kindsysCustomToKind),
		codejen.AdaptManyToOne[codegen.Kind, kindsys.Custom](&jennies.OperatorConfigJenny{}, kindsysCustomToKind),
	)
	return g
}
