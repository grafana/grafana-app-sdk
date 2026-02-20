package conversion

import (
	"bytes"
	"errors"
	"fmt"

	v0alpha1 "github.com/grafana/grafana-app-sdk/examples/apiserver/apis/example/v0alpha1"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1alpha1 "github.com/grafana/grafana-app-sdk/examples/apiserver/apis/example/v1alpha1"

	v2alpha1 "github.com/grafana/grafana-app-sdk/examples/apiserver/apis/example/v2alpha1"
)

var _ k8s.Converter = &TestKindConverter{}

type TestKindConverter struct{}

func (c TestKindConverter) Convert(obj resource.ObjectOrRaw, targetAPIVersion string) ([]byte, error) {
	gvk := schema.FromAPIVersionAndKind(targetAPIVersion, obj.Kind)
	encoding := obj.Encoding
	if encoding == "" {
		encoding = resource.KindEncodingJSON
	}
	var internal *resource.ObjectOrRaw
	var err error
	switch gvk.Version {
	case v0alpha1.APIVersion:
		internal, err = ConvertV0alpha1TestKindToInternal(obj)

	case v1alpha1.APIVersion:
		internal, err = ConvertV1alpha1TestKindToInternal(obj)

	case v2alpha1.APIVersion:
		internal, err = ConvertV2alpha1TestKindToInternal(obj)

	default:
		return nil, fmt.Errorf("unsupported api version: %s", gvk.Version)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to internal version: %w", err)
	}
	if internal == nil {
		return nil, fmt.Errorf("failed to convert object to internal version: not found")
	}
	targetGVK := schema.FromAPIVersionAndKind(targetAPIVersion, obj.Kind)
	switch targetGVK.Version {
	case v0alpha1.APIVersion:
		result, err := ConvertInternalToV0alpha1TestKind(*internal)
		if err != nil {
			return nil, fmt.Errorf("failed to convert object to %s from internal version: %w", v0alpha1.APIVersion, err)
		}
		if result.Raw == nil {
			k := v0alpha1.TestKindKind()
			writer := bytes.Buffer{}
			err = k.Write(result.Object, &writer, encoding)
			if err != nil {
				return nil, fmt.Errorf("failed to write object bytes: %w", err)
			}
			result.Raw = writer.Bytes()
		}
		return result.Raw, nil

	case v1alpha1.APIVersion:
		result, err := ConvertInternalToV1alpha1TestKind(*internal)
		if err != nil {
			return nil, fmt.Errorf("failed to convert object to %s from internal version: %w", v1alpha1.APIVersion, err)
		}
		if result.Raw == nil {
			k := v1alpha1.TestKindKind()
			writer := bytes.Buffer{}
			err = k.Write(result.Object, &writer, encoding)
			if err != nil {
				return nil, fmt.Errorf("failed to write object bytes: %w", err)
			}
			result.Raw = writer.Bytes()
		}
		return result.Raw, nil

	case v2alpha1.APIVersion:
		result, err := ConvertInternalToV2alpha1TestKind(*internal)
		if err != nil {
			return nil, fmt.Errorf("failed to convert object to %s from internal version: %w", v2alpha1.APIVersion, err)
		}
		if result.Raw == nil {
			k := v2alpha1.TestKindKind()
			writer := bytes.Buffer{}
			err = k.Write(result.Object, &writer, encoding)
			if err != nil {
				return nil, fmt.Errorf("failed to write object bytes: %w", err)
			}
			result.Raw = writer.Bytes()
		}
		return result.Raw, nil

	default:
		return nil, fmt.Errorf("unsupported api version: %s", gvk.String())
	}
}
