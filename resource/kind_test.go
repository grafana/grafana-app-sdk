package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUntypedKind_Read(t *testing.T) {
	emptyJSONErr := json.NewDecoder(&bytes.Buffer{}).Decode(&struct{}{})
	unexpectedEndErr := json.Unmarshal([]byte{}, &struct{}{})

	tests := []struct {
		name  string
		input []byte
		obj   *UntypedObject
		err   error
	}{{
		name:  "nil bytes",
		input: nil,
		obj:   nil,
		err:   emptyJSONErr,
	}, {
		name:  "empty bytes",
		input: []byte{},
		obj:   nil,
		err:   emptyJSONErr,
	}, {
		name:  "missing Kind",
		input: []byte(`{"apiVersion":"foo.bar/v1","metadata":{}}`),
		err:   fmt.Errorf("error reading kind: %w", unexpectedEndErr),
	}, {
		name:  "missing APIVersion",
		input: []byte(`{"kind":"Foo","metadata":{}}`),
		err:   fmt.Errorf("error reading apiVersion: %w", unexpectedEndErr),
	}, {
		name:  "missing Metadata",
		input: []byte(`{"kind":"Foo","apiVersion":"foo.bar/v1"}`),
		err:   fmt.Errorf("error reading metadata: %w", unexpectedEndErr),
	}, {
		name:  "mostly empty",
		input: []byte(`{"kind":"Foo","apiVersion":"foo.bar/v1","metadata":{"namespace":"ns","name":"bar"}}`),
		obj: &UntypedObject{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Foo",
				APIVersion: "foo.bar/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "bar",
			},
		},
	}, {
		name:  "simple",
		input: []byte(`{"kind":"Foo","apiVersion":"foo.bar/v1","metadata":{"namespace":"ns","name":"bar"},"spec":{"foo":{"inner":"bar"}},"status":{"bar":"foo"}}`),
		obj: &UntypedObject{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Foo",
				APIVersion: "foo.bar/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "bar",
			},
			Spec:         map[string]any{"foo": map[string]any{"inner": "bar"}},
			Subresources: map[string]json.RawMessage{"status": []byte(`{"bar":"foo"}`)},
		},
	}}

	for _, test := range tests {
		k := &UntypedKind{}

		t.Run(test.name, func(t *testing.T) {
			out, err := k.Read(bytes.NewReader(test.input), KindEncodingJSON)
			require.Equal(t, test.err, err)
			if test.obj != nil {
				require.NotNil(t, out)
				obj, ok := out.(*UntypedObject)
				require.True(t, ok, "output object is not of type *UntypedObject")
				assert.Equal(t, *test.obj, *obj)
			}
		})
	}
}
