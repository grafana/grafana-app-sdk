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

// ManifestData is the data in a Manifest, representing the Kinds and Admission of an App.
// NOTE: ManifestData is still experimental and subject to change
type ManifestData struct {
	AppName string `json:"appName" yaml:"appName"`
	Group   string `json:"group" yaml:"group"`
	Kinds   []ManifestKind
}

// ManifestKind is the manifest for a particular kind, including its Kind, Scope, and Versions
type ManifestKind struct {
	Kind       string                `json:"kind" yaml:"kind"`
	Scope      string                `json:"scope" yaml:"scope"`
	Versions   []ManifestKindVersion `json:"versions" yaml:"versions"`
	Conversion bool                  `json:"conversion" yaml:"conversion"`
}

type ManifestKindVersion struct {
	Name      string                 `yaml:"name" json:"name"`
	Admission *AdmissionCapabilities `json:"admission,omitempty" yaml:"admission,omitempty"`
	Schema    any                    `json:"schema" yaml:"schema"` // TODO: actual schema
}

type AdmissionCapabilities struct {
	Validation *ValidationCapability `json:"validation,omitempty" yaml:"validation,omitempty"`
	Mutation   *MutationCapability   `json:"mutation,omitempty" yaml:"mutation,omitempty"`
}

func (c AdmissionCapabilities) SupportsAnyValidation() bool {
	if c.Validation == nil {
		return false
	}
	return len(c.Validation.Operations) > 0
}

func (c AdmissionCapabilities) SupportsAnyMutation() bool {
	if c.Mutation == nil {
		return false
	}
	return len(c.Mutation.Operations) > 0
}

type ValidationCapability struct {
	Operations []string `json:"operations,omitempty" yaml:"operations,omitempty"`
}

type MutationCapability struct {
	Operations []string `json:"operations,omitempty" yaml:"operations,omitempty"`
}
