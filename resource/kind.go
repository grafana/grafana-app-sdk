package resource

import (
	"encoding/json"
	"fmt"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type KindEncoding string

// KindEncoding constants which reflect the string used for a Content-Type header.
const (
	KindEncodingJSON    KindEncoding = "application/json"
	KindEncodingYAML    KindEncoding = "application/yaml"
	KindEncodingUnknown KindEncoding = ""
)

var (
	_ Schema = &Kind{}
)

// Codec is an interface which describes any object which can read and write Object implementations to/from bytes.
// A codec is often specific to an encoding of the bytes in the reader/writer, and may also be specific to
// Object implementations.
type Codec interface {
	Read(in io.Reader, into Object) error
	Write(out io.Writer, obj Object) error
}

// Kind is a struct which encapsulates Schema information and Codecs for reading/writing Objects which are instances
// of the contained Schema. It implements Schema using the Schema field.
type Kind struct {
	Schema
	Codecs map[KindEncoding]Codec
}

// Codec is a nil-safe way of accessing the Codecs map in the Kind.
// It will return nil if the map key does not exist, or the key is explicitly set to nil.
func (k *Kind) Codec(encoding KindEncoding) Codec {
	if k.Codecs == nil {
		return nil
	}
	return k.Codecs[encoding]
}

// Read is a convenience wrapper for getting the Codec for a particular KindEncoding and reading into Schema.ZeroObject()
func (k *Kind) Read(in io.Reader, encoding KindEncoding) (Object, error) {
	codec := k.Codec(encoding)
	if codec == nil {
		return nil, fmt.Errorf("no codec for encoding '%s'", encoding)
	}
	into := k.ZeroValue()
	err := codec.Read(in, into)
	if err != nil {
		return nil, err
	}
	return into, nil
}

// Write is a convenience wrapper for getting the Codec for a particular KindEncoding and calling Codec.Write
func (k *Kind) Write(obj Object, out io.Writer, encoding KindEncoding) error {
	codec := k.Codec(encoding)
	if codec == nil {
		return fmt.Errorf("no codec for encoding '%s'", encoding)
	}
	return codec.Write(out, obj)
}

func NewJSONCodec() *JSONCodec {
	return &JSONCodec{}
}

// JSONCodec is a Codec-implementing struct that reads and writes kubernetes-formatted JSON bytes.
type JSONCodec struct{}

// Read is a simple wrapper for the json package unmarshal into the object.
// TODO: expect kubernetes-formatted bytes on input?
func (j *JSONCodec) Read(in io.Reader, out Object) error {
	// TODO: keep it this basic compared to Write?
	return json.NewDecoder(in).Decode(&out)
	/*m := make(map[string]json.RawMessage)
	err := json.NewDecoder(in).Decode(m)
	if err != nil {
		return err
	}

	// GVK
	kind, apiVersion := "", ""
	if encKind, ok := m["kind"]; ok {
		err = json.Unmarshal(encKind, &kind)
		if err != nil {
			return fmt.Errorf("error unmarshaling kind: %w", err)
		}
	} else {
		return fmt.Errorf("provided JSON bytes do not contain root-level field 'kind'")
	}
	if encAPIVersion, ok := m["apiVersion"]; ok {
		err = json.Unmarshal(encAPIVersion, &apiVersion)
		if err != nil {
			return fmt.Errorf("error unmarshaling apiVersion: %w", err)
		}
	} else {
		return fmt.Errorf("provided JSON bytes do not contain root-level field 'kind'")
	}
	out.SetGroupVersionKind(schema.FromAPIVersionAndKind(apiVersion, kind))

	// Metadata

	// Spec
	// Reflection?

	// Subresources
	// Reflection?
	*/
}

// Write marshals the provided Object into kubernetes-formatted JSON bytes.
func (j *JSONCodec) Write(out io.Writer, in Object) error {
	m := make(map[string]any)
	m["apiVersion"], m["kind"] = in.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
	m["metadata"] = metav1.ObjectMeta{
		Name:                       in.GetName(),
		GenerateName:               in.GetGenerateName(),
		Namespace:                  in.GetNamespace(),
		SelfLink:                   in.GetSelfLink(),
		UID:                        in.GetUID(),
		ResourceVersion:            in.GetResourceVersion(),
		Generation:                 in.GetGeneration(),
		CreationTimestamp:          in.GetCreationTimestamp(),
		DeletionTimestamp:          in.GetDeletionTimestamp(),
		DeletionGracePeriodSeconds: in.GetDeletionGracePeriodSeconds(),
		Labels:                     in.GetLabels(),
		Annotations:                in.GetAnnotations(),
		OwnerReferences:            in.GetOwnerReferences(),
		Finalizers:                 in.GetFinalizers(),
		ManagedFields:              in.GetManagedFields(),
	}
	m["spec"] = in.GetSpec()
	for k, v := range in.GetSubresources() {
		m[k] = v
	}
	return json.NewEncoder(out).Encode(m)
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
