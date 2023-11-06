package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/grafana/codejen"
	"golang.org/x/tools/imports"

	"github.com/grafana/grafana-app-sdk/codegen"
	deepmapcodegen "github.com/grafana/grafana-app-sdk/internal/deepmap/oapi-codegen/pkg/codegen"
)

const GoTypesMaxDepth = 5

// ToPackageName sanitizes an input into a deterministic allowed go package name.
// It is used to turn kind names or versions into package names when performing go code generation.
func ToPackageName(input string) string {
	return regexp.MustCompile(`([^A-Za-z0-9_])`).ReplaceAllString(input, "_")
}

// GoTypes is a Jenny for turning a codegen.Kind into go types according to its codegen settings.
type GoTypes struct {
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

func (*GoTypes) JennyName() string {
	return "GoTypes"
}

func (g *GoTypes) Generate(kind codegen.Kind) (codejen.Files, error) {
	if g.GenerateOnlyCurrent {
		ver := kind.Version(kind.Properties().Current)
		if ver == nil {
			return nil, fmt.Errorf("version '%s' of kind '%s' does not exist", kind.Properties().Current, kind.Name())
		}
		return g.generateFiles(ver, kind.Properties().MachineName, kind.Properties().MachineName, kind.Properties().MachineName)
	}

	files := make(codejen.Files, 0)
	versions := kind.Versions()
	for i := 0; i < len(versions); i++ {
		ver := versions[i]
		if !ver.Codegen.Backend {
			continue
		}

		generated, err := g.generateFiles(&ver, kind.Properties().MachineName, ToPackageName(ver.Version), filepath.Join(kind.Properties().MachineName, ToPackageName(ver.Version)))
		if err != nil {
			return nil, err
		}
		files = append(files, generated...)
	}

	return files, nil
}

func (g *GoTypes) generateFiles(version *codegen.KindVersion, machineName string, packageName string, pathPrefix string) (codejen.Files, error) {
	if g.Depth > 0 {
		return g.generateFilesAtDepth(version.Schema, version, 0, machineName, packageName, pathPrefix)
	}

	goBytes, err := GoTypesFromCUE(version.Schema, CUEGoConfig{
		PackageName: packageName,
		Name:        exportField(machineName),
		Version:     version.Version,
	}, 0)
	if err != nil {
		return nil, err
	}
	return codejen.Files{codejen.File{
		Data:         goBytes,
		RelativePath: fmt.Sprintf(path.Join(pathPrefix, "%s_gen.go"), strings.ToLower(machineName)),
		From:         []codejen.NamedJenny{g},
	}}, nil
}

func (g *GoTypes) generateFilesAtDepth(v cue.Value, kv *codegen.KindVersion, currDepth int, machineName string, packageName string, pathPrefix string) (codejen.Files, error) {
	if currDepth == g.Depth {
		fieldName := make([]string, 0)
		for _, s := range TrimPathPrefix(v.Path(), kv.Schema.Path()).Selectors() {
			fieldName = append(fieldName, s.String())
		}
		goBytes, err := GoTypesFromCUE(v, CUEGoConfig{
			PackageName: packageName,
			Name:        exportField(strings.Join(fieldName, "")),
			Version:     kv.Version,
		}, len(v.Path().Selectors())-(g.Depth-g.NamingDepth))
		if err != nil {
			return nil, err
		}
		return codejen.Files{codejen.File{
			Data:         goBytes,
			RelativePath: fmt.Sprintf(path.Join(pathPrefix, "%s_%s_gen.go"), strings.ToLower(machineName), strings.Join(fieldName, "_")),
			From:         []codejen.NamedJenny{g},
		}}, nil
	}

	it, err := v.Fields()
	if err != nil {
		return nil, err
	}

	files := make(codejen.Files, 0)
	for it.Next() {
		f, err := g.generateFilesAtDepth(it.Value(), kv, currDepth+1, machineName, packageName, pathPrefix)
		if err != nil {
			return nil, err
		}
		files = append(files, f...)
	}
	return files, nil
}

type CUEGoConfig struct {
	PackageName             string
	Name                    string
	Version                 string
	IgnoreDiscoveredImports bool

	// ApplyFuncs is a slice of AST manipulation funcs that will be executed against
	// the generated Go file prior to running it through goimports. For each slice
	// element, [dstutil.Apply] is called with the element as the "pre" parameter.
	ApplyFuncs []dstutil.ApplyFunc

	// UseGoDeclInComments sets the name of the fields and structs at the beginning of each comment.
	UseGoDeclInComments bool
}

func GoTypesFromCUE(v cue.Value, cfg CUEGoConfig, maxNamingDepth int) ([]byte, error) {
	openAPIConfig := CUEOpenAPIConfig{
		Name:    cfg.Name,
		Version: cfg.Version,
		NameFunc: func(value cue.Value, path cue.Path) string {
			i := 0
			for ; i < len(path.Selectors()) && i < len(v.Path().Selectors()); i++ {
				if maxNamingDepth > 0 && i >= maxNamingDepth {
					break
				}
				if !SelEq(path.Selectors()[i], v.Path().Selectors()[i]) {
					break
				}
			}
			if i > 0 {
				path = cue.MakePath(path.Selectors()[i:]...)
			}
			return strings.Trim(path.String(), "?#")
		},
	}

	yml, err := CUEValueToOAPIYAML(v, openAPIConfig)
	if err != nil {
		return nil, err
	}

	loader := openapi3.NewLoader()
	oT, err := loader.LoadFromData(yml)
	if err != nil {
		return nil, fmt.Errorf("loading generated openapi failed: %w", err)
	}

	ccfg := deepmapcodegen.Configuration{
		PackageName: cfg.PackageName,
		Compatibility: deepmapcodegen.CompatibilityOptions{
			AlwaysPrefixEnumValues: true,
		},
		Generate: deepmapcodegen.GenerateOptions{
			Models: true,
		},
		OutputOptions: deepmapcodegen.OutputOptions{
			SkipPrune: true,
			// SkipFmt:   true, // we should be able to skip fmt, but dst's parser panics on nested structs when we don't
			UserTemplates: map[string]string{
				"imports.tmpl": importstmpl,
			},
		},
		ImportMapping:     nil,
		AdditionalImports: nil,
	}

	gostr, err := deepmapcodegen.Generate(oT, ccfg)
	if err != nil {
		return nil, fmt.Errorf("openapi generation failed: %w", err)
	}

	applyFuncs := []dstutil.ApplyFunc{depointerizer(), fixRawData(), fixUnderscoreInTypeName()}
	if !cfg.UseGoDeclInComments {
		applyFuncs = append(applyFuncs, fixTODOComments())
	}
	applyFuncs = append(applyFuncs, cfg.ApplyFuncs...)

	return postprocessGoFile(genGoFile{
		path:     fmt.Sprintf("%s_type_gen.go", cfg.Name),
		appliers: applyFuncs,
		in:       []byte(gostr),
		errifadd: !cfg.IgnoreDiscoveredImports,
	})
}

type genGoFile struct {
	errifadd bool
	path     string
	appliers []dstutil.ApplyFunc
	in       []byte
}

func postprocessGoFile(cfg genGoFile) ([]byte, error) {
	fname := sanitizeLabelString(filepath.Base(cfg.path))
	buf := new(bytes.Buffer)
	fset := token.NewFileSet()
	gf, err := decorator.ParseFile(fset, fname, string(cfg.in), parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("error parsing generated file: %w", err)
	}

	for _, af := range cfg.appliers {
		dstutil.Apply(gf, af, nil)
	}

	err = decorator.Fprint(buf, gf)
	if err != nil {
		return nil, fmt.Errorf("error formatting generated file: %w", err)
	}

	byt, err := imports.Process(fname, buf.Bytes(), nil)
	if err != nil {
		return nil, fmt.Errorf("goimports processing of generated file failed: %w", err)
	}

	if cfg.errifadd {
		// Compare imports before and after; warn about performance if some were added
		gfa, _ := parser.ParseFile(fset, fname, string(byt), parser.ParseComments)
		imap := make(map[string]bool)
		for _, im := range gf.Imports {
			imap[im.Path.Value] = true
		}
		var added []string
		for _, im := range gfa.Imports {
			if !imap[im.Path.Value] {
				added = append(added, im.Path.Value)
			}
		}

		if len(added) != 0 {
			// TODO improve the guidance in this error if/when we better abstract over imports to generate
			return nil, fmt.Errorf("goimports added the following import statements to %s: \n\t%s\nRelying on goimports to find imports significantly slows down code generation. Either add these imports with an AST manipulation in cfg.ApplyFuncs, or set cfg.IgnoreDiscoveredImports to true", cfg.path, strings.Join(added, "\n\t"))
		}
	}

	return format.Source(byt)
}

// SanitizeLabelString strips characters from a string that are not allowed for
// use in a CUE label.
func sanitizeLabelString(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			fallthrough
		case r >= 'A' && r <= 'Z':
			fallthrough
		case r >= '0' && r <= '9':
			fallthrough
		case r == '_':
			return r
		default:
			return -1
		}
	}, s)
}

// exportField makes a field name exported
func exportField(field string) string {
	if len(field) > 0 {
		return strings.ToUpper(field[:1]) + field[1:]
	}
	return strings.ToUpper(field)
}

// Almost all of the below imports are eliminated by dst transformers and calls
// to goimports - but if they're not present in the template, then the internal
// call to goimports that oapi-codegen makes will trigger a search for them,
// which can slow down codegen by orders of magnitude.
var importstmpl = `package {{ .PackageName }}

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/deepmap/oapi-codegen/pkg/runtime"
	openapi_types "github.com/deepmap/oapi-codegen/pkg/types"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v4"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/mux"
)
`
