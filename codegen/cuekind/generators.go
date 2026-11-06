package cuekind

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/grafana/codejen"
	"sigs.k8s.io/yaml"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/jennies"
)

// CRDGenerator returns a Generator which will create a CRD file
func CRDGenerator(outputEncoder jennies.CRDOutputEncoder, outputExtension string) *codejen.JennyList[codegen.AppManifest] {
	g := codejen.JennyListWithNamer(namerFuncManifest)
	g.Append(jennies.CRDGenerator(outputEncoder, outputExtension))
	return g
}

// ResourceGenerator returns a collection of jennies which generate backend resource code from kinds.
// The `versioned` parameter governs whether to generate all versions where codegen.backend == true,
// or just generate code for the current version.
// If `groupKinds` is true, kinds within the same group will exist in the same package.
// When combined with `versioned`, each version package will contain all kinds in the group
// which have a schema for that version.
func ResourceGenerator(projectRepo, generatedAPIPath string, groupKinds bool) *codejen.JennyList[codegen.AppManifest] {
	g := codejen.JennyListWithNamer(namerFuncManifest)
	g.Append(
		&jennies.GoTypes{
			Depth:                1,
			AddKubernetesCodegen: true,
			GroupByKind:          !groupKinds,
			AnyAsInterface:       true,                 // This is for compatibility with kube openAPI generator, which has issues with map[string]any
			ExcludeFields:        []string{"metadata"}, // We don't want an object generated for the metadata, as we use the k8s metadata objects
			OpenAPINamer: func(info jennies.OpenAPINamerInfo) string {
				path := filepath.Join(projectRepo, generatedAPIPath, jennies.GetGeneratedGoTypePath(!groupKinds, info.ShortGroup, info.Version, strings.ToLower(info.Kind)), info.TypeName)
				return ToOpenAPIName(path)
			},
		},
		&jennies.ResourceObjectGenerator{
			SubresourceTypesArePrefixed: groupKinds,
			GroupByKind:                 !groupKinds,
			OpenAPINamer: func(info jennies.OpenAPINamerInfo) string {
				path := filepath.Join(projectRepo, generatedAPIPath, jennies.GetGeneratedGoTypePath(!groupKinds, info.ShortGroup, info.Version, strings.ToLower(info.Kind)), info.TypeName)
				return ToOpenAPIName(path)
			},
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

// TypeScriptResourceGenerator returns a Generator which generates TypeScript resource code.
func TypeScriptResourceGenerator() *codejen.JennyList[codegen.AppManifest] {
	g := codejen.JennyListWithNamer(namerFuncManifest)
	g.Append(&jennies.TypeScriptTypes{
		Depth: 1,
	}, &jennies.TypeScriptResourceTypes{})
	return g
}

// OperatorGenerator returns a Generator which will build out watcher boilerplate for each resource,
// and a main func to run an operator for the watchers.
func OperatorGenerator(projectRepo, codegenPath string, groupKinds bool) *codejen.JennyList[codegen.AppManifest] {
	g := codejen.JennyListWithNamer(namerFuncManifest)
	g.Append(
		&jennies.OperatorKubeConfigJenny{},
		jennies.OperatorMainJenny(projectRepo, codegenPath, !groupKinds),
		&jennies.OperatorConfigJenny{},
	)
	return g
}

func AppGenerator(projectRepo, codegenPath string, manifestGoFilePath string, groupKinds bool) *codejen.JennyList[codegen.AppManifest] {
	parts := strings.Split(projectRepo, "/")
	if len(parts) == 0 {
		parts = []string{""}
	}
	g := codejen.JennyListWithNamer(namerFuncManifest)
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

func PostResourceGenerationGenerator(projectRepo, goGenPath string, groupKinds bool) *codejen.JennyList[codegen.AppManifest] {
	g := codejen.JennyListWithNamer[codegen.AppManifest](namerFuncManifest)
	g.Append(&jennies.OpenAPI{
		GoModName:   projectRepo,
		GoGenPath:   goGenPath,
		GroupByKind: !groupKinds,
	})
	return g
}

// ManifestGeneratorConfig configures the ManifestGenerator JennyList.
type ManifestGeneratorConfig struct {
	// Extension selects both the output encoder and, unless FileName is set, the filename suffix.
	Extension string
	// FileName, when non-empty, overrides the generated manifest's filename entirely.
	// The encoder is still selected by Extension.
	FileName       string
	IncludeSchemas bool
	Version        string
}

func ManifestGenerator(cfg ManifestGeneratorConfig) *codejen.JennyList[codegen.AppManifest] {
	generator := &jennies.ManifestGenerator{
		Encoder:         jsonEncoder,
		FileExtension:   cfg.Extension,
		FileName:        cfg.FileName,
		IncludeSchemas:  cfg.IncludeSchemas,
		ManifestVersion: cfg.Version,
	}

	switch cfg.Extension {
	case "json":
		generator.Encoder = jsonEncoder
	case "yaml":
		generator.Encoder = yaml.Marshal
	default:
		// defaults to json
	}

	g := codejen.JennyListWithNamer[codegen.AppManifest](namerFuncManifest)
	g.Append(generator)
	return g
}

// ManifestGoGeneratorConfig configures the ManifestGoGenerator JennyList.
type ManifestGoGeneratorConfig struct {
	Package            string
	IncludeSchemas     bool
	ProjectRepo        string
	GoGenPath          string
	ManifestGoFilePath string
	GroupKinds         bool
	// SkipImportsProcess skips the imports.Process calls in the generated code.
	// This is useful in tests where import resolution is unnecessary and slow.
	SkipImportsProcess bool
}

func ManifestGoGenerator(cfg ManifestGoGeneratorConfig) *codejen.JennyList[codegen.AppManifest] {
	g := codejen.JennyListWithNamer(namerFuncManifest)
	g.Append(&jennies.ManifestGoGenerator{
		Package:            cfg.Package,
		IncludeSchemas:     cfg.IncludeSchemas,
		ProjectRepo:        cfg.ProjectRepo,
		CodegenPath:        cfg.GoGenPath,
		GroupByKind:        !cfg.GroupKinds,
		DestinationPath:    cfg.ManifestGoFilePath,
		SkipImportsProcess: cfg.SkipImportsProcess,
	},
		&jennies.CustomRouteGoTypesJenny{
			AddKubernetesCodegen: true,
			GroupByKind:          !cfg.GroupKinds,
			AnyAsInterface:       true, // This is for compatibility with kube openAPI generator, which has issues with map[string]any
			SkipImportsProcess:   cfg.SkipImportsProcess,
			OpenAPINamer: func(info jennies.OpenAPINamerInfo) string {
				path := filepath.Join(cfg.ProjectRepo, cfg.GoGenPath, jennies.GetGeneratedGoTypePath(!cfg.GroupKinds, info.ShortGroup, info.Version, strings.ToLower(info.Kind)), info.TypeName)
				return ToOpenAPIName(path)
			},
		},
		&jennies.ClientJenny{
			GroupByKind:        !cfg.GroupKinds,
			SkipImportsProcess: cfg.SkipImportsProcess,
			ProjectRepo:        cfg.ProjectRepo,
			GoGenPath:          cfg.GoGenPath,
		})
	return g
}

// ToOpenAPIName is copied from k8s.io/apimachinery/pkg/runtime/scheme.go's toOpenAPIDefinitionName
// To ensure we use the same naming definition as kubernetes expects
// TODO: @IfSentient we should start using OpenAPIModelName() on all go types generated, this will likely need to be done in grafana/cog
func ToOpenAPIName(name string) string {
	nameParts := strings.Split(name, "/")
	// Reverse first part. e.g., io.k8s... instead of k8s.io...
	if len(nameParts) > 0 && strings.Contains(nameParts[0], ".") {
		nameParts[0] = reverseParts(nameParts[0])
	}
	return strings.Join(nameParts, ".")
}

func reverseParts(dotSeparatedName string) string {
	parts := strings.Split(dotSeparatedName, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ".")
}

func namerFuncManifest(m codegen.AppManifest) string {
	if m == nil {
		return "nil"
	}
	return m.Name()
}

var jsonEncoder = func(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "    ")
}
