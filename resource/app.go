package resource

import (
	"context"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

type ConversionRequest struct {
	SourceGVK schema.GroupVersionKind
	TargetGVK schema.GroupVersionKind
	Raw       RawObject
}

// RawObject represents the raw bytes of the object and its encoding, optionally with a decoded version of the object,
// which may be any valid resource.Object implementation.
type RawObject struct {
	Raw      []byte       `json:",inline"`
	Object   Object       `json:"-"`
	Encoding KindEncoding `json:"-"`
}

type AppManifest struct {
	// ManifestData must be present if Location.Type == "embedded"
	ManifestData *AppManifestData
	Location     AppManifestLocation
}

type AppManifestLocation struct {
	Type AppManifestLocationType
	// Path is the path to the manifest, based on location.
	// For "filepath", it is the path on disk. For "apiserver", it is the NamespacedName. For "embedded", it is empty.
	Path string
}

type AppManifestLocationType string

const (
	AppManifestLocationFilePath          = AppManifestLocationType("filepath")
	AppManifestLocationAPIServerResource = AppManifestLocationType("apiserver")
	AppManifestLocationEmbedded          = AppManifestLocationType("embedded")
)

type AppManifestData struct {
	Capabilities AppCapabilities
}

type AppCapabilities struct {
	Validator bool
	Mutator   bool
	Converter bool
}

type SubresourceRequest struct {
	ResourceIdentifier FullIdentifier
	SubresourcePath    string
	Method             string
	Headers            http.Header
	Body               []byte
}

type AppConfig struct {
	Kubeconfig  rest.Config
	ExtraConfig map[string]any
}

type AppProvider interface {
	Manifest() AppManifest
	NewApp(AppConfig) (App, error)
}

type Runnable interface {
	Run(<-chan struct{}) error
}

type App interface {
	// ManagedKinds returns a slice of Kinds which are managed by this App.
	ManagedKinds() []Kind
	// Runner returns a Runnable with an app main loop. Any business logic that is not/can not be exposed
	// via other App interfaces should be contained within this method.
	// Runnable MAY be nil, in which case, the app has no main loop business logic.
	Runner() Runnable
}

type ValidatorApp interface {
	ValidatingAdmissionController
}

type MutatorApp interface {
	MutatingAdmissionController
}

type ConverterApp interface {
	Convert(ctx context.Context, req ConversionRequest) (*RawObject, error)
}

type SubresourceApp interface {
	CallSubresource(ctx context.Context, writer http.ResponseWriter, req *SubresourceRequest) error
}

type DatasourceQueryApp interface {
}

type DatasourceStreamApp interface {
}

type FullApp interface {
	App
	ValidatorApp
	MutatorApp
	ConverterApp
	SubresourceApp
}
