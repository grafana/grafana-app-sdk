package k8s

import (
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCodecDecoder_Decode(t *testing.T) {
	defaultCodecDecoder := CodecDecoder{
		SampleObject: testKind.ZeroValue(),
		SampleList:   testKind.ZeroListValue(),
		Codec:        testKind.Codecs[resource.KindEncodingJSON],
		Decoder:      json.Unmarshal,
	}

	jsonEmptyErr := json.Unmarshal([]byte(``), &struct{}{})
	testObjectJSON, _ := json.Marshal(getTestObject())
	testObjectUntyped := &resource.UntypedObject{}
	_ = json.Unmarshal(testObjectJSON, testObjectUntyped)
	testObjectList := &resource.TypedList[*resource.TypedSpecObject[testSpec]]{
		TypeMeta: metav1.TypeMeta{
			Kind:       testKind.Kind(),
			APIVersion: testKind.GroupVersionKind().GroupVersion().String(),
		},
		Items: []*resource.TypedSpecObject[testSpec]{getTestObject()},
	}
	testListJSON, _ := json.Marshal(testObjectList)

	tests := []struct {
		name           string
		codec          CodecDecoder
		data           []byte
		defaults       *schema.GroupVersionKind
		into           runtime.Object
		expectedObject runtime.Object
		expectedGVK    *schema.GroupVersionKind
		expectedError  error
	}{{
		name:           "no data",
		codec:          defaultCodecDecoder,
		data:           nil,
		defaults:       nil,
		into:           nil,
		expectedObject: nil,
		expectedGVK:    nil,
		expectedError:  fmt.Errorf("error decoding object TypeMeta: %w", jsonEmptyErr),
	}, {
		name: "codec error",
		codec: CodecDecoder{
			Codec: &TestCodec{
				ReadFunc: func(reader io.Reader, object resource.Object) error {
					return fmt.Errorf("I AM ERROR")
				},
			},
			Decoder: json.Unmarshal,
		},
		data:           []byte(`{}`),
		defaults:       nil,
		into:           testKind.ZeroValue(),
		expectedObject: testKind.ZeroValue(), // Codec error returns `into` as-is
		expectedGVK:    nil,
		expectedError:  fmt.Errorf("I AM ERROR"),
	}, {
		name:           "testKind into",
		codec:          defaultCodecDecoder,
		data:           testObjectJSON,
		into:           testKind.ZeroValue(),
		expectedObject: getTestObject(),
	}, {
		name:           "testKind list into",
		codec:          defaultCodecDecoder,
		data:           testListJSON,
		into:           testKind.ZeroListValue(),
		expectedObject: testObjectList,
	}, {
		name:  "*v1.WatchEvent into",
		codec: defaultCodecDecoder,
		data:  []byte(`{"type":"UPDATE","object":{}}`),
		into:  &metav1.WatchEvent{},
		expectedObject: &metav1.WatchEvent{
			Type: "UPDATE",
			Object: runtime.RawExtension{
				Raw: []byte(`{}`),
			},
		},
	}, {
		name:  "*v1.List into",
		codec: defaultCodecDecoder,
		data:  testListJSON,
		into:  &metav1.List{},
		expectedObject: &metav1.List{
			TypeMeta: metav1.TypeMeta{
				Kind:       testObjectList.Kind,
				APIVersion: testObjectList.APIVersion,
			},
			Items: []runtime.RawExtension{{
				Raw: testObjectJSON,
			}},
		},
	}, {
		name:  "*v1.Status into",
		codec: defaultCodecDecoder,
		data:  []byte(`{"status":"Failure","message":"a failure","reason":"ERR_BECAUSE"}`),
		into:  &metav1.Status{},
		expectedObject: &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: "a failure",
			Reason:  "ERR_BECAUSE",
		},
	}, {
		name:           "unregistered into",
		codec:          defaultCodecDecoder,
		data:           []byte(`{"kind":"Foo","apiVersion":"bar"}`),
		into:           &testRuntimeObject{},
		expectedObject: &testRuntimeObject{},
	}, {
		name:  "nil into, v1.Status defaults",
		codec: defaultCodecDecoder,
		data:  []byte(`{"status":"Failure","message":"a failure","reason":"ERR_BECAUSE"}`),
		defaults: &schema.GroupVersionKind{
			Version: "v1",
			Kind:    "Status",
		},
		expectedObject: &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: "a failure",
			Reason:  "ERR_BECAUSE",
		},
		expectedGVK: &schema.GroupVersionKind{
			Version: "v1",
			Kind:    "Status",
		},
	}, {
		name:  "nil into and defaults, v1.Status TypeMeta",
		codec: defaultCodecDecoder,
		data:  []byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","message":"a failure","reason":"ERR_BECAUSE"}`),
		expectedObject: &metav1.Status{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Status",
				APIVersion: "v1",
			},
			Status:  metav1.StatusFailure,
			Message: "a failure",
			Reason:  "ERR_BECAUSE",
		},
	}, {
		name:           "nil into and defaults, has list items",
		codec:          defaultCodecDecoder,
		data:           testListJSON,
		expectedObject: testObjectList,
	}, {
		name:           "nil into and defaults",
		codec:          defaultCodecDecoder,
		data:           testObjectJSON,
		expectedObject: getTestObject(),
	}, {
		name: "nil into and defaults, has list item, no SampleList",
		codec: CodecDecoder{
			Codec:   testKind.Codecs[resource.KindEncodingJSON],
			Decoder: json.Unmarshal,
		},
		data: testListJSON,
		expectedObject: &resource.TypedList[*resource.UntypedObject]{
			TypeMeta: metav1.TypeMeta{
				Kind:       testObjectList.Kind,
				APIVersion: testObjectList.APIVersion,
			},
			Items: []*resource.UntypedObject{testObjectUntyped},
		},
	}, {
		name: "nil into and defaults, no SampleObject",
		codec: CodecDecoder{
			Codec:   testKind.Codecs[resource.KindEncodingJSON],
			Decoder: json.Unmarshal,
		},
		data:           testObjectJSON,
		expectedObject: testObjectUntyped,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, gvk, err := test.codec.Decode(test.data, test.defaults, test.into)
			assert.Equal(t, test.expectedObject, out)
			assert.Equal(t, test.expectedGVK, gvk)
			assert.Equal(t, test.expectedError, err)
		})
	}
}

var _ runtime.Object = &testRuntimeObject{}

type testRuntimeObject struct {
	GetObjectKindFunc  func() schema.ObjectKind
	DeepCopyObjectFunc func() runtime.Object
}

func (t testRuntimeObject) GetObjectKind() schema.ObjectKind {
	if t.GetObjectKindFunc != nil {
		return t.GetObjectKindFunc()
	}
	return schema.EmptyObjectKind
}

func (t testRuntimeObject) DeepCopyObject() runtime.Object {
	if t.DeepCopyObjectFunc != nil {
		return t.DeepCopyObjectFunc()
	}
	return &testRuntimeObject{}
}
