package resource

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

// WireFormat enumerates values for possible message wire formats.
// Constants with these values are in this package with a `WireFormat` prefix.
type WireFormat int

const (
	// WireFormatUnknown is an unknown message wire format.
	WireFormatUnknown WireFormat = iota
	// WireFormatJSON is a JSON message wire format, which should be handle-able by the `json` package.
	// (messages which _contain_ JSON, but are not parsable by the go json package should not be
	// considered to be of the JSON wire format).
	WireFormatJSON
)

// UnmarshalConfig is the config used for unmarshaling Objects.
// It consists of fields that are descriptive of the underlying content, based on knowledge the caller has.
type UnmarshalConfig struct {
	// WireFormat is the wire format of the provided payload
	WireFormat WireFormat
	// VersionHint is what the client thinks the version is (if non-empty)
	VersionHint string
}

// ObjectBytes is the collection of different Object components as raw bytes.
// It is used for unmarshaling an Object, and can be used for Marshaling as well.
// Client-implementations are required to process their own storage representation into
// a uniform representation in ObjectBytes.
type ObjectBytes struct {
	// Spec contains the marshaled SpecObject. It should be unmarshalable directly into the Object-implementation's
	// Spec object using an unmarshaler of the appropriate WireFormat type
	Spec []byte
	// Metadata includes object-specific metadata, and may include CommonMetadata depending on implementation.
	// Clients must call SetCommonMetadata on the object after an Unmarshal if CommonMetadata is not provided in the bytes.
	Metadata []byte
	// Subresources contains a map of all subresources that are both part of the underlying Object implementation,
	// AND are supported by the Client implementation. Each entry should be unmarshalable directly into the
	// Object-implementation's relevant subresource using an unmarshaler of the appropriate WireFormat type
	Subresources map[string][]byte
}

// Object represents a concrete schema object. This is an abstract representation,
// and most concrete implementations should come from generated code.
// In abstract, an Object consists of a single `Spec` object, optional subresources as specified by a schema,
// and metadata, divided into Static and Object Metadata.
//
// Relationship-wise, an Object can be viewed as an instance of a Schema, though an Object has no
// concrete ties to a Schema aside from a Schema generating it with Schema.ZeroValue() and
// StaticMetadata Group,Version,Kind commonalities. Thus, an Object does not _need_ a correlated Schema,
// but functions best when related to one.
//
// TODO: comments on how to generate concrete Object implementations with the SDK's codegen
type Object interface {
	// CommonMetadata returns the Object's CommonMetadata
	CommonMetadata() CommonMetadata

	// SetCommonMetadata overwrites the CommonMetadata of the object.
	// Implementations should always overwrite, rather than attempt merges of the metadata.
	// Callers wishing to merge should get current metadata with CommonMetadata() and set specific values.
	SetCommonMetadata(metadata CommonMetadata)

	// StaticMetadata returns the Object's StaticMetadata
	StaticMetadata() StaticMetadata

	// SetStaticMetadata overwrites the Object's StaticMetadata with the provided StaticMetadata.
	// Implementations should always overwrite, rather than attempt merges of the metadata.
	// Callers wishing to merge should get current metadata with StaticMetadata() and set specific values.
	// Note that StaticMetadata is only mutable in an object create context.
	SetStaticMetadata(metadata StaticMetadata)

	// CustomMetadata returns metadata unique to this Object's kind, as opposed to Common and Static metadata,
	// which are the same across all kinds. An object may have no kind-specific CustomMetadata.
	// CustomMetadata can only be read from this interface, for use with resource.Client implementations,
	// those who wish to set CustomMetadata should use the interface's underlying type.
	CustomMetadata() CustomMetadata

	// SpecObject returns the actual "schema" object, which holds the main body of data
	SpecObject() any

	// Subresources returns a map of subresource name(s) to the object value for that subresource.
	// Spec is not considered a subresource, and should only be returned by SpecObject
	Subresources() map[string]any

	// Unmarshal unmarshals the spec object and all provided subresources according to the provided WireFormat.
	// It returns an error if any part of the provided bytes cannot be unmarshaled.
	Unmarshal(bytes ObjectBytes, config UnmarshalConfig) error

	// Copy returns a full copy of the Object with all its data
	Copy() Object
}

// CustomMetadata is an interface describing a resource.Object's kind-specific metadata
type CustomMetadata interface {
	// MapFields converts the custom metadata's fields into a map of field key to value.
	// This is used so Clients don't need to engage in reflection for marshaling metadata,
	// as various implementations may not store kind-specific metadata the same way.
	MapFields() map[string]any
}

// ListObject represents a List of Object-implementing objects with list metadata.
// The simplest way to use it is to use the implementation returned by a Client's List call.
type ListObject interface {
	ListMetadata() ListMetadata
	SetListMetadata(ListMetadata)
	ListItems() []Object
	SetItems([]Object)
}

// StaticMetadata consists of all non-mutable metadata for an object.
// It is set in the initial Create call for an Object, then will always remain the same.
type StaticMetadata struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// Identifier creates an Identifier struct from the StaticMetadata
func (s StaticMetadata) Identifier() Identifier {
	return Identifier{
		Namespace: s.Namespace,
		Name:      s.Name,
	}
}

// FullIdentifier returns a FullIdentifier struct from the StaticMetadata.
// Plural cannot be inferred so is left empty.
func (s StaticMetadata) FullIdentifier() FullIdentifier {
	return FullIdentifier{
		Group:     s.Group,
		Version:   s.Version,
		Kind:      s.Kind,
		Namespace: s.Namespace,
		Name:      s.Name,
	}
}

// CommonMetadata is the generic common metadata for a resource.Object
// TODO: should this be in kindsys, based on the CUE type (once kindsys changes are in effect)?
type CommonMetadata struct {
	// UID is the unique ID of the object. This can be used to uniquely identify objects,
	// but is not guaranteed to be able to be used for lookups.
	UID string `json:"uid"`
	// ResourceVersion is a version string used to identify any and all changes to the object.
	// Any time the object changes in storage, the ResourceVersion will be changed.
	// This can be used to block updates if a change has been made to the object between when the object was
	// retrieved, and when the update was applied.
	ResourceVersion string `json:"resourceVersion"`
	// Labels are string key/value pairs attached to the object. They can be used for filtering,
	// or as additional metadata
	Labels map[string]string `json:"labels"`
	// CreationTimestamp indicates when the resource has been created.
	CreationTimestamp time.Time `json:"creationTimestamp"`
	// DeletionTimestamp indicates that the resource is pending deletion as of the provided time if non-nil.
	// Depending on implementation, this field may always be nil, or it may be a "tombstone" indicator.
	// It may also indicate that the system is waiting on some task to finish before the object is fully removed.
	DeletionTimestamp *time.Time `json:"deletionTimestamp"`
	// Finalizers are a list of identifiers of interested parties for delete events for this resource.
	// Once a resource with finalizers has been deleted, the object should remain in the store,
	// DeletionTimestamp is set to the time of the "delete," and the resource will continue to exist
	// until the finalizers list is cleared.
	Finalizers []string `json:"finalizers"`
	// UpdateTimestamp is the timestamp of the last update to the resource
	UpdateTimestamp time.Time `json:"updateTimestamp"`
	// CreatedBy is a string which indicates the user or process which created the resource.
	// Implementations may choose what this indicator should be.
	CreatedBy string `json:"createdBy"`
	// UpdatedBy is a string which indicates the user or process which last updated the resource.
	// Implementations may choose what this indicator should be.
	UpdatedBy string `json:"updatedBy"`
	// TODO: additional fields?

	// ExtraFields stores implementation-specific metadata.
	// Not all Client implementations are required to honor all ExtraFields keys.
	// Generally, this field should be shied away from unless you know the specific
	// Client implementation you're working with and wish to track or mutate extra information.
	ExtraFields map[string]any `json:"extraFields"`
}

// ListMetadata is metadata for a list of objects. This is typically only used in responses from the storage layer.
type ListMetadata struct {
	ResourceVersion string `json:"resourceVersion"`

	Continue string `json:"continue"`

	RemainingItemCount *int64 `json:"remainingItemCount"`

	// ExtraFields stores implementation-specific metadata.
	// Not all Client implementations are required to honor all ExtraFields keys.
	// Generally, this field should be shied away from unless you know the specific
	// Client implementation you're working with and wish to track or mutate extra information.
	ExtraFields map[string]any `json:"extraFields"`
}

// SimpleCustomMetadata is an implementation of CustomMetadata
type SimpleCustomMetadata map[string]any

// MapFields returns a map of string->value for all CustomMetadata fields
func (s SimpleCustomMetadata) MapFields() map[string]any {
	return s
}

// CopyObject is an implementation of the receiver method `Copy()` required for implementing Object.
// It should be used in your own runtime.Object implementations if you do not wish to implement custom behavior.
// Example:
//
//	func (c *CustomObject) Copy() resource.Object {
//	    return resource.CopyObject(c)
//	}
func CopyObject(in any) Object {
	val := reflect.ValueOf(in).Elem()

	cpy := reflect.New(val.Type())
	cpy.Elem().Set(val)

	// Using the <obj>, <ok> for the type conversion ensures that it doesn't panic if it can't be converted
	if obj, ok := cpy.Interface().(Object); ok {
		return obj
	}

	// TODO: better return than nil?
	return nil
}

// BasicMetadataObject is a composable base struct to attach Metadata, and its associated functions, to another struct.
// BasicMetadataObject provides a Metadata field composed of StaticMetadata and ObjectMetadata, as well as the
// ObjectMetadata(),SetObjectMetadata(), StaticMetadata(), and SetStaticMetadata() receiver functions.
type BasicMetadataObject struct {
	StaticMeta StaticMetadata       `json:"staticMetadata"`
	CommonMeta CommonMetadata       `json:"commonMetadata"`
	CustomMeta SimpleCustomMetadata `json:"customMetadata"`
}

// CommonMetadata returns the object's CommonMetadata
func (b *BasicMetadataObject) CommonMetadata() CommonMetadata {
	return b.CommonMeta
}

// SetCommonMetadata overwrites the ObjectMetadata.Common() supplied by BasicMetadataObject.ObjectMetadata()
func (b *BasicMetadataObject) SetCommonMetadata(m CommonMetadata) {
	b.CommonMeta = m
}

// StaticMetadata returns the object's StaticMetadata
func (b *BasicMetadataObject) StaticMetadata() StaticMetadata {
	return b.StaticMeta
}

// SetStaticMetadata overwrites the StaticMetadata supplied by BasicMetadataObject.StaticMetadata()
func (b *BasicMetadataObject) SetStaticMetadata(m StaticMetadata) {
	b.StaticMeta = m
}

// CustomMetadata returns the object's CustomMetadata
func (b *BasicMetadataObject) CustomMetadata() CustomMetadata {
	return b.CustomMeta
}

// SimpleObject is a very simple implementation of the Object interface.
// Its subresources are provided by the untyped map[string]any SubresourceMap.
// If you use this as a composable piece of another object,
// ensure that you shadow the Copy() method for correct behavior.
type SimpleObject[SpecType any] struct {
	BasicMetadataObject
	Spec           SpecType       `json:"spec"`
	SubresourceMap map[string]any `json:"subresources"`
}

// SpecObject returns the
func (t *SimpleObject[T]) SpecObject() any {
	return t.Spec
}

// Subresources returns the SubresourceMap field
func (t *SimpleObject[T]) Subresources() map[string]any {
	return t.SubresourceMap
}

// Copy provides a copy of this SimpleObject. If SimpleObject is used as a component in another type,
// this method must be shadowed by the parent type for proper behavior.
// TODO: not sure whether this should be implemented in SimpleObject at all, but I can see use-cases where you use
// SimpleObject directly, rather than as a component
func (t *SimpleObject[T]) Copy() Object {
	return CopyObject(t)
}

// Unmarshal takes in bytes of the spec and subresources, unmarshals the spec into t.Spec,
// and puts all subresource bytes into t.SubresourceMap without unmarshaling.
func (t *SimpleObject[T]) Unmarshal(bytes ObjectBytes, c UnmarshalConfig) error {
	if t.SubresourceMap == nil {
		t.SubresourceMap = make(map[string]any)
	}
	switch c.WireFormat {
	case WireFormatJSON:
		err := json.Unmarshal(bytes.Spec, &t.Spec)
		if err != nil {
			return err
		}
		for k, v := range bytes.Subresources {
			t.SubresourceMap[k] = json.RawMessage(v)
		}
		err = json.Unmarshal(bytes.Metadata, &t.CommonMeta)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("cannot handle specified wire format")
	}
}

// SimpleList is a simple implementation of ListObject.
type SimpleList[ItemType Object] struct {
	ListMeta ListMetadata `json:"metadata"`
	Items    []ItemType   `json:"items"`
}

// ListItems returns the ListItems, re-cast as a slice of Object elements
func (l *SimpleList[T]) ListItems() []Object {
	items := make([]Object, len(l.Items))
	for i := 0; i < len(l.Items); i++ {
		items[i] = l.Items[i]
	}
	return items
}

// SetItems overwrites ListItems with the contents of items, provided that each element in items is of type T
func (l *SimpleList[T]) SetItems(items []Object) {
	newItems := make([]T, 0)
	for _, i := range items {
		if cast, ok := i.(T); ok {
			newItems = append(newItems, cast)
		}
	}
	l.Items = newItems
}

// ListMetadata returns ListMeta
func (l *SimpleList[T]) ListMetadata() ListMetadata {
	return l.ListMeta
}

// SetListMetadata overwrites the contents of ListMeta with metadata
func (l *SimpleList[T]) SetListMetadata(metadata ListMetadata) {
	l.ListMeta = metadata
}
