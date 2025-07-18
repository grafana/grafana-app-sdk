package jennies

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCRDOpenAPISchema(t *testing.T) {
	tests := []struct {
		name          string
		schemaName    string
		jsonData      []byte
		outputJSON    []byte
		expectedError error
	}{{
		name:          "empty document",
		schemaName:    "foo",
		jsonData:      []byte(`{}`),
		expectedError: fmt.Errorf("invalid components or schemas"),
	}, {
		name:          "missing schema",
		schemaName:    "foo",
		jsonData:      []byte(`{"components":{"schemas":{}}}`),
		expectedError: fmt.Errorf("schema foo not found"),
	}, {
		name:       "recursive schema",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"item":{"type":"object","properties":{"val":{"type":"string"},"next":{"type":"object","$ref":"#/components/schemas/item"}}},"foo":{"type":"object","properties":{"linked":{"type":"object","$ref":"#/components/schemas/item"}}}}}}`),
		outputJSON: []byte(`{"type":"object","properties":{"linked":{"type":"object","properties":{"val":{"type":"string"},"next":{"type":"object","x-kubernetes-preserve-unknown-fields":true}}}}}`),
	}, {
		name:       "additionalProperties replaced",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"foo":{"type":"object","additionalProperties":{"type":"object"}}}}}`),
		outputJSON: []byte(`{"type":"object","x-kubernetes-preserve-unknown-fields":true}`),
	}, {
		name:       "additionalProperties resolved",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"bar":{"type":"object","properties":{"baz":{"type":"string"}}},"foo":{"type":"object","additionalProperties":{"type":"object","$ref":"#/components/schemas/bar"}}}}}`),
		outputJSON: []byte(`{"type":"object","additionalProperties":{"type":"object","properties":{"baz":{"type":"string"}}}}`),
	}, {
		name:       "simple additionalProperties",
		schemaName: "foo",
		jsonData:   []byte(`{"components":{"schemas":{"foo":{"type":"object","properties":{"bar":{"type":"string"}},"additionalProperties":{}}}}}`),
		outputJSON: []byte(`{"type":"object","properties":{"bar":{"type":"string"}},"x-kubernetes-preserve-unknown-fields":true}`),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := openapi3.NewLoader().LoadFromData(test.jsonData)
			assert.NoError(t, err)
			output, err := GetCRDOpenAPISchema(doc.Components, test.schemaName)
			if test.expectedError != nil {
				assert.Equal(t, test.expectedError, err)
			} else {
				require.NoError(t, err)
				m, err := json.MarshalIndent(output, "", "  ")
				require.NoError(t, err)
				assert.JSONEq(t, string(test.outputJSON), string(m))
			}
		})
	}
}
