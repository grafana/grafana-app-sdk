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

type PythonResourceTypes struct {
    GenerateOnlyCurrent bool
}

func (t *PythonResourceTypes) JennyName() string { return "PythonResourceTypes" }

func (t *PythonResourceTypes) Generate(kind codegen.Kind) (codejen.Files, error) {
    files := make(codejen.Files, 0)
    if t.GenerateOnlyCurrent {
        ver := kind.Version(kind.Properties().Current)
        if ver == nil {
            return nil, fmt.Errorf("no version for %s", kind.Properties().Current)
        }
        if !ver.Codegen.Python.Enabled {
            return nil, nil
        }
        b, err := t.generateObjectFile(kind, ver, strings.ToLower(kind.Properties().MachineName)+"_")
        if err != nil {
            return nil, err
        }
        files = append(files, codejen.File{
            RelativePath: fmt.Sprintf("models/%s.py", kind.Properties().MachineName),
            Data:         b,
            From:         []codejen.NamedJenny{t},
        })
    } else {
        allVersions := kind.Versions()
        for i := 0; i < len(allVersions); i++ {
            ver := allVersions[i]
            if !ver.Codegen.Python.Enabled {
                continue
            }
            b, err := t.generateObjectFile(kind, &ver, "")
            if err != nil {
                return nil, err
            }
            files = append(files, codejen.File{
                RelativePath: fmt.Sprintf("%s/%s/%s.py", kind.Properties().MachineName, ver.Version, kind.Properties().MachineName),
                Data:         b,
                From:         []codejen.NamedJenny{t},
            })
        }
    }
    return files, nil
}

// TODO: Support metadata for Python
func (t *PythonResourceTypes) generateObjectFile(kind codegen.Kind, version *codegen.KindVersion, pythonTypePrefix string) ([]byte, error) {
    metadata := templates.ResourceTSTemplateMetadata{
        TypeName:     exportField(kind.Name()),
        Subresources: make([]templates.SubresourceMetadata, 0),
        FilePrefix:   pythonTypePrefix,
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

    pythonBytes := &bytes.Buffer{}
    err = templates.WriteResourceTSType(metadata, pythonBytes)
    if err != nil {
        return nil, err
    }
    return pythonBytes.Bytes(), nil
}

// PythonTypes is a one-to-many jenny that generates one or more PythonTypes types for a kind.
// Each type is a specific version of the kind where codegen.frontend is true.
// If GenerateOnlyCurrent is true, then all other versions of the kind will be ignored and only
// the kind.Properties().Current version will be used for PythonTypes type generation
// (this will impact the generated file path).
type PythonTypes struct {
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

func (j *PythonTypes) JennyName() string { return "PythonTypes" }

func (j *PythonTypes) Generate(kind codegen.Kind) (codejen.Files, error) {
    if j.GenerateOnlyCurrent {
        ver := kind.Version(kind.Properties().Current)
        if ver == nil {
            return nil, fmt.Errorf("version '%s' of kind '%s' does not exist", kind.Properties().Current, kind.Name())
        }
        if !ver.Codegen.Python.Enabled {
            return nil, nil
        }

        return j.generateFiles(ver, kind.Name())
    }

    files := make(codejen.Files, 0)
    // For each version, check if we need to codegen
    allVersions := kind.Versions()
    for i := 0; i < len(allVersions); i++ {
        v := allVersions[i]
        if !v.Codegen.Python.Enabled {
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

func (j *PythonTypes) generateFiles(version *codegen.KindVersion, name string) (codejen.Files, error) {
    if j.Depth > 0 {
        return j.generateFilesAtDepth(version.Schema, version, 0)
    }

    pythonBytes, err := generatePythonBytes(version.Schema, ToPackageName(version.Version), exportField(sanitizeLabelString(name)), cog.PythonConfig{})
    if err != nil {
        return nil, err
    }
    return codejen.Files{codejen.File{
        Data:         pythonBytes,
        RelativePath: fmt.Sprintf("%s.py", name),
        From:         []codejen.NamedJenny{j},
    }}, nil
}

func (j *PythonTypes) generateFilesAtDepth(v cue.Value, kv *codegen.KindVersion, currDepth int) (codejen.Files, error) {
    if currDepth == j.Depth {
        fieldName := make([]string, 0)
        for _, s := range TrimPathPrefix(v.Path(), kv.Schema.Path()).Selectors() {
            fieldName = append(fieldName, s.String())
        }
        pythonBytes, err := generatePythonBytes(v, ToPackageName(kv.Version), exportField(strings.Join(fieldName, "")), cog.PythonConfig{})
        if err != nil {
            return nil, err
        }
        return codejen.Files{codejen.File{
            Data:         pythonBytes,
            RelativePath: path.Join(strings.Join(fieldName, ""), ".py"),
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

func generatePythonBytes(v cue.Value, packageName string, name string, pythonConfig cog.PythonConfig) ([]byte, error) {
    files, err := cog.TypesFromSchema().
        CUEValue(packageName, v, cog.ForceEnvelope(name)).
        Python(pythonConfig).
        Run(context.Background())
    if err != nil {
        return nil, err
    }

    if len(files) != 1 {
        return nil, fmt.Errorf("expected one file to be generated, got %d", len(files))
    }

    return files[0].Data, nil
}
