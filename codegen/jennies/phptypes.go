package jennies

import (
    "bytes"
    "context"
    "cuelang.org/go/cue"
    "fmt"
    "github.com/grafana/codejen"
    "github.com/grafana/cog"
    "github.com/grafana/grafana-app-sdk/codegen"
    "github.com/grafana/grafana-app-sdk/codegen/templates"
    "path"
    "strings"
)

type PHPResourceTypes struct {
    GenerateOnlyCurrent bool
}

func (t *PHPResourceTypes) JennyName() string { return "PHPResourceTypes" }

func (t *PHPResourceTypes) Generate(kind codegen.Kind) (codejen.Files, error) {
    files := make(codejen.Files, 0)
    if t.GenerateOnlyCurrent {
        ver := kind.Version(kind.Properties().Current)
        if ver == nil {
            return nil, fmt.Errorf("no version for %s", kind.Properties().Current)
        }
        if !ver.Codegen.PHP.Enabled {
            return nil, nil
        }
        b, err := t.generateObjectFile(kind, ver, strings.ToLower(kind.Properties().MachineName)+"_")
        if err != nil {
            return nil, err
        }
        files = append(files, codejen.File{
            RelativePath: fmt.Sprintf("src/%s/%s.php", kind.Properties().MachineName, formatJavaFilename(kind.Properties().MachineName)),
            Data:         b,
            From:         []codejen.NamedJenny{t},
        })
    } else {
        allVersions := kind.Versions()
        for i := 0; i < len(allVersions); i++ {
            ver := allVersions[i]
            if !ver.Codegen.PHP.Enabled {
                continue
            }
            b, err := t.generateObjectFile(kind, &ver, "")
            if err != nil {
                return nil, err
            }
            files = append(files, codejen.File{
                RelativePath: fmt.Sprintf("%s/%s/%s.php", kind.Properties().MachineName, ver.Version, formatJavaFilename(kind.Properties().MachineName)),
                Data:         b,
                From:         []codejen.NamedJenny{t},
            })
        }
    }
    return files, nil
}

// TODO: Support metadata for PHP
func (t *PHPResourceTypes) generateObjectFile(kind codegen.Kind, version *codegen.KindVersion, phpTypePrefix string) ([]byte, error) {
    metadata := templates.ResourceTSTemplateMetadata{
        TypeName:     exportField(kind.Name()),
        Subresources: make([]templates.SubresourceMetadata, 0),
        FilePrefix:   phpTypePrefix,
    }

    it, err := version.Schema.Fields()
    if err != nil {
        return nil, err
    }
    for it.Next() {
        if it.Selector().String() == "spec" || it.Selector().String() == "metadata" {
            continue
        }
        metadata.Subresources = append(metadata.Subresources, templates.SubresourceMetadata{
            TypeName: exportField(it.Selector().String()),
            JSONName: it.Selector().String(),
        })
    }

    phpBytes := &bytes.Buffer{}
    err = templates.WriteResourceTSType(metadata, phpBytes)
    if err != nil {
        return nil, err
    }
    return phpBytes.Bytes(), nil
}

// PHPTypes is a one-to-many jenny that generates one or more PHPTypes types for a kind.
// Each type is a specific version of the kind where codegen.frontend is true.
// If GenerateOnlyCurrent is true, then all other versions of the kind will be ignored and only
// the kind.Properties().Current version will be used for PHPTypes type generation
// (this will impact the generated file path).
type PHPTypes struct {
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

    GenBuilders bool
}

func (j *PHPTypes) JennyName() string { return "PHPTypes" }

func (j *PHPTypes) Generate(kind codegen.Kind) (codejen.Files, error) {
    if j.GenerateOnlyCurrent {
        ver := kind.Version(kind.Properties().Current)
        if ver == nil {
            return nil, fmt.Errorf("version '%s' of kind '%s' does not exist", kind.Properties().Current, kind.Name())
        }
        if !ver.Codegen.PHP.Enabled {
            return nil, nil
        }

        return j.generateFiles(ver, kind.Name())
    }

    files := make(codejen.Files, 0)
    // For each version, check if we need to codegen
    allVersions := kind.Versions()
    for i := 0; i < len(allVersions); i++ {
        v := allVersions[i]
        if !v.Codegen.PHP.Enabled {
            continue
        }

        generated, err := j.generateFiles(&v, kind.Name())
        if err != nil {
            return nil, err
        }
        files = append(files, generated...)
    }
    return files, nil
}

func (j *PHPTypes) generateFiles(version *codegen.KindVersion, name string) (codejen.Files, error) {
    if j.Depth > 0 {
        return j.generateFilesAtDepth(version.Schema, version, 0)
    }

    phpBytes, err := generatePHPBytes(version.Schema, ToPackageName(version.Version), exportField(sanitizeLabelString(name)), j.GenBuilders, cog.PHPConfig{})
    if err != nil {
        return nil, err
    }
    return codejen.Files{codejen.File{
        Data:         phpBytes,
        RelativePath: fmt.Sprintf("%s.php", formatJavaFilename(name)),
        From:         []codejen.NamedJenny{j},
    }}, nil
}

func (j *PHPTypes) generateFilesAtDepth(v cue.Value, kv *codegen.KindVersion, currDepth int) (codejen.Files, error) {
    if currDepth == j.Depth {
        fieldName := make([]string, 0)
        for _, s := range TrimPathPrefix(v.Path(), kv.Schema.Path()).Selectors() {
            fieldName = append(fieldName, s.String())
        }
        phpBytes, err := generatePHPBytes(v, ToPackageName(kv.Version), exportField(strings.Join(fieldName, "")), j.GenBuilders, cog.PHPConfig{})
        if err != nil {
            return nil, err
        }
        return codejen.Files{codejen.File{
            Data:         phpBytes,
            RelativePath: path.Join(formatJavaFilename(strings.Join(fieldName, "")), ".py"),
            From:         []codejen.NamedJenny{j},
        }}, nil
    }

    it, err := v.Fields()
    if err != nil {
        return nil, err
    }

    files := make(codejen.Files, 0)
    for it.Next() {
        f, err := j.generateFilesAtDepth(it.Value(), kv, currDepth+1)
        if err != nil {
            return nil, err
        }
        files = append(files, f...)
    }
    return files, nil
}

func generatePHPBytes(v cue.Value, packageName string, name string, genBuilders bool, phpConfig cog.PHPConfig) ([]byte, error) {
    codegenPipeline := cog.TypesFromSchema().
        CUEValue(packageName, v, cog.ForceEnvelope(name)).
        PHP(phpConfig)

    if genBuilders {
        codegenPipeline = codegenPipeline.GenerateBuilders()
    }

    files, err := codegenPipeline.Run(context.Background())
    if err != nil {
        return nil, err
    }

    if len(files) != 1 {
        return nil, fmt.Errorf("expected one file to be generated, got %d", len(files))
    }

    return files[0].Data, nil
}
