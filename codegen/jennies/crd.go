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

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/codegen"
	"github.com/grafana/grafana-app-sdk/k8s"
)

// CRDOutputEncoder is a function which marshals an object into a desired output format
type CRDOutputEncoder func(any) ([]byte, error)

func CRDGenerator(encoder CRDOutputEncoder, extension string) codejen.OneToMany[codegen.AppManifest] {
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

type kindWithVersion struct {
	version string
	kind    codegen.VersionedKind
}

func (c *crdGenerator) Generate(appManifest codegen.AppManifest) (codejen.Files, error) {
	files := make(codejen.Files, 0)

	// Need to group all versions of a kind together to make a CRD (each CRD contains schemas for all versions)
	kinds := make(map[string][]kindWithVersion)
	for version, kind := range codegen.VersionedKinds(appManifest) {
		kv, ok := kinds[kind.Kind]
		if !ok {
			kv = make([]kindWithVersion, 0)
		}
		kv = append(kv, kindWithVersion{
			version: version.Name(),
			kind:    kind,
		})
		kinds[kind.Kind] = kv
	}

	for _, kv := range kinds {
		// Edge case that should never happen, but just in case
		if len(kv) == 0 {
			continue
		}
		resource := customResourceDefinition{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
			Metadata: customResourceDefinitionMetadata{
				Name: fmt.Sprintf("%s.%s", kv[0].kind.PluralMachineName, appManifest.Properties().FullGroup),
			},
			Spec: k8s.CustomResourceDefinitionSpec{
				Group: appManifest.Properties().FullGroup,
				Scope: kv[0].kind.Scope,
				Names: k8s.CustomResourceDefinitionSpecNames{
					Kind:   kv[0].kind.Kind,
					Plural: kv[0].kind.PluralMachineName,
				},
				Versions: make([]k8s.CustomResourceDefinitionSpecVersion, 0),
			},
		}

		if kv[0].kind.Conversion && kv[0].kind.ConversionWebhookProps.URL != "" {
			webhookURL, err := url.Parse(kv[0].kind.ConversionWebhookProps.URL)
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

		for _, vs := range kv {
			v, err := KindVersionToCRDSpecVersion(vs.kind.Schema, vs.kind, vs.version, vs.version == appManifest.Properties().PreferredVersion)
			if err != nil {
				return nil, err
			}

			// Check for edge case that results in CRDs that may not work with discovery, but should still be allowed to work.
			// If there is only one version, storage must always be true.
			if len(kv) == 1 {
				v.Storage = true
			}
			resource.Spec.Versions = append(resource.Spec.Versions, v)
		}

		contents, err := c.outputEncoder(resource)
		if err != nil {
			return nil, err
		}
		files = append(files, codejen.File{
			Data:         contents,
			RelativePath: fmt.Sprintf("%s.%s.%s", kv[0].kind.MachineName, appManifest.Properties().FullGroup, c.outputExtension),
			From:         []codejen.NamedJenny{c},
		})
	}

	return files, nil
}

func KindVersionToCRDSpecVersion(schema cue.Value, kind codegen.VersionedKind, version string, stored bool) (k8s.CustomResourceDefinitionSpecVersion, error) {
	props, err := cueToCRDOpenAPI(schema, kind.Kind)
	if err != nil {
		return k8s.CustomResourceDefinitionSpecVersion{}, err
	}

	def := k8s.CustomResourceDefinitionSpecVersion{
		Name:    version,
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
	if len(kind.SelectableFields) > 0 {
		sf := make([]k8s.CustomResourceDefinitionSelectableField, len(kind.SelectableFields))
		for i, field := range kind.SelectableFields {
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

	if len(kind.AdditionalPrinterColumns) > 0 {
		apc := make([]k8s.CustomResourceDefinitionAdditionalPrinterColumn, len(kind.AdditionalPrinterColumns))
		for i, col := range kind.AdditionalPrinterColumns {
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

func cueToOpenAPIBytes(v cue.Value, name string) ([]byte, error) {
	defpath := cue.MakePath(cue.Def(name))
	val := v.Context().CompileString(fmt.Sprintf("#%s: _", name))
	defsch := val.FillPath(defpath, v)
	codegenPipeline := cog.TypesFromSchema().
		CUEValue(name, defsch, cog.ForceEnvelope(name)).
		GenerateOpenAPI(cog.OpenAPIGenerationConfig{})
	files, err := codegenPipeline.Run(context.Background())
	if err != nil {
		return nil, err
	}
	// should only be one file
	if len(files) != 1 {
		return nil, fmt.Errorf("expected one OpenAPI definition but got %d", len(files))
	}
	return files[0].Data, nil
}

// TODO: once the CRD jenny uses a manifest, we can use the same process as the manifest jenny
func cueToCRDOpenAPI(v cue.Value, name string) (map[string]any, error) {
	defpath := cue.MakePath(cue.Def(name))
	val := v.Context().CompileString(fmt.Sprintf("#%s: _", name))
	defsch := val.FillPath(defpath, v)
	codegenPipeline := cog.TypesFromSchema().
		CUEValue(name, defsch, cog.ForceEnvelope(name)).
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
	converted, err := app.GetCRDOpenAPISchema(doc.Components, name)
	if err != nil {
		return nil, err
	}
	// Delete the "metadata" property
	delete(converted.Properties, "metadata")
	// Convert to JSON and then into a map
	j, err := json.Marshal(converted.Properties)
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
