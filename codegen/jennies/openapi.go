package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"strings"

	"github.com/grafana/codejen"
	"github.com/grafana/grafana-app-sdk/codegen"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/gengo/v2"
	"k8s.io/gengo/v2/generator"
	"k8s.io/kube-openapi/cmd/openapi-gen/args"
	"k8s.io/kube-openapi/pkg/generators"
)

// OpenAPI jenny uses kube-openapi to build OpenAPI spec.
// Because k8s.io/gengo doesn't allow you to use in-memory files (as the Overlay in the config is not accessible),
// this jenny must be run _after_ the go type has been written to disk.
type OpenAPI struct {
	// GenerateOnlyCurrent should be set to true if you only want to generate code for the kind.Properties().Current version.
	// This will affect the package and path(s) of the generated file(s).
	GenerateOnlyCurrent bool

	GoModName string
	GoGenPath string

	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	// When GroupByKind is false, only one generated OpenAPI file will exist for the entire GroupVersion.
	GroupByKind bool
}

func (*OpenAPI) JennyName() string {
	return "OpenAPI"
}

func (o *OpenAPI) Generate(kinds ...codegen.Kind) (codejen.Files, error) {
	fs := codejen.NewFS()
	if o.GenerateOnlyCurrent {
		/*ver := kind.Version(kind.Properties().Current)
		if ver == nil {
			return nil, fmt.Errorf("version '%s' of kind '%s' does not exist", kind.Properties().Current, kind.Name())
		}
		return g.generateFiles(ver, kind.Name(), kind.Properties().MachineName, kind.Properties().MachineName, kind.Properties().MachineName)*/
	}

	// Group kinds by package name
	if o.GroupByKind {
		for _, k := range kinds {
			versions := k.Versions()
			for i := 0; i < len(versions); i++ {
				ver := versions[i]
				if !ver.Codegen.Backend {
					continue
				}

				err := gengo.Execute(generators.NameSystems(),
					generators.DefaultNameSystem(),
					o.getTargetsFunc(ToPackageName(ver.Version), filepath.Join(o.GoGenPath, GetGeneratedPath(o.GroupByKind, k, ver.Version)), fs),
					gengo.StdBuildTag,
					[]string{fmt.Sprintf("%s/%s/%s", o.GoModName, o.GoGenPath, GetGeneratedPath(o.GroupByKind, k, ver.Version))},
				)
				if err != nil {
					return nil, err
				}

				/*generated, err := g.generateFiles(&ver, kind.Name(), kind.Properties().MachineName, ToPackageName(ver.Version), filepath.Join(kind.Properties().MachineName, ToPackageName(ver.Version)))
				if err != nil {
					return nil, err
				}
				files = append(files, generated...)*/
			}
		}
	} else {
		gvs := make(map[schema.GroupVersion]struct{})
		for _, k := range kinds {
			for _, v := range k.Versions() {
				if !v.Codegen.Backend {
					continue
				}
				gvs[schema.GroupVersion{Group: k.Properties().Group, Version: v.Version}] = struct{}{}
			}
		}
		for gv, _ := range gvs {
			err := gengo.Execute(generators.NameSystems(),
				generators.DefaultNameSystem(),
				o.getTargetsFunc(ToPackageName(gv.Version), filepath.Join(o.GoGenPath, ToPackageName(strings.ToLower(gv.Group)), ToPackageName(gv.Version)), fs),
				gengo.StdBuildTag,
				[]string{filepath.Join(o.GoModName, o.GoGenPath, ToPackageName(gv.Group), ToPackageName(gv.Version))},
			)
			if err != nil {
				return nil, err
			}
		}
	}

	return fs.AsFiles(), nil
}

func (o *OpenAPI) getTargetsFunc(packageName string, packagePath string, fs *codejen.FS) func(context *generator.Context) []generator.Target {
	fmt.Println(packagePath)
	return func(context *generator.Context) []generator.Target {
		context.FileTypes[generator.GoFileType] = &GoFile{
			FS:     fs,
			Source: o,
		}
		arguments := args.New()
		arguments.OutputPkg = packagePath
		arguments.OutputDir = packagePath
		arguments.OutputFile = "zz_openapi_gen.go"
		return generators.GetTargets(context, arguments)
	}
}

/*func (o *OpenAPI) getTargetsFunc(ver *codegen.KindVersion, packageName string, packagePath string) func(context *generator.Context) []generator.Target {
	return func(context *generator.Context) []generator.Target {
		return []generator.Target{
			&generator.SimpleTarget{
				PkgName:       packageName, // `path` vs. `filepath` because packages use '/'
				PkgPath:       packagePath,
				PkgDir:        "",
				HeaderComment: []byte(""),
				GeneratorsFunc: func(c *generator.Context) (generators []generator.Generator) {
					return []generator.Generator{
						newOpenAPIGen(
							args.OutputFile,
							args.OutputPkg,
						),
						newAPIViolationGen(),
					}
				},
				FilterFunc: apiTypeFilterFunc,
			},
		}
	}
}*/

type GoFile struct {
	FS     *codejen.FS
	Source codejen.NamedJenny
}

func (g *GoFile) AssembleFile(f *generator.File, pathname string) error {
	fmt.Println("HELLO")
	buf := &bytes.Buffer{}

	// Writing go file copied from k8s.io/gengo/generator.assembleGolangFile
	// (https://github.com/kubernetes/gengo/blob/master/generator/execute.go#L128)
	buf.Write(f.Header)
	fmt.Fprintf(buf, "package %v\n\n", f.PackageName)

	if len(f.Imports) > 0 {
		fmt.Fprint(buf, "import (\n")
		for i := range f.Imports {
			if strings.Contains(i, "\"") {
				// they included quotes, or are using the
				// `name "path/to/pkg"` format.
				fmt.Fprintf(buf, "\t%s\n", i)
			} else {
				fmt.Fprintf(buf, "\t%q\n", i)
			}
		}
		fmt.Fprint(buf, ")\n\n")
	}

	if f.Vars.Len() > 0 {
		fmt.Fprint(buf, "var (\n")
		buf.Write(f.Vars.Bytes())
		fmt.Fprint(buf, ")\n\n")
	}

	if f.Consts.Len() > 0 {
		fmt.Fprint(buf, "const (\n")
		buf.Write(f.Consts.Bytes())
		fmt.Fprint(buf, ")\n\n")
	}

	buf.Write(f.Body.Bytes())
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}

	path := f.PackagePath
	pt := filepath.SplitList(f.PackagePath)
	if len(pt) > 1 {
		path = filepath.Join(pt[1:]...)
	}
	return g.FS.Add(codejen.File{
		RelativePath: filepath.Join(path, f.Name),
		Data:         formatted,
		From:         []codejen.NamedJenny{g.Source},
	})
}
