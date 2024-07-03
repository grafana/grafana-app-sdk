package resource

import (
	"context"
	"net/http"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type SubresourceRequest struct {
	*backend.CallResourceRequest // TODO: should we exclude a plugin-sdk-go ref here to keep that dependency only in the submodule?
	ResourceIdentifier           Identifier
	SubresourcePath              string
	Group                        string
	Version                      string
	Kind                         string
}

// App represents an application that can be run with any wrapper (such as plugin.App).
// On its own, an App controller can be run, but Admission and custom Subresource Routes will not function.
// It exposes functionality in a generic context and wrappers wrap this in logic which translates to the specific protocols of the wrapper.
type App interface {
	ValidatingAdmissionController
	MutatingAdmissionController
	Convert(ctx context.Context, req ConversionRequest) (*RawObject, error)
	// Kind returns the resource.Kind managed by the App if it exists.
	// lookup must contain enough information to look up the kind per App implementation
	// ([group,version,kind] and/or [group,version,plural] should be supported by all App implementations)
	Kind(lookup ResourceLookup) *Kind
	// Init is TODO!
	Init()
	CallSubresource(ctx context.Context, responseWriter http.ResponseWriter, request *SubresourceRequest)
	// Controller returns the Runnable controller for the app.
	// Controller().Run() MAY defer execution until Init() is called on the App if it requires config from Init()
	// TODO: Init() returns the controller instead?
	Controller() Runnable
}

type Runnable interface {
	Run(<-chan struct{}) error
}

// RawObject represents the raw bytes of the object and its encoding, optionally with a decoded version of the object,
// which may be any valid resource.Object implementation.
type RawObject struct {
	Raw      []byte       `json:",inline"`
	Object   Object       `json:"-"`
	Encoding KindEncoding `json:"-"`
}

type ConversionRequest struct {
	SourceGVK schema.GroupVersionKind
	TargetGVK schema.GroupVersionKind
	Raw       RawObject
}

// ResourceLookup contains fields used for looking up a managed resource in an StandardApp.
// An StandardApp should be able to look up a resource by (group, version, kind) or (group, version, plural),
// but may also allow for lookups with other combinations of fields
type ResourceLookup struct {
	Group   string
	Version string
	Kind    string
	Plural  string
}
