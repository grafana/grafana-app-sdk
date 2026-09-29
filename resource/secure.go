package resource

import (
	"fmt"
	"io"

	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// RawSecureValue holds plaintext received in a create request. Normal formatting
// and direct JSON/YAML serialization redact it. JSONCodec preserves plaintext for transport.
type RawSecureValue string

func (RawSecureValue) String() string                 { return "[REDACTED]" }
func (RawSecureValue) GoString() string               { return "[REDACTED]" }
func (RawSecureValue) Format(state fmt.State, _ rune) { _, _ = io.WriteString(state, "[REDACTED]") }
func (RawSecureValue) MarshalJSON() ([]byte, error)   { return []byte(`"[REDACTED]"`), nil }
func (RawSecureValue) MarshalYAML() (any, error)      { return "[REDACTED]", nil }
func (v RawSecureValue) IsZero() bool                 { return v == "" }

// NewSecretValue wraps plaintext for use in an inline secure create operation.
func NewSecretValue(value string) RawSecureValue { return RawSecureValue(value) }

// DangerouslyExposeAndConsumeValue returns the plaintext and clears this wrapper.
// It panics if the wrapper is empty or has already been consumed.
func (v *RawSecureValue) DangerouslyExposeAndConsumeValue() string {
	if *v == "" {
		panic("secure value is empty or was consumed")
	}
	value := string(*v)
	*v = ""
	return value
}

// InlineSecureValue matches Grafana's inline secure value wire format.
// Exactly one of create, name, or remove must be supplied.
// +k8s:openapi-gen=true
type InlineSecureValue struct {
	Create      RawSecureValue `json:"create,omitempty" yaml:"create,omitempty"`
	Description *string        `json:"description,omitempty" yaml:"description,omitempty"`
	Name        string         `json:"name,omitempty" yaml:"name,omitempty"`
	Remove      bool           `json:"remove,omitempty" yaml:"remove,omitempty"`
}

func (v InlineSecureValue) IsZero() bool { return v.Create.IsZero() && v.Name == "" && !v.Remove }

// OpenAPIModelName identifies the shared value type in generated OpenAPI definitions.
func (InlineSecureValue) OpenAPIModelName() string {
	return "com.github.grafana.grafana-app-sdk.resource.InlineSecureValue"
}

// OpenAPIDefinition describes the inline value, including mutually exclusive operations.
func (InlineSecureValue) OpenAPIDefinition() common.OpenAPIDefinition {
	stringSchema := func(description string, minimum, maximum int64) spec.Schema {
		return spec.Schema{SchemaProps: spec.SchemaProps{Description: description, Type: []string{"string"}, MinLength: &minimum, MaxLength: &maximum}}
	}
	return common.OpenAPIDefinition{Schema: spec.Schema{SchemaProps: spec.SchemaProps{
		Type:                 []string{"object"},
		AdditionalProperties: &spec.SchemaOrBool{Allows: false},
		Properties: map[string]spec.Schema{
			"create": stringSchema("Plaintext value to create in the secret service. Used only in create or update requests.", 1, 24576),
			"name":   stringSchema("Reference to an existing value in the secret service.", 1, 253),
			"remove": {SchemaProps: spec.SchemaProps{
				Type: []string{"boolean"}, Description: "Remove this entry from the secure value map, deleting the secret if it is owned by this resource.",
			}},
			"description": {SchemaProps: spec.SchemaProps{
				Type: []string{"string"}, Description: "Optional description for a newly created secret. Only valid when create is set.",
			}},
		},
		OneOf: []spec.Schema{
			{SchemaProps: spec.SchemaProps{Required: []string{"create"}}},
			{SchemaProps: spec.SchemaProps{Required: []string{"name"}}},
			{SchemaProps: spec.SchemaProps{Required: []string{"remove"}}},
		},
		// Reject description when create is absent, including an empty description.
		Not: &spec.Schema{SchemaProps: spec.SchemaProps{
			Required: []string{"description"},
			Not:      &spec.Schema{SchemaProps: spec.SchemaProps{Required: []string{"create"}}},
		}},
	}}}
}

// InlineSecureValues is a map of resource-local keys to secure value operations or references.
type InlineSecureValues = map[string]InlineSecureValue

// DecryptedSecureValues expose the raw values already decrypted
type DecryptedSecureValues = map[string]RawSecureValue

// ObjectWithSecureValues is an optional capability for objects with a top-level secure map.
// Secure values are not REST subresources.
type ObjectWithSecureValues interface {
	Object
	GetSecureValues() InlineSecureValues
	SetSecureValues(InlineSecureValues) error
}

// CopySecureValues copies the map and its descriptions without serializing plaintext.
func CopySecureValues(values InlineSecureValues) InlineSecureValues {
	if values == nil {
		return nil
	}
	result := make(InlineSecureValues, len(values))
	for key, value := range values {
		if value.Description != nil {
			description := *value.Description
			value.Description = &description
		}
		result[key] = value
	}
	return result
}
