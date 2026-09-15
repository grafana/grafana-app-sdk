package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestKind_GroupVersionKind(t *testing.T) {
	// nil Schema
	k := Kind{}
	gvk := k.GroupVersionKind()
	assert.Equal(t, schema.GroupVersionKind{}, gvk)
	// Values
	k.Schema = NewSimpleSchema("group", "version", &UntypedObject{}, &UntypedList{}, WithKind("kind"))
	gvk = k.GroupVersionKind()
	assert.Equal(t, schema.GroupVersionKind{
		Group:   k.Schema.Group(),
		Version: k.Schema.Version(),
		Kind:    k.Schema.Kind(),
	}, gvk)

	require.Equal(t, gvk, Kind{
		Schema: k.Schema,
		Codecs: k.Codecs,
	}.GroupVersionKind(), "GVK does not require pointer")
}

func TestKind_GroupVersionResource(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		plural   string
		resource string
	}{
		{
			name:     "lowercase plural",
			kind:     "Widget",
			plural:   "widgets",
			resource: "widgets",
		},
		{
			name:     "mixed-case plural is lowercased",
			kind:     "Widget",
			plural:   "WidgetItems",
			resource: "widgetitems",
		},
		{
			name:     "missing plural falls back to lowercased kind",
			kind:     "WidgetItem",
			resource: "widgetitems",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceSchema := NewSimpleSchema(
				"group",
				"version",
				&UntypedObject{},
				&UntypedList{},
				WithKind(tt.kind),
			)
			// NewSimpleSchema supplies a default plural, so set the field directly to
			// exercise Kind's behavior when a Schema returns an empty plural.
			resourceSchema.plural = tt.plural
			kind := Kind{
				Schema: resourceSchema,
			}

			expected := schema.GroupVersionResource{
				Group:    "group",
				Version:  "version",
				Resource: tt.resource,
			}
			assert.Equal(t, expected, kind.GroupVersionResource())
		})
	}

	t.Run("nil schema", func(t *testing.T) {
		assert.Equal(t, schema.GroupVersionResource{}, Kind{}.GroupVersionResource())
	})

	kind := Kind{Schema: NewSimpleSchema(
		"group",
		"version",
		&UntypedObject{},
		&UntypedList{},
		WithKind("Widget"),
	)}
	require.Equal(t, kind.GroupVersionResource(), Kind{
		Schema: kind.Schema,
		Codecs: kind.Codecs,
	}.GroupVersionResource(), "GVR does not require pointer")
}

func TestJSONCodec_Read(t *testing.T) {
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
		c := &JSONCodec{}

		t.Run(test.name, func(t *testing.T) {
			out := &UntypedObject{}
			err := c.Read(bytes.NewReader(test.input), out)
			require.Equal(t, test.err, err)
			if test.obj != nil {
				assert.Equal(t, *test.obj, *out)
			}
		})
	}
}
