package jennies

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/codejen"
	"github.com/grafana/cog"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

type TypeScriptResourceTypes struct {
	GenerateOnlyCurrent bool
}

func (*TypeScriptResourceTypes) JennyName() string { return "TypeScriptResourceTypes" }

func (t *TypeScriptResourceTypes) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0)
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
	files := make(codejen.Files, 0)
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
		fieldName := make([]string, 0)
		for _, s := range TrimPathPrefix(v.Path(), vk.Schema.Path()).Selectors() {
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

	files := make(codejen.Files, 0)
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
