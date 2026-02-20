package jennies

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type ConverterGenerator struct {
	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	// When GroupByKind is false, subresource types (such as spec and status) are prefixed with the kind name,
	// i.e. generating FooSpec instead of Spec for kind.Name() = "Foo" and Depth=1
	GroupByKind bool
	ProjectRepo string
	CodegenPath string
}

func (*ConverterGenerator) JennyName() string {
	return "conversion"
}

type conversionKind struct {
	versions   []string
	schemas    map[string]cue.Value
	conversion bool
}

func (c *ConverterGenerator) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	kinds := map[string]conversionKind{}
	internal := appManifest.Properties().PreferredVersion
	if internal == "" {
		internal = appManifest.Versions()[len(appManifest.Versions())-1].Name()
	}
	for _, v := range appManifest.Versions() {
		for _, kind := range v.Kinds() {
			ck, ok := kinds[kind.Kind]
			if !ok {
				ck = conversionKind{
					versions:   make([]string, 0),
					schemas:    make(map[string]cue.Value),
					conversion: kind.Conversion,
				}
			}
			ck.versions = append(ck.versions, v.Name())
			if ck.conversion != kind.Conversion {
				return nil, fmt.Errorf("kind %s has mismatched conversion values between versions", kind.Kind)
			}
			ck.schemas[v.Name()] = kind.Schema
			kinds[kind.Kind] = ck
		}
	}

	files := make(codejen.Files, 0)

	for kind, ck := range kinds {
		// For each version, write conversion funcs file
		for _, version := range ck.versions {
			// Get version OpenAPI
			versionOpenAPI, err := manifests.GetKindVersionOpenAPI(appManifest, kind, version)
			if err != nil {
				return nil, err
			}
			internalOpenAPI, err := manifests.GetKindVersionOpenAPI(appManifest, kind, internal)
			if err != nil {
				return nil, err
			}
			b := bytes.Buffer{}
			err = templates.WriteConversionVersionKindConverter(templates.ConversionVersionKindConverterMetadata{
				VersionPackagePath:           filepath.Join(c.ProjectRepo, c.CodegenPath, GetGeneratedGoTypePath(c.GroupByKind, appManifest.Properties().Group, version, ToPackageName(strings.ToLower(kind)))),
				InternalPackagePath:          filepath.Join(c.ProjectRepo, c.CodegenPath, GetGeneratedGoTypePath(c.GroupByKind, appManifest.Properties().Group, internal, ToPackageName(strings.ToLower(kind)))),
				VersionPackage:               ToPackageName(strings.ToLower(version)),
				InternalPackage:              ToPackageName(strings.ToLower(internal)),
				KindTypeName:                 exportField(kind),
				VersionOpenAPI:               versionOpenAPI.components,
				VersionOpenAPIKindComponent:  versionOpenAPI.kindKey,
				InternalOpenAPI:              internalOpenAPI.components,
				InternalOpenAPIKindComponent: internalOpenAPI.kindKey,
			}, &b)
			if err != nil {
				return nil, err
			}
			formatted, err := format.Source(b.Bytes())
			if err != nil {
				return nil, err
			}
			files = append(files, codejen.File{
				RelativePath: filepath.Join(ToPackageName(appManifest.Properties().Group), "conversion", fmt.Sprintf("%s_%s_converter.gen.go", strings.ToLower(ToPackageName(version)), strings.ToLower(kind))),
				Data:         formatted,
				From:         []codejen.NamedJenny{c},
			})
		}
		// Write converter
		md := templates.ConversionKindConverterMetadata{
			KindTypeName: exportField(kind),
			Versions:     make([]templates.ConversionKindConverterMetadataVersion, 0, len(ck.versions)),
		}
		for _, v := range ck.versions {
			md.Versions = append(md.Versions, templates.ConversionKindConverterMetadataVersion{
				PackageName: ToPackageName(v),
				PackagePath: filepath.Join(c.ProjectRepo, c.CodegenPath, GetGeneratedGoTypePath(c.GroupByKind, appManifest.Properties().Group, v, ToPackageName(strings.ToLower(kind)))),
			})
		}
		b := bytes.Buffer{}
		err := templates.WriteConversionKindConverter(md, &b)
		if err != nil {
			return nil, err
		}
		formatted, err := format.Source(b.Bytes())
		if err != nil {
			return nil, err
		}
		files = append(files, codejen.File{
			RelativePath: filepath.Join(ToPackageName(appManifest.Properties().Group), "conversion", fmt.Sprintf("%s_converter.gen.go", strings.ToLower(kind))),
			Data:         formatted,
			From:         []codejen.NamedJenny{c},
		})
	}

	// Whole-APIGroup Converter and Generic functions/types
	b := bytes.Buffer{}
	err := templates.WriteConversionConverter(templates.ConversionConverterMetadata{}, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	files = append(files, codejen.File{
		RelativePath: filepath.Join(ToPackageName(appManifest.Properties().Group), "conversion", "converter.gen.go"),
		Data:         formatted,
		From:         []codejen.NamedJenny{c},
	})

	return files, nil
}
