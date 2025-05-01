package app

import (
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
	"k8s.io/kube-openapi/pkg/spec3"
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

// func (v *VersionSchema) AsKubeOpenAPI(kindName string, ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
// TODO convert AsOpenAPI to kube-openapi?
//	return nil
// }

type ManifestOperatorInfo struct {
	URL      string                             `json:"url" yaml:"url"`
	Webhooks *ManifestOperatorWebhookProperties `json:"webhooks,omitempty" yaml:"webhooks,omitempty"`
}

type ManifestOperatorWebhookProperties struct {
	ConversionPath string `json:"conversionPath" yaml:"conversionPath"`
	ValidationPath string `json:"validationPath" yaml:"validationPath"`
	MutationPath   string `json:"mutationPath" yaml:"mutationPath"`
}
