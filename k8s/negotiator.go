package k8s

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/grafana/grafana-app-sdk/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"

	"github.com/grafana/grafana-app-sdk/resource"
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

type KindNegotiatedSerializer struct {
	Kind resource.Kind
}

// SupportedMediaTypes returns the JSON supported media type with a GenericJSONDecoder and kubernetes JSON Framer.
func (k *KindNegotiatedSerializer) SupportedMediaTypes() []runtime.SerializerInfo {
	supported := make([]runtime.SerializerInfo, 0)
	for encoding, codec := range k.Kind.Codecs {
		serializer := &CodecDecoder{
			SampleObject: k.Kind.ZeroValue(),
			Codec:        codec,
		}
		info := runtime.SerializerInfo{
			MediaType:  string(encoding),
			Serializer: serializer,
		}

		// Framer is used for the stream serializer
		switch encoding {
		case resource.KindEncodingJSON:
			serializer.Decoder = json.Unmarshal
			info.Serializer = serializer
			info.StreamSerializer = &runtime.StreamSerializerInfo{
				Serializer: serializer,
				Framer:     jsonserializer.Framer,
			}
		case resource.KindEncodingYAML:
			// TODO: YAML framer
			//	framer = yamlserializer.Framer <- doesn't exist
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
func (*KindNegotiatedSerializer) DecoderToVersion(d runtime.Decoder, _ runtime.GroupVersioner) runtime.Decoder {
	return d
}

// CodecDecoder implements runtime.Serializer and works with Untyped* objects to implement runtime.Object
type CodecDecoder struct {
	SampleObject resource.Object
	Codec        resource.Codec
	Decoder      func([]byte, any) error
}

// Decode decodes the provided data into UntypedWatchObject or UntypedObjectWrapper
//
//nolint:gocritic,revive
func (c *CodecDecoder) Decode(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (
	runtime.Object, *schema.GroupVersionKind, error) {
	if defaults != nil {
		logging.DefaultLogger.Debug("decoding object", "group", defaults.Group, "version", defaults.Version, "kind", defaults.Kind, "payload", string(data))
	} else {
		logging.DefaultLogger.Debug("decoding object with nil defaults", "payload", string(data))
	}
	if into != nil {
		logging.DefaultLogger.Debug("using provided into object", "gvk", into.GetObjectKind().GroupVersionKind().String())
		if cast, ok := into.(resource.Object); ok {
			logging.DefaultLogger.Debug("decoding into resource.Object")
			err := c.Codec.Read(bytes.NewReader(data), cast)
			return cast, defaults, err
		}
		if cast, ok := into.(*metav1.WatchEvent); ok {
			logging.DefaultLogger.Debug("decoding into *metav1.WatchEvent")
			err := c.Decoder(data, cast)
			return cast, defaults, err
		}
		if cast, ok := into.(*metav1.List); ok {
			logging.DefaultLogger.Debug("decoding into *metav1.List")
			err := c.Decoder(data, cast)
			return cast, defaults, err
		}
		// We shouldn't encounter this
		// TODO: make better or return error?
		logging.DefaultLogger.Debug("generic decode")
		err := c.Decoder(data, into)
		return into, defaults, err
	}

	logging.DefaultLogger.Debug("into is nil, checking defaults")
	if defaults != nil {
		if defaults.Kind == "Status" && defaults.Version == "v1" {
			logging.DefaultLogger.Debug("defaults is a v1/Status object, decoding into that")
			obj := &metav1.Status{}
			err := c.Decoder(data, obj)
			return obj, defaults, err
		}
	}
	logging.DefaultLogger.Debug("nothing found in defaults, doing a partial unmarshal to check GVK")
	tm := metav1.TypeMeta{}
	err := c.Decoder(data, &tm)
	if err != nil {
		return nil, nil, fmt.Errorf("error decoding object TypeMeta: %w", err)
	}
	logging.DefaultLogger.Debug("decoded TypeMeta", "gvk", tm.GroupVersionKind().String())
	if tm.GroupVersionKind().Version == "v1" && tm.GroupVersionKind().Kind == "Status" {
		logging.DefaultLogger.Debug("TypeMeta is a v1/Status object, decoding into that")
		obj := &metav1.Status{}
		err := c.Decoder(data, obj)
		return obj, defaults, err
	}
	logging.DefaultLogger.Debug("decoding into a new empty object instance", "gvk", c.SampleObject.GetObjectKind().GroupVersionKind().String())
	obj := c.SampleObject.Copy()
	err = c.Codec.Read(bytes.NewReader(data), obj)
	return obj, defaults, err
}

// Encode json-encodes the provided object
func (c *CodecDecoder) Encode(obj runtime.Object, w io.Writer) error {
	if cast, ok := obj.(resource.Object); ok {
		return c.Codec.Write(w, cast)
	}
	return fmt.Errorf("provided object is not a resource.Object")
}

// Identifier returns "generic-json-decoder"
func (*CodecDecoder) Identifier() runtime.Identifier {
	return "codec-decoder"
}
