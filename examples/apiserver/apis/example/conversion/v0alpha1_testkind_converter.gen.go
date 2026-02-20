package conversion

import (
	"github.com/grafana/grafana-app-sdk/resource"

	v0alpha1 "github.com/grafana/grafana-app-sdk/examples/apiserver/apis/example/v0alpha1"
	v1alpha1 "github.com/grafana/grafana-app-sdk/examples/apiserver/apis/example/v1alpha1"
)

// Set one or both of these variables to a non-nil function to override the generated default behavior.
// Since this file is auto-generated, set these functions in a separate file which will not be overwritten by the codegen.
var (
	v0alpha1TestKindToInternalFunc   func(resource.ObjectOrRaw) (*TypedObjectOrRaw[*v0alpha1.TestKind], error)
	v0alpha1TestKindFromInternalFunc func(TypedObjectOrRaw[*v1alpha1.TestKind]) (*resource.ObjectOrRaw, error)
)

func ConvertV0alpha1TestKindToInternal(raw resource.ObjectOrRaw) (*resource.ObjectOrRaw, error) {
	if v0alpha1TestKindToInternalFunc != nil {
		res, err := v0alpha1TestKindToInternalFunc(raw)
		if err != nil {
			return nil, err
		}
		return &resource.ObjectOrRaw{
			Raw:      res.Raw,
			Encoding: res.Encoding,
			Object:   res.Object,
		}, nil
	}

	// Unmarshal the object if necessary
	obj, err := getObjectFromRawType[*v0alpha1.TestKind](raw, v0alpha1.TestKindKind())
	if err != nil {
		return nil, err
	}

	// Do the conversion
	// TODO
	return &resource.ObjectOrRaw{
		Object: obj,
	}, nil
}

func ConvertInternalToV0alpha1TestKind(raw resource.ObjectOrRaw) (*resource.ObjectOrRaw, error) {
	if v0alpha1TestKindFromInternalFunc != nil {
		typed := TypedObjectOrRaw[*v1alpha1.TestKind]{
			Raw:      raw.Raw,
			Encoding: raw.Encoding,
		}
		if raw.Object != nil {
			cast, ok := raw.Object.(*v1alpha1.TestKind)
			if ok {
				typed.Object = cast
			}
		}
		return v0alpha1TestKindFromInternalFunc(typed)
	}

	// Unmarshal the object if necessary
	obj, err := getObjectFromRawType[*v1alpha1.TestKind](raw, v1alpha1.TestKindKind())
	if err != nil {
		return nil, err
	}

	// Do the conversion
	// TODO
	return &resource.ObjectOrRaw{
		Object: obj,
	}, nil
}
