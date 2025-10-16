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
	// SubresourceTypesArePrefixed should be set to true if the subresource go types (such as spec or status)
	// are prefixed with the exported Kind name. Generally, if you generated go types with Depth: 1 and PrefixWithKindName: true,
	// then you should set this value to true as well.
	SubresourceTypesArePrefixed bool

	// GroupByKind determines whether kinds are grouped by GroupVersionKind or just GroupVersion.
	// If GroupByKind is true, generated paths are <kind>/<version>/<file>, instead of the default <version>/<file>.
	// When GroupByKind is false, subresource types (such as spec and status) are assumed to be prefixed with the
	// kind name, which can be accomplished by setting GroupByKind=false on the GoTypesGenerator.
	GroupByKind bool
}

func (*ResourceObjectGenerator) JennyName() string {
	return "ResourceObjectGenerator"
}

func (r *ResourceObjectGenerator) Generate(kind codegen.Kind) (codejen.Files, error) {
	files := make(codejen.Files, 0)
	allVersions := kind.Versions()
	for i := range len(allVersions) {
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
		if it.Selector().String() == "spec" || it.Selector().String() == "metadata" { //nolint:goconst
			continue
		}
		fieldName := exportField(it.Selector().String())
		md.Subresources = append(md.Subresources, templates.SubresourceMetadata{
			Name:     fieldName,
			TypeName: typePrefix + fieldName,
			JSONName: it.Selector().String(),
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

// nolint:gocritic
func goTypeFromCUEValue(value cue.Value) templates.CustomMetadataFieldGoType {
	// Super janky for now--there's _got_ to be a better way of determining the type for a definition field
	// Maybe we take it from openapi instead?
	st := cueFmtState{}
	value.Format(&st, 'v')
	typeString := st.String()
	if strings.Contains(typeString, "time.Time") { //nolint:revive
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
