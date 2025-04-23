//nolint:goconst
package jennies

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/grafana/codejen"
	"github.com/grafana/cog"

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
			Name: fmt.Sprintf("%s.%s", props.PluralMachineName, props.Group),
		},
		Spec: k8s.CustomResourceDefinitionSpec{
			Group: props.Group,
			Scope: props.Scope,
			Names: k8s.CustomResourceDefinitionSpecNames{
				Kind:   props.Kind,
				Plural: props.PluralMachineName,
			},
			Versions: make([]k8s.CustomResourceDefinitionSpecVersion, 0),
		},
	}

	if kind.Properties().Conversion && kind.Properties().ConversionWebhookProps.URL != "" {
		webhookURL, err := url.Parse(kind.Properties().ConversionWebhookProps.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid conversion webhook URL: %w", err)
		}
		resource.Spec.Conversion = &k8s.CustomResourceDefinitionSpecConversion{
			Strategy: "webhook",
			Webhook: &k8s.CustomResourceDefinitionSpecConversionWebhook{
				ConversionReviewVersions: []string{"v1"},
				ClientConfig: k8s.CustomResourceDefinitionClientConfig{
					URL: webhookURL.String(),
				},
			},
		}
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

	return codejen.NewFile(fmt.Sprintf("%s.%s.%s", kind.Properties().MachineName, kind.Properties().Group, c.outputExtension), contents, c), nil
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

const extKubernetesPreserveUnknownFields = "x-kubernetes-preserve-unknown-fields"

func CUEToCRDOpenAPI(v cue.Value, name, version string) (map[string]any, error) {
	codegenPipeline := cog.TypesFromSchema().
		CUEValue(name, v, cog.ForceEnvelope(name)).
		GenerateOpenAPI(cog.OpenAPIGenerationConfig{})
	files, err := codegenPipeline.Run(context.Background())
	if err != nil {
		return nil, err
	}
	// should only be one file
	if len(files) != 1 {
		return nil, fmt.Errorf("expected one OpenAPI definition but got %d", len(files))
	}
	// Parse the JSON in the file into openAPI components
	doc, err := openapi3.NewLoader().LoadFromData(files[0].Data)
	if err != nil {
		return nil, err
	}
	converted, err := GetCRDOpenAPISchema(doc.Components, name)
	if err != nil {
		return nil, err
	}
	// Delete the "metadata" property
	delete(converted.Properties, "metadata")
	// cog-generated openAPI doesn't consider CUE values which have fields other than _ or [string]:any in them to be open,
	// so they don't have additionalProperties/x-kubernetes-preserve-unknown-fields
	// TODO: @IfSentient file a bug for this with cog
	// TODO: @IfSentient CUE actually considers all structs to be open, so might need to re-map to a definition before handing off to cog
	// In the meantime, use the @appsdk(open=true) attribute to tell the generator to consider a struct open
	for k, sch := range converted.Properties {
		val := v.LookupPath(cue.MakePath(cue.Str(k)))
		if !val.Exists() {
			continue
		}
		attr := val.Attribute("appsdk")
		if attr.Err() != nil {
			continue
		}
		attrVal, found, _ := attr.Lookup(0, "open")
		if found {
			if len(attrVal) >= 1 && attrVal[0] == 't' || attrVal[0] == 'T' {
				if sch.Value.Extensions == nil {
					sch.Value.Extensions = make(map[string]any)
				}
				sch.Value.Extensions[extKubernetesPreserveUnknownFields] = true
				converted.Properties[k] = sch
			}
		}
		/*fmt.Println("check ", k)
		chk := val.Context().CompileString("{[string]: _}")
		if chk.Err() != nil {
			return nil, chk.Err()
		}
		if err := val.Subsume(chk, cue.Final()); err != nil {
			fmt.Println("subsume err "+k+":", err)
		}
		if val.Eval().Allows(cue.AnyString) {
			fmt.Println(CUEValueToString(val.Eval()))
			fmt.Println("allows ", k)
			if sch.Value == nil {
				// We shouldn't be able to hit this, but just in case, let's return something nice instead of panicking
				return nil, fmt.Errorf("property %s has no value after transforming with GetCRDOpenAPISchema", k)
			}
			if sch.Value.Extensions == nil {
				sch.Value.Extensions = make(map[string]any)
			}
			sch.Value.Extensions[extKubernetesPreserveUnknownFields] = true
			converted.Properties[k] = sch
		}*/
	}
	// Convert to JSON and then into a map
	j, err := json.MarshalIndent(converted.Properties, "", "  ")
	if err != nil {
		return nil, err
	}
	m := make(map[string]any)
	err = json.Unmarshal(j, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// GetCRDOpenAPISchema takes a Components object and a schema name, resolves all $ref references
// and handles recursive references by converting them to objects with x-kubernetes-preserve-unknown-fields set to true.
// It returns the resolved schema and any error encountered.
func GetCRDOpenAPISchema(components *openapi3.Components, schemaName string) (*openapi3.Schema, error) {
	if components == nil || components.Schemas == nil {
		return nil, fmt.Errorf("invalid components or schemas")
	}

	schema := components.Schemas[schemaName]
	if schema == nil {
		return nil, fmt.Errorf("schema %s not found", schemaName)
	}

	visited := make(map[string]bool)
	return resolveSchema(schema, components, visited)
}

func resolveSchema(schema *openapi3.SchemaRef, components *openapi3.Components, visited map[string]bool) (*openapi3.Schema, error) {
	if schema == nil {
		return nil, nil
	}

	// If this is a reference, resolve it
	if schema.Ref != "" {
		refName := getRefName(schema.Ref)

		// Check if we've seen this reference before
		if visited[refName] {
			// We've found a cycle, return object with x-kubernetes-preserve-unknown-fields
			return &openapi3.Schema{
				Type:       &openapi3.Types{openapi3.TypeObject},
				Extensions: map[string]any{extKubernetesPreserveUnknownFields: true},
			}, nil
		}

		// Mark this reference as visited
		visited[refName] = true

		// Get the referenced schema
		refSchema := components.Schemas[refName]
		if refSchema == nil {
			return nil, fmt.Errorf("referenced schema %s not found", refName)
		}

		// Create a new visited map for this branch to avoid false positives in parallel branches
		branchVisited := make(map[string]bool)
		for k, v := range visited {
			branchVisited[k] = v
		}

		// Resolve the referenced schema
		resolved, err := resolveSchema(refSchema, components, branchVisited)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve reference %s: %w", refName, err)
		}

		return resolved, nil
	}

	// Create a new schema to avoid modifying the original
	result := &openapi3.Schema{
		Type:                 schema.Value.Type,
		Format:               schema.Value.Format,
		Description:          schema.Value.Description,
		Default:              schema.Value.Default,
		Example:              schema.Value.Example,
		ExclusiveMin:         schema.Value.ExclusiveMin,
		ExclusiveMax:         schema.Value.ExclusiveMax,
		Min:                  schema.Value.Min,
		Max:                  schema.Value.Max,
		MultipleOf:           schema.Value.MultipleOf,
		MinLength:            schema.Value.MinLength,
		MaxLength:            schema.Value.MaxLength,
		Pattern:              schema.Value.Pattern,
		MinItems:             schema.Value.MinItems,
		MaxItems:             schema.Value.MaxItems,
		UniqueItems:          schema.Value.UniqueItems,
		MinProps:             schema.Value.MinProps,
		MaxProps:             schema.Value.MaxProps,
		Required:             schema.Value.Required,
		Enum:                 schema.Value.Enum,
		Title:                schema.Value.Title,
		AdditionalProperties: schema.Value.AdditionalProperties,
		Nullable:             schema.Value.Nullable,
		ReadOnly:             schema.Value.ReadOnly,
		WriteOnly:            schema.Value.WriteOnly,
		AllOf:                make([]*openapi3.SchemaRef, 0),
		OneOf:                make([]*openapi3.SchemaRef, 0),
		AnyOf:                make([]*openapi3.SchemaRef, 0),
	}

	// Fix additionalProperties being an empty object for what kubernetes CRD's expect (using the `x-kubernetes-preserve-unknown-fields` extension)
	if result.AdditionalProperties.Has != nil || result.AdditionalProperties.Schema != nil {
		if result.AdditionalProperties.Schema != nil {
			// If there's a schema, resolve references and check if we need to transform this into a plain object with x-kubernetes-preserve-unknown-fields: true
			if result.AdditionalProperties.Schema.Ref != "" {
				resolved, err := resolveSchema(result.AdditionalProperties.Schema, components, visited)
				if err != nil {
					return nil, err
				}
				result.AdditionalProperties.Schema = openapi3.NewSchemaRef("", resolved)
			}
			if result.AdditionalProperties.Schema.Value != nil && result.AdditionalProperties.Schema.Value.Type.Is(openapi3.TypeObject) && len(result.AdditionalProperties.Schema.Value.Properties) == 0 {
				result.AdditionalProperties.Has = nil
				result.AdditionalProperties.Schema = nil
				if result.Extensions == nil {
					result.Extensions = make(map[string]any)
				}
				result.Extensions[extKubernetesPreserveUnknownFields] = true
			}
		} else if *result.AdditionalProperties.Has {
			// If AdditionalProperties.Schema is nil, then remove AdditionalProperties and set x-kubernetes-preserve-unknown-fields to true
			result.AdditionalProperties.Has = nil
			result.AdditionalProperties.Schema = nil
			if result.Extensions == nil {
				result.Extensions = make(map[string]any)
			}
			result.Extensions[extKubernetesPreserveUnknownFields] = true
		} else {
			result.AdditionalProperties.Has = nil
		}
	}

	// Resolve properties for objects
	if schema.Value.Properties != nil {
		result.Properties = make(map[string]*openapi3.SchemaRef)
		for name, prop := range schema.Value.Properties {
			resolved, err := resolveSchema(prop, components, visited)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve property %s: %w", name, err)
			}
			result.Properties[name] = openapi3.NewSchemaRef("", resolved)
		}
	}

	// Resolve items for arrays
	if schema.Value.Items != nil {
		resolved, err := resolveSchema(schema.Value.Items, components, visited)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve array items: %w", err)
		}
		result.Items = openapi3.NewSchemaRef("", resolved)
	}

	// Resolve AllOf schemas
	for _, s := range schema.Value.AllOf {
		resolved, err := resolveSchema(s, components, visited)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve allOf schema: %w", err)
		}
		result.AllOf = append(result.AllOf, openapi3.NewSchemaRef("", resolved))
	}

	// Resolve OneOf schemas
	for _, s := range schema.Value.OneOf {
		resolved, err := resolveSchema(s, components, visited)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve oneOf schema: %w", err)
		}
		result.OneOf = append(result.OneOf, openapi3.NewSchemaRef("", resolved))
	}

	// Resolve AnyOf schemas
	for _, s := range schema.Value.AnyOf {
		resolved, err := resolveSchema(s, components, visited)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve anyOf schema: %w", err)
		}
		result.AnyOf = append(result.AnyOf, openapi3.NewSchemaRef("", resolved))
	}

	return result, nil
}

// getRefName extracts the schema name from a $ref string
func getRefName(ref string) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}
