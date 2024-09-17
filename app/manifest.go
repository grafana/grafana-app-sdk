package app

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
	Schema any `json:"schema,omitempty" yaml:"schema,omitempty"` // TODO: actual schema
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
