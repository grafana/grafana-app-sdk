package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/go-multierror"
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
	// Versions is a list of versions supported by this App
	Versions []ManifestVersion `json:"versions" yaml:"versions"`
	// PreferredVersion is the preferred version for API use. If empty, it will use the latest from versions.
	// For CRDs, this also dictates which version is used for storage.
	PreferredVersion string `json:"preferredVersion" yaml:"preferredVersion"`
	// Permissions is the extra permissions for non-owned kinds this app needs to operate its backend.
	// It may be nil if no extra permissions are required.
	ExtraPermissions *Permissions `json:"extraPermissions,omitempty" yaml:"extraPermissions,omitempty"`
	// Operator has information about the operator being run for the app, if there is one.
	// When present, it can indicate to the API server the URL and paths for webhooks, if applicable.
	// This is only required if you run your app as an operator and any of your kinds support webhooks for validation,
	// mutation, or conversion.
	Operator *ManifestOperatorInfo `json:"operator,omitempty" yaml:"operator,omitempty"`
}

func (m *ManifestData) IsEmpty() bool {
	return m.AppName == "" && m.Group == "" && len(m.Versions) == 0 && m.PreferredVersion == "" && m.ExtraPermissions == nil && m.Operator == nil
}

// Validate validates the ManifestData to ensure that the kind data across all Versions is consistent
func (m *ManifestData) Validate() error {
	type kindData struct {
		kind       string
		plural     string
		scope      string
		conversion bool
		version    string
	}
	var errs error
	kinds := make(map[string]kindData)
	for _, version := range m.Versions {
		for _, kind := range version.Kinds {
			if k, ok := kinds[kind.Kind]; !ok {
				k = kindData{
					kind:       kind.Kind,
					plural:     kind.Plural,
					scope:      kind.Scope,
					conversion: kind.Conversion,
					version:    version.Name,
				}
				kinds[kind.Kind] = k
			} else {
				if k.plural != kind.Plural {
					errs = multierror.Append(errs, fmt.Errorf("kind '%s' has a different plural in versions '%s' and '%s'", kind.Kind, k.version, version.Name))
				}
				if k.scope != kind.Scope {
					errs = multierror.Append(errs, fmt.Errorf("kind '%s' has a different scope in versions '%s' and '%s'", kind.Kind, k.version, version.Name))
				}
				if k.conversion != kind.Conversion {
					errs = multierror.Append(errs, fmt.Errorf("kind '%s' conversion does not match in versions '%s' and '%s'", kind.Kind, k.version, version.Name))
				}
			}
		}
	}
	return errs
}

// Kinds returns a list of ManifestKinds parsed from Versions, for compatibility with kind-centric usage
// Deprecated: this exists to support current workflows, and should not be used for new ones.
func (m *ManifestData) Kinds() []ManifestKind {
	kinds := make(map[string]ManifestKind)
	for _, version := range m.Versions {
		for _, kind := range version.Kinds {
			k, ok := kinds[kind.Kind]
			if !ok {
				k = ManifestKind{
					Kind:       kind.Kind,
					Plural:     kind.Plural,
					Scope:      kind.Scope,
					Conversion: kind.Conversion,
					Versions:   make([]ManifestKindVersion, 0),
				}
			}
			k.Versions = append(k.Versions, ManifestKindVersion{
				ManifestVersionKind: kind,
				VersionName:         version.Name,
			})
			kinds[kind.Kind] = k
		}
	}
	k := make([]ManifestKind, 0, len(kinds))
	for _, kind := range kinds {
		k = append(k, kind)
	}
	return k
}

// ManifestKind is the manifest for a particular kind, including its Kind, Scope, and Versions.
// The values for Kind, Plural, Scope, and Conversion are hoisted up from their namesakes in Versions entries
// Deprecated: this is used only for the deprecated method ManifestData.Kinds()
type ManifestKind struct {
	// Kind is the name of the kind
	Kind string `json:"kind" yaml:"kind"`
	// Scope if the scope of the kind, typically restricted to "Namespaced" or "Cluster"
	Scope string `json:"scope" yaml:"scope"`
	// Plural is the plural of the kind
	Plural string `json:"plural" yaml:"plural"`
	// Versions is the set of versions for the kind. This list should be ordered as a series of progressively later versions.
	Versions []ManifestKindVersion `json:"versions" yaml:"versions"`
	// Conversion is true if the app has a conversion capability for this kind
	Conversion bool `json:"conversion" yaml:"conversion"`
}

// ManifestKindVersion is an extension on ManifestVersionKind that adds the version name
// Deprecated: this type if used only as part of the deprecated method ManifestData.Kinds()
type ManifestKindVersion struct {
	ManifestVersionKind `json:",inline" yaml:",inline"`
	VersionName         string `json:"versionName" yaml:"versionName"`
}

type ManifestVersion struct {
	// Name is the version name string, such as "v1" or "v1alpha1"
	Name string `json:"name" yaml:"name"`
	// Served dictates whether this version is served by the API server.
	// A version cannot be removed from a manifest until it is no longer served.
	Served bool `json:"served" yaml:"served"`
	// Kinds is a list of all the kinds served in this version.
	// Generally, kinds should exist in each version unless they have been deprecated (and no longer exist in a newer version)
	// or newly added (and didn't exist for older versions).
	Kinds []ManifestVersionKind `json:"kinds" yaml:"kinds"`
	// Routes is a map of path patterns to custom routes for this version.
	// Routes should not conflict with the plural name of any kinds for this version.
	Routes map[string]spec3.PathProps `json:"routes,omitempty" yaml:"routes,omitempty"`
}

// ManifestVersionKind contains details for a version of a kind in a Manifest
type ManifestVersionKind struct {
	// Kind is the name of the kind. This should begin with a capital letter and be CamelCased
	Kind string `json:"kind" yaml:"kind"`
	// Plural is the plural version of `kind`. This is optional and defaults to the kind + "s" if not present.
	Plural string `json:"plural,omitempty" yaml:"plural,omitempty"`
	// Scope dictates the scope of the kind. This field must be the same for all versions of the kind.
	// Different values will result in an error or undefined behavior.
	Scope string `json:"scope" yaml:"scope"`
	// Admission is the collection of admission capabilities for this version.
	// If nil, no admission capabilities exist for the version.
	Admission *AdmissionCapabilities `json:"admission,omitempty" yaml:"admission,omitempty"`
	// Schema is the schema of this version, as an OpenAPI document.
	// This is currently an `any` type as implementation is incomplete.
	Schema *VersionSchema `json:"schema,omitempty" yaml:"schema,omitempty"`
	// SelectableFields are the set of JSON paths in the schema which can be used as field selectors
	SelectableFields []string `json:"selectableFields,omitempty" yaml:"selectableFields,omitempty"`
	// Routes is a map of path patterns to custom routes for this kind to be used as custom subresource routes.
	Routes map[string]spec3.PathProps `json:"routes,omitempty" yaml:"routes,omitempty"`
	// Conversion indicates whether this kind supports custom conversion behavior exposed by the Convert method in the App.
	// It may not prevent automatic conversion behavior between versions of the kind when set to false
	// (for example, CRDs will always support simple conversion, and this flag enables webhook conversion).
	// This field should be the same for all versions of the kind. Different values will result in an error or undefined behavior.
	Conversion bool `json:"conversion" yaml:"conversion"`

	AdditionalPrinterColumns []ManifestVersionKindAdditionalPrinterColumn `json:"additionalPrinterColumns,omitempty" yaml:"additionalPrinterColumns,omitempty"`
}

type ManifestVersionKindAdditionalPrinterColumn struct {
	// name is a human readable name for the column.
	Name string `json:"name"`
	// type is an OpenAPI type definition for this column.
	// See https://github.com/OAI/OpenAPI-Specification/blob/master/versions/2.0.md#data-types for details.
	Type string `json:"type"`
	// format is an optional OpenAPI type definition for this column. The 'name' format is applied
	// to the primary identifier column to assist in clients identifying column is the resource name.
	// See https://github.com/OAI/OpenAPI-Specification/blob/master/versions/2.0.md#data-types for details.
	Format string `json:"format,omitempty"`
	// description is a human readable description of this column.
	Description string `json:"description,omitempty"`
	// priority is an integer defining the relative importance of this column compared to others. Lower
	// numbers are considered higher priority. Columns that may be omitted in limited space scenarios
	// should be given a priority greater than 0.
	Priority *int32 `json:"priority,omitempty"`
	// jsonPath is a simple JSON path (i.e. with array notation) which is evaluated against
	// each custom resource to produce the value for this column.
	JSONPath string `json:"jsonPath"`
}

const parsedCRDSchemaKindName = "__KIND__"

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

func VersionSchemaFromMap(openAPISchema map[string]any, kindName string) (*VersionSchema, error) {
	vs := &VersionSchema{
		raw: openAPISchema,
	}
	err := vs.fixRaw()
	// replace parsedCRDSchemaKindName with the kindName
	if _, ok := vs.raw[parsedCRDSchemaKindName]; ok {
		vs.raw[kindName] = vs.raw[parsedCRDSchemaKindName]
		delete(vs.raw, parsedCRDSchemaKindName)
	}
	return vs, err
}

// VersionSchema represents the schema of a KindVersion in a Manifest.
// It allows retrieval of the schema in a variety of ways, and can be unmarshaled from a CRD's version schema,
// an OpenAPI document for a kind, or from just the schemas component of an openAPI document.
// It marshals to the schemas component of an openAPI document.
// A Manifest VersionSchema does not contain a metadata object, as that is consistent between every app platform kind.
// This is modeled after kubernetes' behavior for describing a CRD schema.
type VersionSchema struct {
	// raw is the openAPI components.schemas section of an openAPI document, represented as a map[string]any
	raw map[string]any
	// serilizeAsCRD is a control flag for MarshalJSON/MarshalYAML to serialize as CRD-compatible JSON/YAML,
	// instead of a standard OpenAPI scheme. It is used by higher-level marshaling.
	// To be used, it should be set to the schema to serialize as a CRD
	serilizeAsCRD string
}

// CRDMarshalable returns a copy of this VersionSchema which will marshal into CRD-compatible JSON/YAML for the provided kindName
func (v *VersionSchema) CRDMarshalable(kindName string) *VersionSchema {
	cpy := VersionSchema{
		raw:           make(map[string]any),
		serilizeAsCRD: kindName,
	}
	maps.Copy(cpy.raw, v.raw)
	return &cpy
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
	if v.serilizeAsCRD != "" {
		conv, err := v.AsCRDMap(v.serilizeAsCRD)
		if err != nil {
			return nil, err
		}
		return json.Marshal(conv)
	}
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
	if v.serilizeAsCRD != "" {
		return v.AsCRDMap(v.serilizeAsCRD)
	}
	return v.raw, nil
}

// fixRaw turns a full OpenAPI document map[string]any in raw into a set of schemas (if required)
func (v *VersionSchema) fixRaw() error {
	if components, ok := v.raw["components"]; ok {
		cast, ok := components.(map[string]any)
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
		v.raw = schemas
		return nil
	}

	if _, ok := v.raw["openAPIV3Schema"]; ok {
		// CRD-like schema, we have to convert this into a set of "components",
		// but we don't know the object name. In this case, we use "KIND" as the name,
		// which can be corrected later if necessary.
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
		v.raw = map[string]any{
			parsedCRDSchemaKindName: map[string]any{
				"properties": m,
				"type":       "object",
			},
		}
		return nil
	}

	// There's another way something might be CRD-shaped, and that's the contents of openAPIV3Schema.properties
	// If the map contains `spec`, and none of the root objects have a `spec` property that references it,
	// we can make the assumption that this is actually a CRD-shaped-object
	if _, ok := v.raw["spec"]; ok {
		for k, v := range v.raw {
			if k == "spec" {
				continue
			}
			cast, ok := v.(map[string]any)
			if !ok {
				continue
			}
			props, ok := cast["properties"]
			if !ok {
				continue
			}
			cast, ok = props.(map[string]any)
			if !ok {
				continue
			}
			spec, ok := cast["spec"]
			if !ok {
				continue
			}
			cast, ok = spec.(map[string]any)
			if !ok {
				continue
			}
			if ref, ok := cast["$ref"]; ok {
				if cr, ok := ref.(string); ok && len(cr) > 5 && cr[len(cr)-5:] == "/spec" {
					return nil // spec is referenced by another object's `spec`, this isn't a CRD
				}
			}
		}
		// This seems to be CRD-shaped, handle it like a CRD
		kind := make(map[string]any)
		props := make(map[string]any)
		root := make(map[string]any)
		kind["properties"] = props
		kind["type"] = "object"
		for k, v := range v.raw {
			// If the field starts with #, it's a definition, lift it out of the object
			if len(k) > 1 && k[0] == '#' {
				root[k] = v
				continue
			}
			props[k] = v
		}
		v.raw = map[string]any{
			parsedCRDSchemaKindName: kind,
		}
		maps.Copy(v.raw, root)
	}
	return nil
}

// AsMap returns the schema as a map[string]any version of an openAPI components.schemas section
func (v *VersionSchema) AsMap() map[string]any {
	return v.raw
}

// AsCRDMap returns the schema as a map[string]any where each key is a top-level resource (ex. 'spec', 'status')
// if the kindObjectName provided doesn't exist in the underlying raw openAPI schemas,
// or the schema's references cannot be resolved into a single object, an error will be returned.
func (v *VersionSchema) AsCRDMap(kindObjectName string) (map[string]any, error) {
	sch, err := v.AsCRDOpenAPI3(kindObjectName)
	if err != nil {
		return nil, err
	}
	dest := make(map[string]any)
	b, err := json.Marshal(sch.Properties)
	if err != nil {
		return nil, fmt.Errorf("error marshaling CRD OpenAPI Schema: %w", err)
	}
	err = json.Unmarshal(b, &dest)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling CRD OpenAPI bytes to map[string]any: %w", err)
	}
	return dest, nil
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

// AsCRDOpenAPI3 returns an openapi3.Schema instances for a CRD for the kind.
// References in the schema will be resolved and embedded, and recursive references
// will be converted into empty objects with `x-kubernetes-preserve-unknown-fields: true`.
// The root object for the CRD in the version components is specified by kindObjectName.
// If kindObjectName does not exist in the list of schemas, an error will be returned.
func (v *VersionSchema) AsCRDOpenAPI3(kindObjectName string) (*openapi3.Schema, error) {
	components, err := v.AsOpenAPI3()
	if err != nil {
		return nil, err
	}
	if _, ok := components.Schemas[kindObjectName]; !ok {
		// Unfixed CRD schema parsed, the only way to get here is from parsing CRD schema directly with UnmarshalJSON
		if _, ok := components.Schemas[parsedCRDSchemaKindName]; !ok {
			return GetCRDOpenAPISchema(components, parsedCRDSchemaKindName)
		}
	}
	return GetCRDOpenAPISchema(components, kindObjectName)
}

// AsKubeOpenAPI converts the schema into a map of reference string to common.OpenAPIDefinition objects, suitable for use with kubernetes API server code.
// It uses the provided schema.GroupVersionKind and pkgPrefix for naming of the kind and for reference naming. The map output will look something like:
//
//	"<pkgPrefix>.<kind>": {...},
//	"<pkgPrefix>.<kind>List": {...},
//	"<pkgPrefix>.<kind>Spec": {...}, // ...etc. for all other resources
//
// If you wish to exclude a field from your kind's object, ensure that the field name begins with a `#`, which will be treated as a definition.
// Definitions are included in the returned map as types, but are not included as fields (alongside "spec","status", etc.) in the kind object.
//
// It will error if the underlying schema cannot be parsed as valid openAPI.
//
//nolint:funlen
func (v *VersionSchema) AsKubeOpenAPI(gvk schema.GroupVersionKind, ref common.ReferenceCallback, pkgPrefix string) (map[string]common.OpenAPIDefinition, error) {
	// Convert the kin-openapi to kube-openapi
	oapi, err := v.AsOpenAPI3()
	if err != nil {
		return nil, fmt.Errorf("error converting OpenAPI Schema: %w", err)
	}
	// Check if we have underlying CRD data that hasn't been appropriately labeled
	if crd, ok := oapi.Schemas[parsedCRDSchemaKindName]; ok {
		// If so, assume that the CRD data is the kind we're looking for
		oapi.Schemas[gvk.Kind] = crd
		delete(oapi.Schemas, parsedCRDSchemaKindName)
	}
	result := make(map[string]common.OpenAPIDefinition)

	// Get the kind object, strip out the metadata field if present, and format it correctly for k8s
	// The key for the kind could be the Kind, "<anystring>.<Kind>", or `parsedCRDSchemaKindName` ("__KIND__") (in edge cases)
	var kindSchema *openapi3.SchemaRef
	kindSchemaKey := ""
	for k := range oapi.Schemas {
		if strings.ToLower(k) == strings.ToLower(gvk.Kind) || k == parsedCRDSchemaKindName {
			kindSchema = oapi.Schemas[k]
			kindSchemaKey = k
			break
		}
		parts := strings.Split(k, ".")
		if len(parts) > 1 && strings.ToLower(parts[len(parts)-1]) == strings.ToLower(gvk.Kind) {
			kindSchema = oapi.Schemas[k]
			kindSchemaKey = k
			break
		}
	}
	if kindSchema == nil {
		return nil, fmt.Errorf("unable to locate openAPI definition for kind %s", gvk.Kind)
	}
	if kindSchema.Ref != "" {
		fmt.Println("uh oh, ", kindSchema.Ref)
		// TODO: Resolve
	}
	if len(kindSchema.Value.AllOf) > 0 || len(kindSchema.Value.AnyOf) > 0 || len(kindSchema.Value.OneOf) > 0 || kindSchema.Value.Not != nil {
		return nil, fmt.Errorf("anyOf, allOf, oneOf, and not are unsupported for the kind's root schema (kind %s)", gvk.Kind)
	}
	// Prefix all the refs the same way
	// Name the schema as <pkgPrefix>.<Kind><schema>
	// This ensures no conflicts when merging with other OpenAPI defs later
	refKey := func(k string) string {
		ucK := strings.ToUpper(k)
		if len(k) > 1 {
			ucK = strings.ToUpper(k[:1]) + k[1:]
		}
		return fmt.Sprintf("%s.%s%s", pkgPrefix, gvk.Kind, ucK)
	}

	// Construct the new kind based on this entry
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
	kind.Dependencies = append(kind.Dependencies, "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta")
	// Add the non-metadata properties
	for k, v := range kindSchema.Value.Properties {
		if k == "metadata" || k == "apiVersion" || k == "kind" {
			continue
		}
		sch, deps := oapi3SchemaToKubeSchema(v, ref, gvk, refKey)
		kind.Schema.Properties[k] = sch
		kind.Dependencies = append(kind.Dependencies, deps...)
	}

	// For each schema, create an entry in the result
	for k, s := range oapi.Schemas {
		if k == kindSchemaKey {
			continue
		}
		key := refKey(k)
		sch, deps := oapi3SchemaToKubeSchema(s, ref, gvk, refKey)
		// sort dependencies for consistent output
		slices.Sort(deps)
		result[key] = common.OpenAPIDefinition{
			Schema:       sch,
			Dependencies: deps,
		}
		// If the key begins with a #, it's definition and should not be included in the kind object
		/*if len(k) > 0 && k[0] == '#' {
			continue
		}
		// Add the entry as a dependency in the kind object, and add it as a subresource
		kind.Dependencies = append(kind.Dependencies, key)
		kind.Schema.Properties[k] = spec.Schema{
			SchemaProps: spec.SchemaProps{
				Default: map[string]any{},
				Ref:     ref(key),
			},
		}*/
	}
	// sort dependencies for consistent output
	slices.Sort(kind.Dependencies)

	// add the kind object to our result map
	result[fmt.Sprintf("%s.%s", pkgPrefix, gvk.Kind)] = kind
	// add the kind list object to our result map (static object type based on the kind object)
	result[fmt.Sprintf("%s.%sList", pkgPrefix, gvk.Kind)] = common.OpenAPIDefinition{
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
										Ref:     ref(fmt.Sprintf("%s.%s", pkgPrefix, gvk.Kind)),
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
			"k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta", fmt.Sprintf("%s.%s", pkgPrefix, gvk.Kind)},
	}

	return result, nil
}

// oapi3SchemaToKubeSchema converts a SchemaRef into a spec.Schema and its dependencies.
// It requires a ReferenceCallback for creating any references, and uses the gvk to rename references as "<group>/<version>.<reference>"
//
//nolint:funlen,unparam
func oapi3SchemaToKubeSchema(sch *openapi3.SchemaRef, ref common.ReferenceCallback, gvk schema.GroupVersionKind, refReplacer func(string) string) (resSchema spec.Schema, dependencies []string) {
	if sch.Ref != "" {
		// Reformat the ref to use the path derived from the GVK
		schRef := refReplacer(strings.TrimPrefix(sch.Ref, "#/components/schemas/"))
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
		s, deps := oapi3SchemaToKubeSchema(sch.Value.AdditionalProperties.Schema, ref, gvk, refReplacer)
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
			s, deps := oapi3SchemaToKubeSchema(v, ref, gvk, refReplacer)
			resSchema.AllOf = append(resSchema.AllOf, s)
			dependencies = updateDependencies(dependencies, deps)
		}
	}
	if sch.Value.AnyOf != nil {
		resSchema.AnyOf = make([]spec.Schema, 0)
		for _, v := range sch.Value.AnyOf {
			s, deps := oapi3SchemaToKubeSchema(v, ref, gvk, refReplacer)
			resSchema.AnyOf = append(resSchema.AnyOf, s)
			dependencies = updateDependencies(dependencies, deps)
		}
	}
	if sch.Value.OneOf != nil {
		resSchema.OneOf = make([]spec.Schema, 0)
		for _, v := range sch.Value.OneOf {
			s, deps := oapi3SchemaToKubeSchema(v, ref, gvk, refReplacer)
			resSchema.OneOf = append(resSchema.OneOf, s)
			dependencies = updateDependencies(dependencies, deps)
		}
	}
	if sch.Value.Not != nil {
		s, deps := oapi3SchemaToKubeSchema(sch.Value.Not, ref, gvk, refReplacer)
		resSchema.Not = &s
		dependencies = updateDependencies(dependencies, deps)
	}

	// Items
	if sch.Value.Items != nil {
		s, deps := oapi3SchemaToKubeSchema(sch.Value.Items, ref, gvk, refReplacer)
		resSchema.Items = &spec.SchemaOrArray{
			Schema: &s,
		}
		dependencies = updateDependencies(dependencies, deps)
	}

	// Properties (recursive evaluation)
	if len(sch.Value.Properties) > 0 {
		resSchema.Properties = make(map[string]spec.Schema)
		for k, v := range sch.Value.Properties {
			s, deps := oapi3SchemaToKubeSchema(v, ref, gvk, refReplacer)
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

func replaceRefs(schema *openapi3.SchemaRef, replaceFunc func(string) string) {
	if schema.Ref != "" {
		schema.Ref = replaceFunc(schema.Ref)
	}
	if schema.Value != nil {

	}
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

const extKubernetesPreserveUnknownFields = "x-kubernetes-preserve-unknown-fields"

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
	// remove the 'metadata','kind', and 'apiVersion' properties
	if schema.Value != nil {
		delete(schema.Value.Properties, "metadata")
		delete(schema.Value.Properties, "kind")
		delete(schema.Value.Properties, "apiVersion")
	}

	visited := make(map[string]bool)
	return resolveSchema(schema, components, visited)
}

//nolint:gocognit,funlen,gocritic
func resolveSchema(schema *openapi3.SchemaRef, components *openapi3.Components, visitedBefore map[string]bool) (*openapi3.Schema, error) {
	if schema == nil {
		return nil, nil
	}
	// copy visisted so referencing something in multiple places doesn't look like a cycle,
	// it's only a cycle if we visit it multiple times while recursing down
	visited := make(map[string]bool)
	maps.Copy(visited, visitedBefore)

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
			// if the schema exists, there are no properties in it, and it's either an object or empty type ("additionalProperties":{"type":"object"} or "additionalProperties":{}),
			// set kubernetes' x-kubernetes-preserve-unknown-fields to true and remove the additionalProperties section
			if result.AdditionalProperties.Schema.Value != nil && len(result.AdditionalProperties.Schema.Value.Properties) == 0 && (result.AdditionalProperties.Schema.Value.Type.Is(openapi3.TypeObject) || result.AdditionalProperties.Schema.Value.Type == nil) {
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
