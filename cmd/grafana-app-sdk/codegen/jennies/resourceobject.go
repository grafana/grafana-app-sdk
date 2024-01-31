package jennies

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/resource"
)

// slightly janky dynamic way of getting the JSON keys of all CommonMetadata fields,
// so they don't need to be hard-coded in the Jenny, and can sync with kindsys
var (
	commonMetadataBytes, _ = json.Marshal(resource.CommonMetadata{})
	commonMetadataFieldMap = make(map[string]any)
	_                      = json.Unmarshal(commonMetadataBytes, &commonMetadataFieldMap)
)

type ResourceObjectGenerator struct {
	// This flag exists for compatibility with thema codegen, which only generates code for the current/latest version of the kind
	OnlyUseCurrentVersion bool
}

func (*ResourceObjectGenerator) JennyName() string {
	return "ResourceObjectGenerator"
}

func (r *ResourceObjectGenerator) Generate(kind codegen.Kind) (codejen.Files, error) {
	files := make(codejen.Files, 0)
	if r.OnlyUseCurrentVersion {
		ver := kind.Version(kind.Properties().Current)
		if ver == nil {
			return nil, fmt.Errorf("no version for %s", kind.Properties().Current)
		}
		b, err := r.generateObjectFile(kind, ver, strings.ToLower(kind.Properties().MachineName))
		if err != nil {
			return nil, err
		}
		files = append(files, codejen.File{
			RelativePath: fmt.Sprintf("%s/%s_object_gen.go", kind.Properties().MachineName, kind.Properties().MachineName),
			Data:         b,
			From:         []codejen.NamedJenny{r},
		})
	} else {
		allVersions := kind.Versions()
		for i := 0; i < len(allVersions); i++ {
			ver := allVersions[i]
			b, err := r.generateObjectFile(kind, &ver, ToPackageName(ver.Version))
			if err != nil {
				return nil, err
			}
			files = append(files, codejen.File{
				RelativePath: fmt.Sprintf("%s/%s/%s_object_gen.go", kind.Properties().MachineName, ToPackageName(ver.Version), kind.Properties().MachineName),
				Data:         b,
				From:         []codejen.NamedJenny{r},
			})
		}
	}
	return files, nil
}

func (*ResourceObjectGenerator) generateObjectFile(kind codegen.Kind, version *codegen.KindVersion, pkg string) ([]byte, error) {
	customMetadataFields := make([]templates.ObjectMetadataField, 0)
	mdv := version.Schema.LookupPath(cue.MakePath(cue.Str("metadata")))
	if mdv.Exists() {
		mit, err := mdv.Fields()
		if err != nil {
			return nil, err
		}
		for mit.Next() {
			lbl := mit.Selector().String()
			// Skip the common metadata fields
			if _, ok := commonMetadataFieldMap[lbl]; ok {
				continue
			}
			fieldName := ""
			// The go field name is the CUE label with the first letter capitalized (to make it exported)
			// We have to track the actual field names of all custom metadata fields for the template
			if len(lbl) > 1 {
				fieldName = strings.ToUpper(lbl[0:1]) + lbl[1:]
			} else {
				fieldName = strings.ToUpper(lbl)
			}
			customMetadataFields = append(customMetadataFields, templates.ObjectMetadataField{
				JSONName:  lbl,
				FieldName: fieldName,
			})
		}
	}

	meta := kind.Properties()
	md := templates.ResourceObjectTemplateMetadata{
		Package:              pkg,
		TypeName:             meta.Kind,
		SpecTypeName:         "Spec",
		ObjectTypeName:       "Object", // Package is the machine name of the object, so this makes it machinename.Object
		ObjectShortName:      "o",
		Subresources:         make([]templates.SubresourceMetadata, 0),
		CustomMetadataFields: customMetadataFields,
	}
	it, err := version.Schema.Fields()
	if err != nil {
		return nil, err
	}
	for it.Next() {
		if it.Label() == "spec" || it.Label() == "metadata" {
			continue
		}
		md.Subresources = append(md.Subresources, templates.SubresourceMetadata{
			TypeName: exportField(it.Label()),
			JSONName: it.Label(),
		})
	}
	b := bytes.Buffer{}
	err = templates.WriteResourceObject(md, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return formatted, nil
}
