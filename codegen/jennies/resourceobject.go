package jennies

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/grafana/codejen"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/codegen/templates"
)

// slightly janky dynamic way of getting the JSON keys of all CommonMetadata fields,
// so they don't need to be hard-coded in the Jenny, and can sync with kindsys
var (
	tmnow                  = metav1.NewTime(time.Now())
	i64                    = int64(1)
	commonMetadataBytes, _ = json.Marshal(metav1.ObjectMeta{
		Name:                       "foo",
		GenerateName:               "bar",
		Namespace:                  "foo",
		SelfLink:                   "bar",
		UID:                        types.UID("foo"),
		ResourceVersion:            "bar",
		Generation:                 i64,
		CreationTimestamp:          tmnow,
		DeletionTimestamp:          &tmnow,
		DeletionGracePeriodSeconds: &i64,
		Labels:                     map[string]string{"foo": "bar"},
		Annotations:                map[string]string{"foo": "bar"},
		OwnerReferences:            []metav1.OwnerReference{{}},
		Finalizers:                 []string{"foo"},
		ManagedFields:              []metav1.ManagedFieldsEntry{{}},
	})
	commonMetadataFieldMap = map[string]any{
		"extraFields": struct{}{}, // This needs to be ignored but is currently joined in in kindsys
	}
	_ = json.Unmarshal(commonMetadataBytes, &commonMetadataFieldMap)
)

type ResourceObjectGenerator struct {
	// This flag exists for compatibility with thema codegen, which only generates code for the current/latest version of the kind
	OnlyUseCurrentVersion bool

	// SubresourceTypesArePrefixed should be set to true if the subresource go types (such as spec or status)
	// are prefixed with the exported Kind name. Generally, if you generated go types with Depth: 1 and PrefixWithKindName: true,
	// then you should set this value to true as well.
	SubresourceTypesArePrefixed bool

	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	// When GroupByKind is false, subresource types (such as spec and status) are assumed to be prefixed with the
	// kind name, which can be accomplished by setting GroupByKind=false on the GoTypesGenerator.
	GroupByKind bool

	// GenericCopy toggles whether the generated code for Copy() calls the generic resource.CopyObject method,
	// or generates code to deep-copy the entire struct.
	GenericCopy bool
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
				RelativePath: filepath.Join(GetGeneratedPath(r.GroupByKind, kind, ver.Version), fmt.Sprintf("%s_object_gen.go", strings.ToLower(kind.Properties().MachineName))),
				Data:         b,
				From:         []codejen.NamedJenny{r},
			})
		}
	}
	return files, nil
}

func (r *ResourceObjectGenerator) generateObjectFile(kind codegen.Kind, version *codegen.KindVersion, pkg string) ([]byte, error) {
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
				GoType:    goTypeFromCUEValue(mit.Value()),
			})
		}
	}
	// Sort extra fields so that codegen is deterministic for ordering
	slices.SortFunc(customMetadataFields, func(a, b templates.ObjectMetadataField) int {
		return strings.Compare(a.FieldName, b.FieldName)
	})

	typePrefix := ""
	if r.SubresourceTypesArePrefixed {
		typePrefix = exportField(kind.Name())
	}
	meta := kind.Properties()
	md := templates.ResourceObjectTemplateMetadata{
		Package:              pkg,
		TypeName:             meta.Kind,
		SpecTypeName:         typePrefix + "Spec",
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
			TypeName: typePrefix + exportField(it.Label()),
			JSONName: it.Label(),
		})
	}
	if !r.GenericCopy {
		// Deep copy code
		buf := strings.Builder{}
		buf.WriteString(fmt.Sprintf("cpy := &%s{}\n\n// Copy metadata\no.ObjectMeta.DeepCopyInto(&cpy.ObjectMeta)\n\n// Copy Spec\n", md.TypeName))
		specCopy, err := generateCopyCodeFor(version, "spec", typePrefix)
		if err != nil {
			return nil, err
		}
		buf.WriteString(specCopy + "\n")
		for _, sr := range md.Subresources {
			srCopy, err := generateCopyCodeFor(version, sr.JSONName, typePrefix+"Status")
			if err != nil {
				return nil, err
			}
			buf.WriteString(fmt.Sprintf("\n\n// Copy %s\n%s\n", sr.TypeName, srCopy))
		}
		buf.WriteString("return cpy")
		md.CopyCode = buf.String()
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

// nolint:gocritic
func goTypeFromCUEValue(value cue.Value) templates.CustomMetadataFieldGoType {
	// Super janky for now--there's _got_ to be a better way of determining the type for a definition field
	// Maybe we take it from openapi instead?
	st := cueFmtState{}
	value.Format(&st, 'v')
	typeString := st.String()
	if strings.Contains(typeString, "time.Time") {
		return templates.GoTypeTime
	} else if strings.Contains(typeString, "int") {
		return templates.GoTypeInt
	} else if strings.Contains(typeString, "struct") {
		// TODO--once we allow non-string types, we need to be able to reference them here
		return templates.GoTypeString
	} else if strings.Contains(typeString, "string") {
		return templates.GoTypeString
	}
	return templates.CustomMetadataFieldGoType{}
}

// functions for copy codegen
func generateCopyCodeFor(version *codegen.KindVersion, subresource, namePrefix string) (string, error) {
	v := version.Schema.LookupPath(cue.MakePath(cue.Str(subresource)))
	openAPIConfig := CUEOpenAPIConfig{
		Name:    subresource,
		Version: version.Version,
		NameFunc: func(value cue.Value, path cue.Path) string {
			i := 0
			for ; i < len(path.Selectors()) && i < len(v.Path().Selectors()); i++ {
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
		return "", err
	}

	loader := openapi3.NewLoader()
	oT, err := loader.LoadFromData(yml)
	if err != nil {
		return "", err
	}

	return generateSchemaCopyCode(oT.Components, oT.Components.Schemas[subresource].Value, fmt.Sprintf("o.%s", exportField(subresource)), fmt.Sprintf("cpy.%s", exportField(subresource)), namePrefix)
}

func generateSchemaCopyCode(root *openapi3.Components, sch *openapi3.Schema, srcName, dstName, namingPrefix string) (string, error) {
	buf := strings.Builder{}
	for k, v := range sch.Properties {
		isPointer := !slices.Contains(sch.Required, k)
		str, err := generateSchemaRefCopyCode(root, k, v, isPointer, srcName, dstName, namingPrefix)
		if err != nil {
			return "", err
		}
		buf.WriteString(str)
	}
	return buf.String(), nil
}

func generateSchemaRefCopyCode(root *openapi3.Components, field string, schemaRef *openapi3.SchemaRef, isPointer bool, srcName, dstName, namingPrefix string) (string, error) {
	fmt.Println(srcName + "." + field)
	ek := exportField(field)
	buf := strings.Builder{}
	if schemaRef.Ref != "" {
		// $ref to another object schema
		ref, err := lookupRef(root, schemaRef.Ref)
		if err != nil {
			return "", err
		}
		refTypeName := namingPrefix + strings.Join(strings.Split(schemaRef.Ref, "/")[3:], "")
		if schemaRef.Value.Type.Is("object") {
			if isPointer {
				buf.WriteString(fmt.Sprintf("if %s.%s != nil {\n%s.%s = &%s{}\n", srcName, ek, dstName, ek, refTypeName))
			}
			str, err := generateSchemaCopyCode(root, ref, fmt.Sprintf("%s.%s", srcName, ek), fmt.Sprintf("%s.%s", dstName, ek), namingPrefix)
			if err != nil {
				return "", err
			}
			buf.WriteString(str)
			if isPointer {
				buf.WriteString("}\n")
			}
		}
		return buf.String(), nil
	}

	// Not a ref, examine the schema
	schema := schemaRef.Value

	if schemaRef.Value.Type.Is("object") {
		if schemaRef.Value.AdditionalProperties.Schema == nil {
			if len(schema.Properties) > 0 {
				// Embedded struct
				for key, prop := range schema.Properties {
					str, err := generateSchemaRefCopyCode(root, key, prop, !slices.Contains(schema.Required, key), fmt.Sprintf("%s.%s", srcName, ek), fmt.Sprintf("%s.%s", dstName, ek), namingPrefix)
					if err != nil {
						return "", err
					}
					buf.WriteString(str)
				}
				return buf.String(), nil
			}
			// map[string]any
			// TODO something better
			buf.WriteString(fmt.Sprintf("%s.%s = make(map[string]any)\n", dstName, ek))
			buf.WriteString(fmt.Sprintf("for key, val := range %s.%s {\n", srcName, ek))
			buf.WriteString(fmt.Sprintf("%s.%s[key] = val\n}\n", dstName, ek))
			return buf.String(), nil
		}
		buf.WriteString(fmt.Sprintf("%s.%s = make(map[string]%s)\n", dstName, ek, oapiTypeToGoType(schema.AdditionalProperties.Schema, namingPrefix)))
		buf.WriteString(fmt.Sprintf("for key, val := range %s.%s {\n", srcName, ek))
		buf.WriteString(fmt.Sprintf("cpyVal := %s{}\n", oapiTypeToGoType(schema.AdditionalProperties.Schema, namingPrefix)))
		ref, err := lookupRef(root, schema.AdditionalProperties.Schema.Ref)
		if err != nil {
			return "", err
		}
		copyStr, err := copyProps(root, ref, "val", "cpyVal", namingPrefix)
		if err != nil {
			return "", err
		}
		buf.WriteString(fmt.Sprintf("%s\n%s.%s[key] = cpyVal\n}\n", copyStr, dstName, ek))
		return buf.String(), nil
	}
	if schema.Type.Is("array") {
		buf.WriteString(fmt.Sprintf("if %s.%s != nil {\n", srcName, ek))
		buf.WriteString(fmt.Sprintf("%s.%s = make([]%s, len(%s.%s))\n", dstName, ek, oapiTypeToGoType(schema.Items, namingPrefix), srcName, ek))
		buf.WriteString(fmt.Sprintf("copy(%s.%s, %s.%s)\n", dstName, ek, srcName, ek))
		buf.WriteString("}\n")
		return buf.String(), nil
	}
	if isPointer {
		buf.WriteString(fmt.Sprintf("if %s.%s != nil {\n", srcName, ek))
		buf.WriteString(fmt.Sprintf("%sCopy := *%s.%s\n", field, srcName, ek))
		buf.WriteString(fmt.Sprintf("%s.%s = &%sCopy\n}\n", dstName, ek, field))
		return buf.String(), nil
	}
	buf.WriteString(fmt.Sprintf("%s.%s = %s.%s\n", dstName, ek, srcName, ek))
	return buf.String(), nil
}

func oapiTypeToGoType(v *openapi3.SchemaRef, refNamePrefix string) string {
	if v.Value.Type.Is("integer") {
		switch v.Value.Format {
		case "int32", "int64":
			return v.Value.Format
		}
		return "int"
	}
	if v.Value.Type.Is("boolean") {
		return "bool"
	}
	if v.Value.Type.Is("object") {
		if v.Ref != "" {
			return refNamePrefix + strings.Join(strings.Split(v.Ref, "/")[3:], "")
		}
		// TODO: inline structs
		return "any"
	}
	if v.Value.Type.Is("array") {
		return "[]" + oapiTypeToGoType(v.Value.Items, refNamePrefix)
	}
	if v.Value.Type.Is("string") {
		if v.Value.Format == "date-time" {
			return "time.Time"
		}
		return "string"
	}
	if v.Value.Type.Is("number") {
		if v.Value.Format == "double" {
			return "float64"
		}
		if v.Value.Format == "float" {
			return "float32"
		}
	}
	return "any"
}

func lookupRef(root *openapi3.Components, ref string) (*openapi3.Schema, error) {
	parts := strings.Split(ref, "/")
	if len(parts) < 3 || strings.Join(parts[:3], "/") != "#/components/schemas" {
		return nil, fmt.Errorf("only references to #/components/schemas are supported")
	}
	for k, v := range root.Schemas {
		if k == parts[3] {
			if len(parts) > 4 {
				return lookupRefInSchema(v.Value.Properties, strings.Join(parts[3:], "/"))
			}
			return v.Value, nil
		}
	}
	return nil, fmt.Errorf("reference %s not found", ref)
}

func lookupRefInSchema(sch openapi3.Schemas, ref string) (*openapi3.Schema, error) {
	parts := strings.Split(ref, "/")
	for k, v := range sch {
		if k == parts[0] {
			if len(parts) > 1 {
				return lookupRefInSchema(v.Value.Properties, strings.Join(parts[1:], "/"))
			}
			return v.Value, nil
		}
	}
	return nil, fmt.Errorf("reference %s not found", ref)
}
