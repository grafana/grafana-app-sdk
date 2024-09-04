package app

import (
	"context"
	"net/http"

	"github.com/grafana/grafana-app-sdk/resource"
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
	Raw      []byte                `json:",inline"`
	Object   resource.Object       `json:"-"`
	Encoding resource.KindEncoding `json:"-"`
}

type SubresourceRequest struct {
	ResourceIdentifier resource.FullIdentifier
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
	Manifest() Manifest
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

type AdmissionRequest resource.AdmissionRequest
type MutatingResponse resource.MutatingResponse

type App interface {
	Validate(ctx context.Context, request *resource.AdmissionRequest) error
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
