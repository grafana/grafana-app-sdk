package resource

import (
	"encoding/json"
	"fmt"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	_ Kind    = &UntypedKind{}
	_ Object2 = &UntypedObject{}
)

// Object2 implements kubernetes' runtime.Object and meta/v1.Object, as well as some additional methods useful for the app-sdk
type Object2 interface {
	runtime.Object
	metav1.Object
	GetSpec() any
	GetSubresources() map[string]any
	GetStaticMetadata() StaticMetadata
	SetStaticMetadata(metadata StaticMetadata)
}

// MultiVersionKind is a collection of Kinds which have the same Group and Kind, but different versions.
// This may allow us to include conversion as a component of the interface.
// TODO: not sure if necessary
type MultiVersionKind interface {
	Group() string
	Kind() string
	Versions() []Kind
}

// Group is a collection of Kinds which share a Group and Version, allowing them to be put under the same
// routing in an API server.
// TODO: not sure if necessary
type Group interface {
	Group() string
	Version() string
	Kinds() []Kind
}

// Kind is an interface representing a kubernetes-compatible Kind, which contains information about the Kind's
// Group, Version, Kind, Plural, and Scope, as well as allowing for creation of a zero-value version of the Kind as an
// Object (which may contain default values for fields as decided by the implementation).
// Additionally, Kind contains the logic for reading wire bytes into an Object instance of the Kind,
// or serializing an Object instance of the Kind into wire bytes.
// In this sense, Kind combines schema information and a serializer into one interface used to interact with the kind.
type Kind interface {
	//Schema <- can't compose because we need Object2
	// TODO: just update Object instead of using Object2?

	// Group returns the Schema group
	Group() string
	// Version returns the Schema version
	Version() string
	// Kind returns the Schema kind
	Kind() string
	// Plural returns the plural name of the Schema kind
	Plural() string
	// ZeroValue returns the "zero-value", "default", or "empty" version of an Object of this Schema
	ZeroValue() Object2
	// Scope returns the scope of the schema object
	Scope() SchemaScope
	// Read consumes the wire-format bytes contained in the io.Reader, and unmarshals them into an instance of the
	// Kind as an Object2.
	// It MAY return an error if the provided bytes are not of the Kind, and MUST return an error if the provided bytes
	// are not of the proper shape to be unmarshaled as a kind, or cannot be unmarshaled for any other reason.
	// TODO: should this _also_ consume a wire format for identifying the type of bytes on the wire, or always assume JSON?
	Read(in io.Reader) (Object2, error)
	// Write consumes an Object2-implementation of an instance of the Kind, and writes marshaled wire-format bytes
	// to the provided io.Writer. It MAY return an error if the provided Object2 is not of the expected type for the Kind,
	// and MUST return an error if the object cannot be marshaled into bytes.
	// TODO: should this _also_ consume a wire format for identifying the type of bytes on the wire, or always assume JSON?
	Write(obj Object2, out io.Writer) error
}

// UntypedKind is a generic implementation of Kind, which will work for any kubernetes kind,
// with Write consuming any Object2 and producing kubernetes bytes, and Read consuming any valid kubernetes kind bytes
// and producing an *UntypedKind. The values for all getter methods are set via the corresponding struct fields.
type UntypedKind struct {
	// GVK is the group, version, and kind of the UntypedKind, returned by Group(), Version(), and Kind() respectively
	GVK schema.GroupVersionKind
	// PluralKind is the plural name of the Kind, returned via Plural()
	PluralKind string
	// KindScope is the scope of the Kind, returned by Scope()
	KindScope SchemaScope
	// ZeroObject is the *UntypedObject returned by ZeroValue(). This only needs to be non-nil if you do not want to use
	// an empty *UntypedObject as the ZeroValue() return.
	ZeroObject *UntypedObject
}

func (u *UntypedKind) Group() string {
	return u.GVK.Group
}
func (u *UntypedKind) Version() string {
	return u.GVK.Version
}
func (u *UntypedKind) Kind() string {
	return u.GVK.Kind
}
func (u *UntypedKind) Plural() string {
	return u.PluralKind
}
func (u *UntypedKind) ZeroValue() Object2 {
	if u.ZeroObject != nil {
		return u.ZeroObject
	}
	return &UntypedObject{}
}
func (u *UntypedKind) Scope() SchemaScope {
	return u.KindScope
}

// Read reads in kubernetes JSON bytes for any kind and returns an *UntypedKind for those bytes
func (u *UntypedKind) Read(in io.Reader) (Object2, error) {
	bytes, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	obj := &UntypedObject{}
	err = json.Unmarshal(bytes, obj)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// Write takes in any Object2 and outputs a kubernetes object JSON
func (u *UntypedKind) Write(obj Object2, out io.Writer) error {
	m := make(map[string]any)
	m["kind"], m["apiVersion"] = obj.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
	m["metadata"] = metav1.ObjectMeta{
		Name:                       obj.GetName(),
		GenerateName:               obj.GetGenerateName(),
		Namespace:                  obj.GetNamespace(),
		SelfLink:                   obj.GetSelfLink(),
		UID:                        obj.GetUID(),
		ResourceVersion:            obj.GetResourceVersion(),
		Generation:                 obj.GetGeneration(),
		CreationTimestamp:          obj.GetCreationTimestamp(),
		DeletionTimestamp:          obj.GetDeletionTimestamp(),
		DeletionGracePeriodSeconds: obj.GetDeletionGracePeriodSeconds(),
		Labels:                     obj.GetLabels(),
		Annotations:                obj.GetAnnotations(),
		OwnerReferences:            obj.GetOwnerReferences(),
		Finalizers:                 obj.GetFinalizers(),
		ManagedFields:              obj.GetManagedFields(),
	}
	m["spec"] = obj.GetSpec()
	for k, v := range obj.GetSubresources() {
		m[k] = v
	}
	return json.NewEncoder(out).Encode(m)
}

type TypedKind[T Object2] struct {
	GVK        schema.GroupVersionKind
	PluralKind string
	KindScope  SchemaScope
	ZeroObject T
}

func (t *TypedKind[T]) Group() string {
	return t.GVK.Group
}
func (t *TypedKind[T]) Version() string {
	return t.GVK.Version
}
func (t *TypedKind[T]) Kind() string {
	return t.GVK.Kind
}
func (t *TypedKind[T]) Plural() string {
	return t.PluralKind
}
func (t *TypedKind[T]) ZeroValue() Object2 {
	if t.ZeroObject != nil {
		return t.ZeroObject
	}
	return &UntypedObject{}
}
func (t *TypedKind[T]) Scope() SchemaScope {
	return t.KindScope
}

func (t *TypedKind[T]) Read(in io.Reader) (Object2, error) {
	into := new(T) // TODO: use ZeroValue instead?
	// TODO: better way of unmarshaling into the object...
	bytes, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	obj := &UntypedObject{}
	err = json.Unmarshal(bytes, obj)
	if err != nil {
		return nil, err
	}
	return *into, nil
}

// Write takes in any Object2 and outputs a kubernetes object JSON.
// This is an identical call to UntypedKind.Write, as they both do not examine the underlying typing of the obj
func (t *TypedKind[T]) Write(obj Object2, out io.Writer) error {
	m := make(map[string]any)
	m["kind"], m["apiVersion"] = obj.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
	m["metadata"] = metav1.ObjectMeta{
		Name:                       obj.GetName(),
		GenerateName:               obj.GetGenerateName(),
		Namespace:                  obj.GetNamespace(),
		SelfLink:                   obj.GetSelfLink(),
		UID:                        obj.GetUID(),
		ResourceVersion:            obj.GetResourceVersion(),
		Generation:                 obj.GetGeneration(),
		CreationTimestamp:          obj.GetCreationTimestamp(),
		DeletionTimestamp:          obj.GetDeletionTimestamp(),
		DeletionGracePeriodSeconds: obj.GetDeletionGracePeriodSeconds(),
		Labels:                     obj.GetLabels(),
		Annotations:                obj.GetAnnotations(),
		OwnerReferences:            obj.GetOwnerReferences(),
		Finalizers:                 obj.GetFinalizers(),
		ManagedFields:              obj.GetManagedFields(),
	}
	m["spec"] = obj.GetSpec()
	for k, v := range obj.GetSubresources() {
		m[k] = v
	}
	return json.NewEncoder(out).Encode(m)
}

// UntypedObject implements Object2 and represents a generic implementation of an instance of any kubernetes Kind.
type UntypedObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta `json:"metadata"`
	Spec              map[string]any `json:"spec"`
	Subresources      map[string]json.RawMessage
}

func (a *UntypedObject) GetSpec() any {
	return a.Spec
}

func (a *UntypedObject) GetSubresources() map[string]any {
	// TODO
	return nil
}

func (u *UntypedObject) DeepCopyObject() runtime.Object {
	// TODO
	return &UntypedObject{}
}

func (u *UntypedObject) UnmarshalJSON(data []byte) error {
	m := make(map[string]json.RawMessage)
	err := json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(m["apiVersion"], &u.TypeMeta.APIVersion); err != nil {
		return fmt.Errorf("error reading apiVersion: %w", err)
	}
	if err = json.Unmarshal(m["kind"], &u.TypeMeta.Kind); err != nil {
		return fmt.Errorf("error reading kind: %w", err)
	}
	if err = json.Unmarshal(m["metadata"], &u.ObjectMeta); err != nil {
		return fmt.Errorf("error reading metadata: %w", err)
	}
	for k, v := range m {
		if k == "apiVersion" || k == "kind" || k == "metadata" {
			continue
		}
		if k == "spec" {
			u.Spec = make(map[string]any)
			if err = json.Unmarshal(v, &u.Spec); err != nil {
				return err
			}
			continue
		}
		if u.Subresources == nil {
			u.Subresources = make(map[string]json.RawMessage)
		}
		u.Subresources[k] = v
	}
	return nil
}

func (u *UntypedObject) MarshalJSON() ([]byte, error) {
	m := make(map[string]any)
	m["kind"] = u.Kind
	m["apiVersion"] = u.APIVersion
	m["metadata"] = u.ObjectMeta
	m["spec"] = u.Spec
	for k, v := range u.Subresources {
		m[k] = v
	}
	return json.Marshal(m)
}

func (u *UntypedObject) GetStaticMetadata() StaticMetadata {
	return StaticMetadata{
		Name:      u.ObjectMeta.Name,
		Namespace: u.ObjectMeta.Namespace,
		Group:     u.GroupVersionKind().Group,
		Version:   u.GroupVersionKind().Version,
		Kind:      u.GroupVersionKind().Kind,
	}
}

func (u *UntypedObject) SetStaticMetadata(metadata StaticMetadata) {
	u.Name = metadata.Name
	u.Namespace = metadata.Namespace
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   metadata.Group,
		Version: metadata.Version,
		Kind:    metadata.Kind,
	})
}
