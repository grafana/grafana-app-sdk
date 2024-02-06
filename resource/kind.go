package resource

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type KindEncoding string

// KindEncoding constants which reflect the string used for a Content-Type header.
const (
	KindEncodingJSON    KindEncoding = "application/json"
	KindEncodingYAML    KindEncoding = "application/yaml"
	KindEncodingUnknown KindEncoding = ""
)

var (
	_ Kind = &UntypedKind{}
	_ Kind = &TypedKind[*UntypedObject]{}
)

// KindReader describes any type capable of reading one or more Kind object bytes (in wire format) into a specific or generic Object.
// KindReaders may be specific to a particular group/version/kind, or generic across any or all of group, version, and kind.
// See TypedKind and UntypedKind as examples of KindReader implementations.
type KindReader interface {
	// Read consumes the wire-format bytes contained in the io.Reader, and unmarshals them into an instance of the
	// Kind as an Object.
	// It MAY return an error if the provided bytes are not of an expected group, version, and/or kind,
	// and MUST return an error if the provided bytes are not of the proper shape to be unmarshaled as a kind, or cannot be unmarshaled for any other reason.
	Read(in io.Reader, encoding KindEncoding) (Object, error)
	// ReadInto consumes the wire-format bytes contained in the io.Reader, and unmarshals them into the provided Object.
	// It MAY return an error if the provided bytes are not of an expected group, version, and/or kind, or if the provided `into` Object
	// is not of a compatible underlying type, and MUST return an error if the provided bytes are not of the proper shape to be unmarshaled
	// as a kind, or cannot be unmarshaled for any other reason.
	ReadInto(in io.Reader, into Object, encoding KindEncoding) error
}

// KindWriter describes any type capable of writing out an Object into wire-format bytes.
// KindWriters may be specific to a particular Group/Version/Kind, or generic across any or all of Group, Version, and Kind.
// See TypedKind and UntypedKind as examples of KindWriter implementations.
type KindWriter interface {
	// Write consumes an Object-implementation of an instance of the Kind, and writes marshaled wire-format bytes
	// to the provided io.Writer. It MAY return an error if the provided Object is not of the expected underlying type(s),
	// and MUST return an error if the object cannot be marshaled into bytes.
	Write(obj Object, out io.Writer, encoding KindEncoding) error
}

// KindReadWriter is an interface that combines KindReader and KindWriter
type KindReadWriter interface {
	KindReader
	KindWriter
}

// Kind is an interface representing a kubernetes-compatible Kind, which contains information about the Kind's
// Group, Version, Kind, Plural, and Scope, as well as allowing for creation of a zero-value version of the Kind as an
// Object (which may contain default values for fields as decided by the implementation).
// Additionally, Kind contains the logic for reading wire bytes into an Object instance of the Kind,
// or serializing an Object instance of the Kind into wire bytes.
// In this sense, Kind combines schema information and a serializer into one interface used to interact with the kind.
type Kind interface {
	KindReadWriter
	Schema
}

// NewKindGroup returns a new KindGroup with the provided group and version
func NewKindGroup(group, version string) *KindGroup {
	return &KindGroup{
		group:   group,
		version: version,
		kinds:   make([]Kind, 0),
	}
}

// KindGroup is a set of Kind objects which share the same Group and Version.
type KindGroup struct {
	group   string
	version string
	kinds   []Kind
}

// Group returns the group value shared by all kinds in the KindGroup
func (k *KindGroup) Group() string {
	return k.group
}

// Version returns the version value shared by all kinds in the KindGroup
func (k *KindGroup) Version() string {
	return k.version
}

// Kinds returns the list of kinds registered with the group
func (k *KindGroup) Kinds() []Kind {
	return k.kinds
}

// AddKind adds a Kind to the group, if its Group() and Version() values match the KindGroup's.
// Otherwise, it returns an error.
func (k *KindGroup) AddKind(kind Kind) error {
	if kind == nil {
		return fmt.Errorf("kind cannot be nil")
	}
	if k.kinds == nil {
		k.kinds = make([]Kind, 0)
		if k.group == "" {
			k.group = kind.Group()
		}
		if k.version == "" {
			k.version = kind.Version()
		}
	}
	if k.group != kind.Group() || k.version != kind.Version() {
		return fmt.Errorf("kind group is restricted to group/version %s/%s, provided kind is %s/%s", k.group, k.version, kind.Group(), kind.Version())
	}
	k.kinds = append(k.kinds, kind)
	return nil
}

// UntypedKind is a generic implementation of Kind, which will work for any kubernetes kind,
// with Write consuming any Object and producing kubernetes bytes, and Read consuming any valid kubernetes kind bytes
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
	ZeroObject UntypedObject
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
	if u.PluralKind == "" {
		return strings.ToLower(u.GVK.Kind) + "s"
	}
	return u.PluralKind
}
func (u *UntypedKind) ZeroValue() Object {
	return u.ZeroObject.Copy()
}
func (u *UntypedKind) Scope() SchemaScope {
	if u.KindScope == SchemaScope("") {
		return NamespacedScope
	}
	return u.KindScope
}

// Read reads in kubernetes JSON bytes for any kind and returns an *UntypedKind for those bytes
func (u *UntypedKind) Read(in io.Reader, encoding KindEncoding) (Object, error) {
	obj := &UntypedObject{}
	if err := u.ReadInto(in, obj, encoding); err != nil {
		return nil, err
	}
	return obj, nil
}

// ReadInto reads in kubernetes JSON bytes for any kind and attempts to unmarshal them into the provided `into` Object
func (u *UntypedKind) ReadInto(in io.Reader, into Object, encoding KindEncoding) error {
	if in == nil {
		return fmt.Errorf("in io.Reader cannot be nil")
	}
	if into == nil {
		return fmt.Errorf("into Object cannot be nil")
	}
	switch encoding {
	case KindEncodingJSON:
		return json.NewDecoder(in).Decode(&into)
	case KindEncodingYAML:
		return yaml.NewDecoder(in).Decode(&into)
	}
	return fmt.Errorf("cannot unmarshal unknown content encoding '%s'", encoding)
}

// Write takes in any Object and outputs a kubernetes object JSON
func (u *UntypedKind) Write(obj Object, out io.Writer, encoding KindEncoding) error {
	m := make(map[string]any)
	m["apiVersion"], m["kind"] = obj.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
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
	switch encoding {
	case KindEncodingJSON:
		return json.NewEncoder(out).Encode(m)
	case KindEncodingYAML:
		return yaml.NewEncoder(out).Encode(m)
	default:
		return fmt.Errorf("cannot marshal unknown content encoding '%s'", encoding)
	}
}

// NewTypedKind is a convenience function for creating a new instance of TypedKind.
// It defaults all fields not required as arguments.
func NewTypedKind[T Object](group, version, kind string, zeroObject T) *TypedKind[T] {
	return &TypedKind[T]{
		GVK: schema.GroupVersionKind{
			Group:   group,
			Version: version,
			Kind:    kind,
		},
		PluralKind: fmt.Sprintf("%ss", strings.ToLower(kind)),
		KindScope:  NamespacedScope,
		ZeroObject: zeroObject,
	}
}

// TypedKind is an implementation of Kind which uses a provided type param for the Object type to read into/read from.
// TypedKind will allow for reading bytes of any kind and apiVersion, but will attempt to parse them into an Object of
// type T.
type TypedKind[T Object] struct {
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
	if t.PluralKind == "" {
		return strings.ToLower(t.GVK.Kind) + "s"
	}
	return t.PluralKind
}
func (t *TypedKind[T]) ZeroValue() Object {
	var o Object = t.ZeroObject
	if o == nil {
		return &UntypedObject{}
	}
	fmt.Println(o)
	return o.Copy()
}
func (t *TypedKind[T]) Scope() SchemaScope {
	if t.KindScope == "" {
		return NamespacedScope
	}
	return t.KindScope
}

func (t *TypedKind[T]) Read(in io.Reader, encoding KindEncoding) (Object, error) {
	into := new(T) // TODO: use ZeroValue instead?
	// TODO: better way of unmarshaling into the object...
	bytes, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	switch encoding {
	case KindEncodingJSON:
		err = json.Unmarshal(bytes, into)
		if err != nil {
			return nil, err
		}
	case KindEncodingYAML:
		err = yaml.Unmarshal(bytes, into)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("cannot unmarshal unknown content encoding '%s'", encoding)
	}
	return *into, nil
}

func (t *TypedKind[T]) ReadInto(in io.Reader, into Object, encoding KindEncoding) error {
	cast, ok := into.(T)
	if !ok {
		return fmt.Errorf("into must be of type parameter T (provided type %#v)", into)
	}
	switch encoding {
	case KindEncodingJSON:
		return json.NewDecoder(in).Decode(cast)
	case KindEncodingYAML:
		return yaml.NewDecoder(in).Decode(cast)
	}
	return fmt.Errorf("cannot unmarshal unknown content encoding '%s'", encoding)
}

// Write takes in any Object and outputs a kubernetes object JSON.
// This is an identical call to UntypedKind.Write, as they both do not examine the underlying typing of the obj
func (t *TypedKind[T]) Write(obj Object, out io.Writer, encoding KindEncoding) error {
	m := make(map[string]any)
	m["apiVersion"], m["kind"] = obj.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
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
	switch encoding {
	case KindEncodingJSON:
		return json.NewEncoder(out).Encode(m)
	case KindEncodingYAML:
		return yaml.NewEncoder(out).Encode(m)
	default:
		return fmt.Errorf("cannot marshal unknown content encoding '%s'", encoding)
	}
}

type TypedList[T Object] struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []T `json:"items"`
}

func (t *TypedList[T]) DeepCopyObject() runtime.Object {
	return t.Copy()
}

func (t *TypedList[T]) Copy() ListObject {
	cpy := &TypedList[T]{
		TypeMeta: t.TypeMeta,
		Items:    make([]T, len(t.Items)),
	}
	t.ListMeta.DeepCopyInto(&cpy.ListMeta)
	for i := 0; i < len(t.Items); i++ {
		cpy.Items[i] = t.Items[i].Copy().(T)
	}
	return cpy
}

func (t *TypedList[T]) GetItems() []Object {
	// TODO: this should be a pointer copy without too much new allocation, but let's double-check
	tmp := make([]Object, len(t.Items))
	for i := 0; i < len(t.Items); i++ {
		tmp[i] = t.Items[i]
	}
	return tmp
}

func (t *TypedList[T]) SetItems(items []Object) {
	t.Items = make([]T, len(items))
	for i := 0; i < len(t.Items); i++ {
		_, ok := items[i].(T)
		if !ok {
			// HMMMMM
		}
		t.Items[i] = items[i].(T)
	}
}
