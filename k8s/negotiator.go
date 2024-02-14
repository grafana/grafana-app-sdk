package k8s

import (
	"encoding/json"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
)

// GenericNegotiatedSerializer implements runtime.NegotiatedSerializer and allows for JSON serialization and
// deserialization of resource.Object. Since it is generic, and has no schema information,
// wrapped objects are returned which require a call to `Into` to marshal into an actual resource.Object.
type GenericNegotiatedSerializer struct {
}

// SupportedMediaTypes returns the JSON supported media type with a GenericJSONDecoder and kubernetes JSON Framer.
func (*GenericNegotiatedSerializer) SupportedMediaTypes() []runtime.SerializerInfo {
	return []runtime.SerializerInfo{{
		MediaType: "application/json",
		StreamSerializer: &runtime.StreamSerializerInfo{
			Serializer: &GenericJSONDecoder{},
			Framer:     jsonserializer.Framer,
		},
	}}
}

// EncoderForVersion returns the `serializer` input
func (*GenericNegotiatedSerializer) EncoderForVersion(serializer runtime.Encoder,
	_ runtime.GroupVersioner) runtime.Encoder {
	return serializer
}

// DecoderToVersion returns a GenericJSONDecoder
func (*GenericNegotiatedSerializer) DecoderToVersion(_ runtime.Decoder, _ runtime.GroupVersioner) runtime.Decoder {
	return &GenericJSONDecoder{}
}

// GenericJSONDecoder implements runtime.Serializer and works with Untyped* objects to implement runtime.Object
type GenericJSONDecoder struct {
}

// Decode decodes the provided data into UntypedWatchObject or UntypedObjectWrapper
//
//nolint:gocritic,revive
func (*GenericJSONDecoder) Decode(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (
	runtime.Object, *schema.GroupVersionKind, error) {
	type check struct {
		metav1.TypeMeta `json:",inline"`
		Type            string        `json:"type"`
		Kind            string        `json:"kind"`
		Items           []interface{} `json:"items,omitempty"`
	}
	if into != nil {
		// We shouldn't encounter this
		// TODO: make better
		err := json.Unmarshal(data, into)
		return into, defaults, err
	}

	// Determine what kind of object we have the raw bytes for
	// We do this by unmarshalling into a superset of a few possible types, then narrowing down
	// TODO: this seems very naive, check how apimachinery does it typically
	chk := check{}
	err := json.Unmarshal(data, &chk)
	if chk.Type != "" {
		// Watch response
		w := &UntypedWatchObject{}
		err = json.Unmarshal(data, w)
		into = w
	} else if chk.Items != nil {
		// List
		// TODO
	} else if chk.Kind != "" {
		o := &UntypedObjectWrapper{}
		err = json.Unmarshal(data, o)
		o.object = data
		into = o
	}
	return into, defaults, err
}

// Encode json-encodes the provided object
func (*GenericJSONDecoder) Encode(obj runtime.Object, w io.Writer) error {
	// TODO: check compliance with resource.Object and use marshalJSON in that case
	b, e := json.Marshal(obj)
	if e != nil {
		return e
	}
	_, e = w.Write(b)
	return e
}

// Identifier returns "generic-json-decoder"
func (*GenericJSONDecoder) Identifier() runtime.Identifier {
	return "generic-json-decoder"
}

/*
type KindNegotiatedSerializer struct {
	Kind resource.Kind
}

// SupportedMediaTypes returns the JSON supported media type with a GenericJSONDecoder and kubernetes JSON Framer.
func (k *KindNegotiatedSerializer) SupportedMediaTypes() []runtime.SerializerInfo {
	supported := make([]runtime.SerializerInfo, 0)
	for encoding, codec := range k.Kind.Codecs {
		serializer := &CodecDecoder{
			Codec: codec,
		}
		info := runtime.SerializerInfo{
			MediaType:  string(encoding),
			Serializer: serializer,
		}

		// Framer is used for the stream serializer
		switch encoding {
		case resource.KindEncodingJSON:
			info.StreamSerializer = &runtime.StreamSerializerInfo{
				Serializer: serializer,
				Framer:     jsonserializer.Framer,
			}
			//case resource.KindEncodingYAML:
			// TODO: YAML framer
			//	framer = yamlserializer.Framer
		}
		supported = append(supported, info)
	}

	return supported
}

// EncoderForVersion returns the `serializer` input
func (*KindNegotiatedSerializer) EncoderForVersion(serializer runtime.Encoder,
	_ runtime.GroupVersioner) runtime.Encoder {
	return serializer
}

// DecoderToVersion returns a GenericJSONDecoder
func (*KindNegotiatedSerializer) DecoderToVersion(_ runtime.Decoder, _ runtime.GroupVersioner) runtime.Decoder {
	return &GenericJSONDecoder{}
}

// GenericJSONDecoder implements runtime.Serializer and works with Untyped* objects to implement runtime.Object
type CodecDecoder struct {
	SampleObject resource.Object
	Codec        resource.Codec
}

// Decode decodes the provided data into UntypedWatchObject or UntypedObjectWrapper
//
//nolint:gocritic,revive
func (c *CodecDecoder) Decode(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (
	runtime.Object, *schema.GroupVersionKind, error) {
	type check struct {
		metav1.TypeMeta `json:",inline"`
		Type            string          `json:"type"`
		Kind            string          `json:"kind"`
		Items           []interface{}   `json:"items,omitempty"`
		Object          json.RawMessage `json:"object"`
	}
	if into != nil {
		// We shouldn't encounter this
		// TODO: make better
		err := json.Unmarshal(data, into)
		return into, defaults, err
	}

	// Determine what kind of object we have the raw bytes for
	// We do this by unmarshalling into a superset of a few possible types, then narrowing down
	// TODO: this seems very naive, check how apimachinery does it typically
	chk := check{}
	err := json.Unmarshal(data, &chk)
	if chk.Type != "" {
		// Watch response
		w := &UntypedWatchObject{}
		err = json.Unmarshal(data, w)
		obj := c.SampleObject.Copy()
		c.Codec.Read(bytes.NewReader(check.Object), obj)
		into = &watch.Event{
			Type:   watch.EventType(chk.Type),
			Object: obj,
		}
		into = w
	} else if chk.Items != nil {
		// List
		// TODO
	} else if chk.Kind != "" {
		o := &UntypedObjectWrapper{}
		err = json.Unmarshal(data, o)
		o.object = data
		into = o
	}
	return into, defaults, err
}

// Encode json-encodes the provided object
func (*CodecDecoder) Encode(obj runtime.Object, w io.Writer) error {
	// TODO: check compliance with resource.Object and use marshalJSON in that case
	b, e := json.Marshal(obj)
	if e != nil {
		return e
	}
	_, e = w.Write(b)
	return e
}

// Identifier returns "generic-json-decoder"
func (*CodecDecoder) Identifier() runtime.Identifier {
	return "generic-json-decoder"
}
*/
