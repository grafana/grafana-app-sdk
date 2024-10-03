//nolint:goconst
package jennies

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/grafana/codejen"
	goyaml "gopkg.in/yaml.v3"

	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/k8s"
)

// CRDOutputEncoder is a function which marshals an object into a desired output format
type CRDOutputEncoder func(any) ([]byte, error)

func CRDGenerator(encoder CRDOutputEncoder, extension string) codejen.OneToOne[codegen.Kind] {
	return &crdGenerator{
		outputEncoder:   encoder,
		outputExtension: extension,
	}
}

type crdGenerator struct {
	outputEncoder   CRDOutputEncoder
	outputExtension string
}

func (*crdGenerator) JennyName() string {
	return "CRD Generator"
}

func (c *crdGenerator) Generate(kind codegen.Kind) (*codejen.File, error) {
	props := kind.Properties()

	resource := customResourceDefinition{
		APIVersion: "apiextensions.k8s.io/v1",
		Kind:       "CustomResourceDefinition",
		Metadata: customResourceDefinitionMetadata{
			Name: fmt.Sprintf("%s.%s", props.PluralMachineName, props.APIResource.Group),
		},
		Spec: k8s.CustomResourceDefinitionSpec{
			Group: props.APIResource.Group,
			Scope: props.APIResource.Scope,
			Names: k8s.CustomResourceDefinitionSpecNames{
				Kind:   props.Kind,
				Plural: props.PluralMachineName,
			},
			Versions: make([]k8s.CustomResourceDefinitionSpecVersion, 0),
		},
	}

	for _, ver := range kind.Versions() {
		v, err := KindVersionToCRDSpecVersion(ver, kind.Properties().Kind, ver.Version == kind.Properties().Current)
		if err != nil {
			return nil, err
		}
		resource.Spec.Versions = append(resource.Spec.Versions, v)
	}

	contents, err := c.outputEncoder(resource)
	if err != nil {
		return nil, err
	}

	return codejen.NewFile(fmt.Sprintf("%s.%s.%s", kind.Properties().MachineName, kind.Properties().APIResource.Group, c.outputExtension), contents, c), nil
}

func KindVersionToCRDSpecVersion(kv codegen.KindVersion, kindName string, stored bool) (k8s.CustomResourceDefinitionSpecVersion, error) {
	props, err := CUEToCRDOpenAPI(kv.Schema, kindName, kv.Version)
	if err != nil {
		return k8s.CustomResourceDefinitionSpecVersion{}, err
	}

	def := k8s.CustomResourceDefinitionSpecVersion{
		Name:    kv.Version,
		Served:  true,
		Storage: stored,
		Schema: map[string]any{
			"openAPIV3Schema": map[string]any{
				"properties": props,
				"required": []any{
					"spec",
				},
				"type": "object",
			},
		},
		Subresources: make(map[string]any),
	}
	if len(kv.SelectableFields) > 0 {
		sf := make([]k8s.CustomResourceDefinitionSelectableField, len(kv.SelectableFields))
		for i, field := range kv.SelectableFields {
			field = strings.Trim(field, " ")
			if field == "" {
				continue
			}
			if field[0] != '.' {
				field = fmt.Sprintf(".%s", field)
			}
			sf[i] = k8s.CustomResourceDefinitionSelectableField{
				JSONPath: field,
			}
		}
		def.SelectableFields = sf
	}

	if len(kv.AdditionalPrinterColumns) > 0 {
		apc := make([]k8s.CustomResourceDefinitionAdditionalPrinterColumn, len(kv.AdditionalPrinterColumns))
		for i, col := range kv.AdditionalPrinterColumns {
			apc[i] = k8s.CustomResourceDefinitionAdditionalPrinterColumn{
				Name:        col.Name,
				Type:        col.Type,
				Format:      col.Format,
				Description: col.Description,
				Priority:    col.Priority,
				JSONPath:    col.JSONPath,
			}
		}
		def.AdditionalPrinterColumns = apc
	}

	for k := range props {
		if k != "spec" {
			def.Subresources[k] = struct{}{}
		}
	}

	return def, nil
}

// customResourceDefinition differs from k8s.CustomResourceDefinition in that it doesn't use the metav1
// TypeMeta and CommonMeta, as those do not contain YAML tags and get improperly serialized to YAML.
// Since we don't need to use it with the kubernetes go-client, we don't need the extra functionality attached.
//
//nolint:lll
type customResourceDefinition struct {
	Kind       string                           `json:"kind,omitempty" yaml:"kind,omitempty" protobuf:"bytes,1,opt,name=kind"`
	APIVersion string                           `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty" protobuf:"bytes,2,opt,name=apiVersion"`
	Metadata   customResourceDefinitionMetadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       k8s.CustomResourceDefinitionSpec `json:"spec"`
}

type customResourceDefinitionMetadata struct {
	Name string `json:"name,omitempty" yaml:"name" protobuf:"bytes,1,opt,name=name"`
	// TODO: other fields as necessary for codegen
}

type cueOpenAPIEncoded struct {
	Components cueOpenAPIEncodedComponents `json:"components"`
}

type cueOpenAPIEncodedComponents struct {
	Schemas map[string]any `json:"schemas"`
}

func CUEToCRDOpenAPI(v cue.Value, name, version string) (map[string]any, error) {
	oyaml, err := CUEValueToOAPIYAML(v, CUEOpenAPIConfig{
		Name:    name,
		Version: version,
		NameFunc: func(_ cue.Value, _ cue.Path) string {
			return ""
		},
		ExpandReferences: true,
	})
	if err != nil {
		return nil, err
	}

	back := cueOpenAPIEncoded{}
	err = goyaml.Unmarshal(oyaml, &back)
	if err != nil {
		return nil, err
	}
	if len(back.Components.Schemas) != 1 {
		// There should only be one schema here...
		// TODO: this may change with subresources--but subresources should have defined names
		return nil, fmt.Errorf("version %s has multiple schemas", version)
	}
	var schemaProps map[string]any
	for k, v := range back.Components.Schemas {
		d, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("error generating openapi schema - generated schema has invalid type")
		}
		schemaProps, ok = d["properties"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("error generating openapi schema - %s has no properties", k)
		}
	}
	// Remove the "metadata" property, as metadata can't be extended in a CRD (the k8s.Client will handle how to encode/decode the metadata)
	delete(schemaProps, "metadata")

	// CRDs have a problem with openness and the "additionalProperties: {}", we need to _instead_ use "x-kubernetes-preserve-unknown-fields": true
	replaceAdditionalProperties(schemaProps)

	return schemaProps, nil
}

func replaceAdditionalProperties(props map[string]any) {
	for _, v := range props {
		cast, ok := v.(map[string]any)
		if !ok {
			return
		}
		if val, ok := cast["additionalProperties"]; ok {
			castVal, ok := val.(map[string]any)
			if !ok {
				return
			}
			if len(castVal) == 0 {
				delete(cast, "additionalProperties")
				cast["x-kubernetes-preserve-unknown-fields"] = true
			} else if innerProps, ok := castVal["properties"]; ok {
				castInnerProps, ok := innerProps.(map[string]any)
				if !ok {
					return
				}
				replaceAdditionalProperties(castInnerProps)
				castVal["properties"] = castInnerProps
				cast["additionalProperties"] = castVal
			}
		}
		if innerProps, ok := cast["properties"]; ok {
			castInnerProps, ok := innerProps.(map[string]any)
			if !ok {
				return
			}
			replaceAdditionalProperties(castInnerProps)
			cast["properties"] = castInnerProps
		}
	}
}
