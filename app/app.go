package app

import (
	"context"
	"errors"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/resource"
)

var (
	// ErrNotImplemented is an error that indicates that an App method is not implemented by the App implementation,
	// or the App method is not implemented for the provided Kind.
	// Typically, the App's ManifestData should indicate this as well.
	ErrNotImplemented = errors.New("not implemented")
)

// ConversionRequest is a request to convert a Kind from one version to another
type ConversionRequest struct {
	SourceGVK schema.GroupVersionKind
	TargetGVK schema.GroupVersionKind
	Raw       RawObject
}

// RawObject represents the raw bytes of the object and its encoding, optionally with a decoded version of the object,
// which may be any valid resource.Object implementation.
type RawObject struct {
	Raw      []byte                `json:",inline"`
	Object   resource.Object       `json:"-"`
	Encoding resource.KindEncoding `json:"-"`
}

// SubresourceRequest is a request to a custom subresource
type SubresourceRequest struct {
	ResourceIdentifier resource.FullIdentifier
	SubresourcePath    string
	Method             string
	Headers            http.Header
	Body               []byte
}

// Config is the app configuration used in a Provider for instantiating a new App.
// It contains kubernetes configuration for communicating with an API server, the App's ManifestData as fetched
// by the runner, and additional arbitrary configuration details that may be app-specific.
type Config struct {
	// KubeConfig is a kubernetes rest.Config used to communicate with the API server where the App's Kinds are stored.
	KubeConfig rest.Config
	// ManifestData is the fetched ManifestData the runner is using for determining app kinds and capabilities.
	ManifestData ManifestData
	// ExtraConfig is additional configuration information, which may be app or runner specific
	ExtraConfig map[string]any
}

// Provider represents a type which can provide an app manifest, and create a new App when given a configuration.
// It should be used by runners to determine an app's capabilities and create an instance of the app to run.
type Provider interface {
	// Manifest returns a Manifest, which may contain ManifestData or may point to a location where ManifestData can be fetched from.
	// The runner should use the ManifestData to determine app capabilities.
	Manifest() Manifest
	// NewApp creates a new App instance using the provided config, or returns an error if an App cannot be instantiated.
	NewApp(Config) (App, error)
}

// Runnable represents a type which can be run until it errors or the provided channel is stopped (or receives a message)
type Runnable interface {
	// Run runs the process and blocks until one of the following conditions are met:
	// * An unrecoverable error occurs, in which case it returns the error
	// * The provided context completes
	// * The process completes and does not need to run again
	Run(context.Context) error
}

type AdmissionRequest resource.AdmissionRequest
type MutatingResponse resource.MutatingResponse

// App represents an app platform application logical structure.
// An App is typically run with a wrapper, such as simple.NewStandaloneOperator,
// which will present a runtime layer (such as kubernetes webhooks in the case of an operator),
// and translate those into calls to the App. The wrapper is typically also responsible for lifecycle management
// and running the Runnable provided by Runner().
// Pre-built implementations of App exist in the simple package, but any type which implements App
// should be capable of being run by an app wrapper.
type App interface {
	// Validate validates the incoming request, and returns an error if validation fails
	Validate(ctx context.Context, request *resource.AdmissionRequest) error
	// Mutate runs mutation on the incoming request, responding with a MutatingResponse on success, or an error on failure
	Mutate(ctx context.Context, request *resource.AdmissionRequest) (*resource.MutatingResponse, error)
	// Convert converts the object based on the ConversionRequest, returning a RawObject which MUST contain
	// the converted bytes and encoding (Raw and Encoding respectively), and MAY contain the Object representation of those bytes.
	// It returns an error if the conversion fails, or if the functionality is not supported by the app.
	Convert(ctx context.Context, req ConversionRequest) (*RawObject, error)
	// CallSubresource handles the subresource call, and writes the response to the http.ResponseWriter.
	// If a non-http-error response is encountered, an error should be returned.
	// It returns an error if the functionality is not supported by the app.
	CallSubresource(ctx context.Context, writer http.ResponseWriter, req *SubresourceRequest) error
	// ManagedKinds returns a slice of Kinds which are managed by this App.
	// If there are multiple versions of a Kind, each one SHOULD be returned by this method,
	// as app runners may depend on having access to all kinds.
	ManagedKinds() []resource.Kind
	// Runner returns a Runnable with an app main loop. Any business logic that is not/can not be exposed
	// via other App interfaces should be contained within this method.
	// Runnable MAY be nil, in which case, the app has no main loop business logic.
	Runner() Runnable
}
