package app

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// NewEmbeddedManifest returns a Manifest which has the ManifestData embedded in it
func NewEmbeddedManifest(manifestData ManifestData) Manifest {
	return Manifest{
		Location: ManifestLocation{
			Type: ManifestLocationEmbedded,
		},
		ManifestData: &manifestData,
	}
}

// NewOnDiskManifest returns a Manifest which points to a path on-disk to load ManifestData from
func NewOnDiskManifest(path string) Manifest {
	return Manifest{
		Location: ManifestLocation{
			Type: ManifestLocationFilePath,
			Path: path,
		},
	}
}

// NewAPIServerManifest returns a Manifest which points to a resource in an API server to load the ManifestData from
func NewAPIServerManifest(resourceName string) Manifest {
	return Manifest{
		Location: ManifestLocation{
			Type: ManifestLocationAPIServerResource,
			Path: resourceName,
		},
	}
}

// Manifest is a type which represents the Location and Data in an App Manifest.
type Manifest struct {
	// ManifestData must be present if Location.Type == "embedded"
	ManifestData *ManifestData
	// Location indicates the place where the ManifestData should be loaded from
	Location ManifestLocation
}

// ManifestLocation contains information of where a Manifest's ManifestData can be found.
type ManifestLocation struct {
	Type ManifestLocationType
	// Path is the path to the manifest, based on location.
	// For "filepath", it is the path on disk. For "apiserver", it is the NamespacedName. For "embedded", it is empty.
	Path string
}

type ManifestLocationType string

const (
	ManifestLocationFilePath          = ManifestLocationType("filepath")
	ManifestLocationAPIServerResource = ManifestLocationType("apiserver")
	ManifestLocationEmbedded          = ManifestLocationType("embedded")
)

// ManifestData is the data in a Manifest, representing the Kinds and Capabilities of an App.
// NOTE: ManifestData is still experimental and subject to change
type ManifestData struct {
	// AppName is the unique identifier for the App
	AppName string `json:"appName" yaml:"appName"`
	// Group is the group used for all kinds maintained by this app.
	// This is usually "<AppName>.ext.grafana.com"
	Group string `json:"group" yaml:"group"`
	// Kinds is a list of all Kinds maintained by this App
	Kinds []ManifestKind `json:"kinds,omitempty" yaml:"kinds,omitempty"`
	// Permissions is the extra permissions for non-owned kinds this app needs to operate its backend.
	// It may be nil if no extra permissions are required.
	ExtraPermissions *Permissions `json:"extraPermissions,omitempty" yaml:"extraPermissions,omitempty"`
	// Operator has information about the operator being run for the app, if there is one.
	// When present, it can indicate to the API server the URL and paths for webhooks, if applicable.
	// This is only required if you run your app as an operator and any of your kinds support webhooks for validation,
	// mutation, or conversion.
	Operator *ManifestOperatorInfo `json:"operator,omitempty" yaml:"operator,omitempty"`
}

// ManifestKind is the manifest for a particular kind, including its Kind, Scope, and Versions
type ManifestKind struct {
	// Kind is the name of the kind
	Kind string `json:"kind" yaml:"kind"`
	// Scope if the scope of the kind, typically restricted to "Namespaced" or "Cluster"
	Scope string `json:"scope" yaml:"scope"`
	// Versions is the set of versions for the kind. This list should be ordered as a series of progressively later versions.
	Versions []ManifestKindVersion `json:"versions" yaml:"versions"`
	// Conversion is true if the app has a conversion capability for this kind
	Conversion bool `json:"conversion" yaml:"conversion"`
}

// ManifestKindVersion contains details for a version of a kind in a Manifest
type ManifestKindVersion struct {
	// Name is the version string name, such as "v1"
	Name string `yaml:"name" json:"name"`
	// Admission is the collection of admission capabilities for this version.
	// If nil, no admission capabilities exist for the version.
	Admission *AdmissionCapabilities `json:"admission,omitempty" yaml:"admission,omitempty"`
	// Schema is the schema of this version, as an OpenAPI document.
	// This is currently an `any` type as implementation is incomplete.
	Schema *VersionSchema `json:"schema,omitempty" yaml:"schema,omitempty"`
	// SelectableFields are the set of JSON paths in the schema which can be used as field selectors
	SelectableFields []string `json:"selectableFields,omitempty" yaml:"selectableFields,omitempty"`
	// CustomRoutes is a map of of path patterns to custom routes for this version.
	CustomRoutes map[string]spec3.PathProps `json:"customRoutes,omitempty" yaml:"customRoutes,omitempty"`
}

// AdmissionCapabilities is the collection of admission capabilities of a kind
type AdmissionCapabilities struct {
	// Validation contains the validation capability details. If nil, the kind does not have a validation capability.
	Validation *ValidationCapability `json:"validation,omitempty" yaml:"validation,omitempty"`
	// Mutation contains the mutation capability details. If nil, the kind does not have a mutation capability.
	Mutation *MutationCapability `json:"mutation,omitempty" yaml:"mutation,omitempty"`
}

// SupportsAnyValidation returns true if the list of operations for validation is not empty.
// This is a convenience method to avoid having to make several nil and length checks.
func (c AdmissionCapabilities) SupportsAnyValidation() bool {
	if c.Validation == nil {
		return false
	}
	return len(c.Validation.Operations) > 0
}

// SupportsAnyMutation returns true if the list of operations for mutation is not empty.
// This is a convenience method to avoid having to make several nil and length checks.
func (c AdmissionCapabilities) SupportsAnyMutation() bool {
	if c.Mutation == nil {
		return false
	}
	return len(c.Mutation.Operations) > 0
}

// ValidationCapability is the details of a validation capability for a kind's admission control
type ValidationCapability struct {
	// Operations is the list of operations that the validation capability is used for.
	// If this list if empty or nil, this is equivalent to the app having no validation capability.
	Operations []AdmissionOperation `json:"operations,omitempty" yaml:"operations,omitempty"`
}

// MutationCapability is the details of a mutation capability for a kind's admission control
type MutationCapability struct {
	// Operations is the list of operations that the mutation capability is used for.
	// If this list if empty or nil, this is equivalent to the app having no mutation capability.
	Operations []AdmissionOperation `json:"operations,omitempty" yaml:"operations,omitempty"`
}

type AdmissionOperation string

const (
	AdmissionOperationAny     AdmissionOperation = "*"
	AdmissionOperationCreate  AdmissionOperation = "CREATE"
	AdmissionOperationUpdate  AdmissionOperation = "UPDATE"
	AdmissionOperationDelete  AdmissionOperation = "DELETE"
	AdmissionOperationConnect AdmissionOperation = "CONNECT"
)

type Permissions struct {
	AccessKinds []KindPermission `json:"accessKinds,omitempty" yaml:"accessKinds,omitempty"`
}

type KindPermissionAction string

type KindPermission struct {
	Group    string                 `json:"group" yaml:"group"`
	Resource string                 `json:"resource" yaml:"resource"`
	Actions  []KindPermissionAction `json:"actions,omitempty" yaml:"actions,omitempty"`
}

// ManifestOperatorInfo contains information on the app's Operator deployment (if a deployment exists).
// This is primarily used to specify the location of webhook endpoints for the app.
type ManifestOperatorInfo struct {
	URL      string                             `json:"url" yaml:"url"`
	Webhooks *ManifestOperatorWebhookProperties `json:"webhooks,omitempty" yaml:"webhooks,omitempty"`
}

// ManifestOperatorWebhookProperties contains information on webhook paths for an app's operator deployment.
type ManifestOperatorWebhookProperties struct {
	ConversionPath string `json:"conversionPath" yaml:"conversionPath"`
	ValidationPath string `json:"validationPath" yaml:"validationPath"`
	MutationPath   string `json:"mutationPath" yaml:"mutationPath"`
}

func VersionSchemaFromMap(openAPISchema map[string]any) (*VersionSchema, error) {
	vs := &VersionSchema{
		raw: openAPISchema,
	}
	err := vs.fixRaw()
	return vs, err
}

// VersionSchema represents the schema of a KindVersion in a Manifest.
// It allows retrieval of the schema in a variety of ways, and can be unmarshaled from a CRD's version schema,
// an OpenAPI document for a kind, or from just the schemas component of an openAPI document.
// It marshals to the schemas component of an openAPI document.
// A Manifest VersionSchema does not contain a metadata object, as that is consistent between every app platform kind.
// This is modeled after kubernetes' behavior for describing a CRD schema.
type VersionSchema struct {
	raw map[string]any
}

func (v *VersionSchema) UnmarshalJSON(data []byte) error {
	v.raw = make(map[string]any)
	err := json.Unmarshal(data, &v.raw)
	if err != nil {
		return err
	}
	return v.fixRaw()
}

func (v *VersionSchema) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.raw)
}

func (v *VersionSchema) UnmarshalYAML(unmarshal func(any) error) error {
	v.raw = make(map[string]any)
	err := unmarshal(&v.raw)
	if err != nil {
		return err
	}
	return v.fixRaw()
}

func (v *VersionSchema) MarshalYAML() (any, error) {
	// MarshalYAML needs to return an object to the marshaler, not bytes like MarshalJSON
	return v.raw, nil
}

// fixRaw turns a full OpenAPI document map[string]any in raw into a set of schemas (if required)
func (v *VersionSchema) fixRaw() error {
	if _, ok := v.raw["openapi"]; !ok {
		// Not openAPI document, check if it's CRD-Like schema
		if _, ok := v.raw["openAPIV3Schema"]; !ok {
			// ok, no adjustments (that we know of) necessary
			return nil
		}
		oapi, ok := v.raw["openAPIV3Schema"].(map[string]any)
		if !ok {
			return fmt.Errorf("'openAPIV3Schema' must be an object")
		}
		props, ok := oapi["properties"]
		if !ok {
			return fmt.Errorf("'openAPIV3Schema' must contain properties")
		}
		castProps, ok := props.(map[string]any)
		if !ok {
			return fmt.Errorf("'openAPIV3Schema' properties must be an object")
		}
		m := make(map[string]any)
		for key, value := range castProps {
			m[key] = value
		}
		v.raw = m
		return nil
	}
	if c, ok := v.raw["components"]; ok {
		cast, ok := c.(map[string]any)
		if !ok {
			return fmt.Errorf("'components' in an OpenAPI document must be an object")
		}
		s, ok := cast["schemas"]
		if !ok {
			v.raw = make(map[string]any)
			return nil
		}
		schemas, ok := s.(map[string]any)
		if !ok {
			return fmt.Errorf("'components.schemas' in an OpenAPI document must be an object")
		}
		v.raw["schemas"] = schemas
	}
	return nil
}

// AsMap returns the schema as a map[string]any where each key is a top-level resource (ex. 'spec', 'status')
func (v *VersionSchema) AsMap() map[string]any {
	return v.raw
}

// AsOpenAPI3 returns an openapi3.Components instance which contains the schema elements
func (v *VersionSchema) AsOpenAPI3() (*openapi3.Components, error) {
	full := map[string]any{
		"openapi": "3.0.0",
		"components": map[string]any{
			"schemas": v.AsMap(),
		},
	}
	yml, err := yaml.Marshal(full)
	if err != nil {
		return nil, err
	}
	loader := openapi3.NewLoader()
	oT, err := loader.LoadFromData(yml)
	if err != nil {
		return nil, err
	}
	return oT.Components, nil
}

// AsKubeOpenAPI converts the schema into a map of reference string to common.OpenAPIDefinition objects, suitable for use with kubernetes API server code.
// It uses the provided schema.GroupVersionKind for naming of the kind and for reference naming. The map output will look something like:
//
//	"<group>/<version>.<kind>": {...},
//	"<group>/<version>.<kind>List": {...},
//	"<group>/<version>.spec": {...}, // ...etc. for all other resources
//
// If you wish to exclude a field from your kind's object, ensure that the field name begins with a `#`, which will be treated as a definition.
// Definitions are included in the returned map as types, but are not included as fields (alongside "spec","status", etc.) in the kind object.
//
// It will error if the underlying schema cannot be parsed as valid openAPI.
//
//nolint:funlen
func (v *VersionSchema) AsKubeOpenAPI(gvk schema.GroupVersionKind, ref common.ReferenceCallback) (map[string]common.OpenAPIDefinition, error) {
	// Convert the kin-openapi to kube-openapi
	oapi, err := v.AsOpenAPI3()
	if err != nil {
		return nil, err
	}
	result := make(map[string]common.OpenAPIDefinition)

	kindProp := spec.Schema{
		SchemaProps: spec.SchemaProps{
			Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
			Type:        []string{"string"},
			Format:      "",
		},
	}
	apiVersionProp := spec.Schema{
		SchemaProps: spec.SchemaProps{
			Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
			Type:        []string{"string"},
			Format:      "",
		},
	}

	// kind must always be present, partially declare it here so we can add subresources and dependencies as we iterate through them
	kind := common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Type: []string{"object"},
				Properties: map[string]spec.Schema{
					"kind":       kindProp,
					"apiVersion": apiVersionProp,
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]any{},
							Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"),
						},
					},
				},
			},
		},
		Dependencies: make([]string, 0),
	}

	// For each schema, create an entry in the result
	for k, s := range oapi.Schemas {
		key := fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, k)
		sch, deps := oapi3SchemaToKubeSchema(s, ref, gvk)
		// sort dependencies for consistent output
		slices.Sort(deps)
		result[key] = common.OpenAPIDefinition{
			Schema:       sch,
			Dependencies: deps,
		}
		// If the key begins with a #, it's definition and should not be included in the kind object
		if len(k) > 0 && k[0] == '#' {
			continue
		}
		// Add the entry as a dependency in the kind object, and add it as a subresource
		kind.Dependencies = append(kind.Dependencies, key)
		kind.Schema.Properties[k] = spec.Schema{
			SchemaProps: spec.SchemaProps{
				Default: map[string]any{},
				Ref:     ref(key),
			},
		}
	}
	// sort dependencies for consistent output
	slices.Sort(kind.Dependencies)

	// add the kind object to our result map
	result[fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, gvk.Kind)] = kind
	// add the kind list object to our result map (static object type based on the kind object)
	result[fmt.Sprintf("%s/%s.%sList", gvk.Group, gvk.Version, gvk.Kind)] = common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Type: []string{"object"},
				Properties: map[string]spec.Schema{
					"kind":       kindProp,
					"apiVersion": apiVersionProp,
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]any{},
							Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta"),
						},
					},
					"items": {
						SchemaProps: spec.SchemaProps{
							Type: []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: map[string]any{},
										Ref:     ref(fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, gvk.Kind)),
									},
								},
							},
						},
					},
				},
				Required: []string{"metadata", "items"},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta", fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, gvk.Kind)},
	}

	return result, nil
}

// oapi3SchemaToKubeSchema converts a SchemaRef into a spec.Schema and its dependencies.
// It requires a ReferenceCallback for creating any references, and uses the gvk to rename references as "<group>/<version>.<reference>"
//
//nolint:funlen
func oapi3SchemaToKubeSchema(sch *openapi3.SchemaRef, ref common.ReferenceCallback, gvk schema.GroupVersionKind) (resSchema spec.Schema, dependencies []string) {
	if sch.Ref != "" {
		// Reformat the ref to use the path derived from the GVK
		schRef := fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, strings.TrimPrefix(sch.Ref, "#/components/schemas/"))
		return spec.Schema{
			SchemaProps: spec.SchemaProps{
				Ref: ref(schRef),
			},
		}, []string{schRef}
	}
	if sch.Value == nil {
		// Not valid
		return spec.Schema{}, []string{}
	}
	dependencies = make([]string, 0)
	resSchema = spec.Schema{
		SchemaProps: spec.SchemaProps{
			Type:        sch.Value.Type.Slice(),
			Format:      sch.Value.Format,
			Description: sch.Value.Description,
			Default:     sch.Value.Default,
			Minimum:     sch.Value.Min,
			Maximum:     sch.Value.Max,
			MultipleOf:  sch.Value.MultipleOf,
			Pattern:     sch.Value.Pattern,
			UniqueItems: sch.Value.UniqueItems,
			Required:    sch.Value.Required,
			Enum:        sch.Value.Enum,
			Title:       sch.Value.Title,
			Nullable:    sch.Value.Nullable,
		},
	}
	// Differing types between k8s and openapi3
	if sch.Value.MinLength != 0 {
		ml := convertUint64(sch.Value.MinLength)
		resSchema.MinLength = &ml
	}
	if sch.Value.MaxLength != nil {
		ml := convertUint64(*sch.Value.MaxLength)
		resSchema.MaxLength = &ml
	}
	if sch.Value.MinItems != 0 {
		mi := convertUint64(sch.Value.MinItems)
		resSchema.MinItems = &mi
	}
	if sch.Value.MaxItems != nil {
		mi := convertUint64(*sch.Value.MaxItems)
		resSchema.MaxItems = &mi
	}
	if sch.Value.MinProps != 0 {
		mp := convertUint64(sch.Value.MinProps)
		resSchema.MinProperties = &mp
	}
	if sch.Value.MaxProps != nil {
		mp := convertUint64(*sch.Value.MaxProps)
		resSchema.MaxProperties = &mp
	}
	// AdditionalProperties
	if sch.Value.AdditionalProperties.Has != nil {
		resSchema.AdditionalProperties = &spec.SchemaOrBool{
			Allows: *sch.Value.AdditionalProperties.Has,
		}
	}
	if sch.Value.AdditionalProperties.Schema != nil {
		s, deps := oapi3SchemaToKubeSchema(sch.Value.AdditionalProperties.Schema, ref, gvk)
		resSchema.AdditionalProperties = &spec.SchemaOrBool{
			Schema: &s,
		}
		dependencies = updateDependencies(dependencies, deps)
	}
	// Handle special case of `x-kubernetes-preserve-unknown-fields: true` to make AdditionalProperties an empty object
	if sch.Value.Extensions != nil {
		if val, ok := sch.Value.Extensions["x-kubernetes-preserve-unknown-fields"]; ok {
			if conv, ok := val.(bool); ok && conv {
				resSchema.AdditionalProperties = &spec.SchemaOrBool{
					Allows: true,
				}
			}
		}
	}

	// AllOf, AnyOf, OneOf, Not
	if sch.Value.AllOf != nil {
		resSchema.AllOf = make([]spec.Schema, 0)
		for _, v := range sch.Value.AllOf {
			s, deps := oapi3SchemaToKubeSchema(v, ref, gvk)
			resSchema.AllOf = append(resSchema.AllOf, s)
			dependencies = updateDependencies(dependencies, deps)
		}
	}
	if sch.Value.AnyOf != nil {
		resSchema.AnyOf = make([]spec.Schema, 0)
		for _, v := range sch.Value.AnyOf {
			s, deps := oapi3SchemaToKubeSchema(v, ref, gvk)
			resSchema.AnyOf = append(resSchema.AnyOf, s)
			dependencies = updateDependencies(dependencies, deps)
		}
	}
	if sch.Value.OneOf != nil {
		resSchema.OneOf = make([]spec.Schema, 0)
		for _, v := range sch.Value.OneOf {
			s, deps := oapi3SchemaToKubeSchema(v, ref, gvk)
			resSchema.OneOf = append(resSchema.OneOf, s)
			dependencies = updateDependencies(dependencies, deps)
		}
	}
	if sch.Value.Not != nil {
		s, deps := oapi3SchemaToKubeSchema(sch.Value.Not, ref, gvk)
		resSchema.Not = &s
		dependencies = updateDependencies(dependencies, deps)
	}

	// Items
	if sch.Value.Items != nil {
		s, deps := oapi3SchemaToKubeSchema(sch.Value.Items, ref, gvk)
		resSchema.Items = &spec.SchemaOrArray{
			Schema: &s,
		}
		dependencies = updateDependencies(dependencies, deps)
	}

	// Properties (recursive evaluation)
	if len(sch.Value.Properties) > 0 {
		resSchema.Properties = make(map[string]spec.Schema)
		for k, v := range sch.Value.Properties {
			s, deps := oapi3SchemaToKubeSchema(v, ref, gvk)
			resSchema.Properties[k] = s
			dependencies = updateDependencies(dependencies, deps)
		}
	}
	// Set to nil if empty
	if len(dependencies) == 0 {
		dependencies = nil
	}
	return resSchema, dependencies
}

// updateDependencies adds each entry in toAdd to the dependencies slice if absent, and returns the updated slice
func updateDependencies(dependencies []string, toAdd []string) []string {
	for _, dep := range toAdd {
		if !slices.Contains(dependencies, dep) {
			dependencies = append(dependencies, dep)
		}
	}
	return dependencies
}

func convertUint64(i uint64) int64 {
	if i > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(i)
}
