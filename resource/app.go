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

func NewEmbeddedAppManifest(manifestData AppManifestData) AppManifest {
	return AppManifest{
		Location: AppManifestLocation{
			Type: AppManifestLocationEmbedded,
		},
		ManifestData: &manifestData,
	}
}

func NewOnDiskAppManifest(path string) AppManifest {
	return AppManifest{
		Location: AppManifestLocation{
			Type: AppManifestLocationFilePath,
			Path: path,
		},
	}
}

func NewAAPIServerAppManifest(resourceName string) AppManifest {
	return AppManifest{
		Location: AppManifestLocation{
			Type: AppManifestLocationAPIServerResource,
			Path: resourceName,
		},
	}
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

// AppProvider represents a type which can provide an app manifest, and create a new App when given a configuration.
// It should be used by runners to determine an app's capabilities and create an instance of the app to run.
type AppProvider interface {
	Manifest() AppManifest
	NewApp(AppConfig) (App, error)
}

// Runnable represents a type which can be run until it errors or the provided channel is stopped (or receives a message)
type Runnable interface {
	// Run runs the process and blocks until one of the following conditions are met:
	// * An unrecoverable error occurs, in which case it returns the error
	// * The provided channel is closed, in which case processing should stop and the method should return
	// * The provided channel is sent a message, in which case processing should stop and the method should return
	// * The process completes and does not need to run again
	Run(<-chan struct{}) error
}

type App interface {
	// ManagedKinds returns a slice of Kinds which are managed by this App.
	// If there are multiple versions of a Kind, each one SHOULD be returned by this method,
	// as app runners may depend on having access to all kinds.
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
