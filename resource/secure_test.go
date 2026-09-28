package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"k8s.io/kube-openapi/pkg/validation/strfmt"
	"k8s.io/kube-openapi/pkg/validation/validate"
)

func TestObjectWithSecureValuesRoundTrip(t *testing.T) {
	for _, tt := range []struct {
		name      string
		newObject func() ObjectWithSecureValues
	}{
		{"typed spec", func() ObjectWithSecureValues { return &TypedSpecObject[map[string]any]{} }},
		{"typed status", func() ObjectWithSecureValues { return &TypedSpecStatusObject[map[string]any, map[string]any]{} }},
		{"typed catalog", func() ObjectWithSecureValues { return &TypedObject[map[string]any, struct{}]{} }},
		{"untyped", func() ObjectWithSecureValues { return &UntypedObject{} }},
		{"wrapped", func() ObjectWithSecureValues { return NewWrappedObject(&TypedSpecObject[map[string]any]{}) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.newObject()
			payload := `{"apiVersion":"test/v1","kind":"Foo","metadata":{},"spec":{},"secure":{"key":{"create":"test-plaintext","description":"a key"},"ref":{"name":"existing"},"delete":{"remove":true}}}`
			require.NoError(t, json.Unmarshal([]byte(payload), obj))
			require.Len(t, obj.GetSecureValues(), 3)
			assert.NotContains(t, obj.GetSubresources(), "secure")
			assert.Equal(t, "test-plaintext", string(obj.GetSecureValues()["key"].Create))
			copied, ok := obj.Copy().(ObjectWithSecureValues)
			require.True(t, ok)
			*copied.GetSecureValues()["key"].Description = "changed"
			assert.Equal(t, "a key", *obj.GetSecureValues()["key"].Description)
			assert.Equal(t, "test-plaintext", string(copied.GetSecureValues()["key"].Create))
			delete(copied.GetSecureValues(), "ref")
			assert.Contains(t, obj.GetSecureValues(), "ref")
			codec := &JSONCodec{}
			for range 2 {
				var encoded bytes.Buffer
				require.NoError(t, codec.Write(&encoded, obj))
				assert.Contains(t, encoded.String(), "test-plaintext")
				restored := tt.newObject()
				require.NoError(t, codec.Read(&encoded, restored))
				assert.Equal(t, obj.GetSecureValues(), restored.GetSecureValues())
			}

			require.NoError(t, obj.SetSecureValues(nil))
			encoded, err := json.Marshal(obj)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), `"secure"`)
		})
	}
}

func TestRawSecureValueRedaction(t *testing.T) {
	value := NewSecretValue("test-plaintext")
	for _, format := range []string{"%s", "%v", "%+v", "%#v", "%q"} {
		assert.NotContains(t, fmt.Sprintf(format, value), "test-plaintext")
	}
	for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal} {
		encoded, err := marshal(InlineSecureValue{Create: value})
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "test-plaintext")
		assert.Contains(t, string(encoded), "[REDACTED]")
	}
	assert.Equal(t, "test-plaintext", value.DangerouslyExposeAndConsumeValue())
	assert.True(t, value.IsZero())
	assert.Panics(t, func() { value.DangerouslyExposeAndConsumeValue() })
}

func TestInlineSecureValueSchema(t *testing.T) {
	schema := InlineSecureValue{}.OpenAPIDefinition().Schema
	for name, property := range schema.Properties {
		assert.NotEmpty(t, property.Description, "description for %s", name)
	}
	for _, tt := range []struct {
		name  string
		value map[string]any
		valid bool
	}{
		{"create", map[string]any{"create": "secret", "description": "from service"}, true},
		{"create without description", map[string]any{"create": "secret"}, true},
		{"create with empty description", map[string]any{"create": "secret", "description": ""}, true},
		{"description only", map[string]any{"description": "from service"}, false},
		{"reference with description", map[string]any{"name": "existing", "description": "from service"}, false},
		{"reference with empty description", map[string]any{"name": "existing", "description": ""}, false},
		{"remove with description", map[string]any{"remove": true, "description": "from service"}, false},
		{"reference", map[string]any{"name": "existing"}, true},
		{"remove", map[string]any{"remove": true}, true},
		{"empty", map[string]any{}, false},
		{"two operations", map[string]any{"create": "secret", "name": "existing"}, false},
		{"empty create", map[string]any{"create": ""}, false},
		{"long create", map[string]any{"create": strings.Repeat("x", 24577)}, false},
		{"long name", map[string]any{"name": strings.Repeat("x", 254)}, false},
		{"unknown field", map[string]any{"name": "existing", "unknown": true}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := validate.NewSchemaValidator(&schema, nil, "", strfmt.Default).Validate(tt.value)
			assert.Equal(t, tt.valid, result.IsValid(), "%v", result.Errors)
		})
	}
}
