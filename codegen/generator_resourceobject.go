package codegen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/codejen"

	"github.com/grafana/grafana-app-sdk/codegen/templates"
	"github.com/grafana/grafana-app-sdk/kindsys"
	"github.com/grafana/grafana-app-sdk/resource"
)

// slightly janky dynamic way of getting the JSON keys of all CommonMetadata fields,
// so they don't need to be hard-coded in the Jenny, and can sync with kindsys
var (
	commonMetadataBytes, _ = json.Marshal(resource.CommonMetadata{})
	commonMetadataFieldMap = make(map[string]any)
	_                      = json.Unmarshal(commonMetadataBytes, &commonMetadataFieldMap)
)

type resourceObjectGenerator struct {
}

func (*resourceObjectGenerator) JennyName() string {
	return "ResourceObjectGenerator"
}

func (r *resourceObjectGenerator) Generate(decl kindsys.Custom) (*codejen.File, error) {
	customMetadataFields := make([]templates.ObjectMetadataField, 0)
	mdv := decl.Lineage().Latest().Underlying().LookupPath(cue.MakePath(cue.Str("schema"), cue.Str("metadata")))
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

	meta := decl.Def().Properties
	md := templates.ResourceObjectTemplateMetadata{
		Package:              strings.ToLower(meta.MachineName),
		TypeName:             decl.Lineage().Name(),
		SpecTypeName:         "Spec",
		ObjectTypeName:       "Object", // Package is the machine name of the object, so this makes it machinename.Object
		ObjectShortName:      "o",
		Subresources:         make([]templates.SubresourceMetadata, 0),
		CustomMetadataFields: customMetadataFields,
	}
	for _, sr := range getSubresources(decl.Lineage().Latest()) {
		if sr.FieldName == "metadata" || sr.FieldName == "spec" {
			continue
		}
		md.Subresources = append(md.Subresources, templates.SubresourceMetadata{
			TypeName: sr.TypeName,
			JSONName: sr.FieldName,
			Comment:  sr.Comment,
		})
	}
	b := bytes.Buffer{}
	err := templates.WriteResourceObject(md, &b)
	if err != nil {
		return nil, err
	}
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return nil, err
	}
	return codejen.NewFile(fmt.Sprintf("%s/%s_object_gen.go", meta.MachineName, meta.MachineName), formatted, r), nil
}
