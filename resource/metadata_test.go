package resource

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testStruct struct {
	Foo string `json:"foo"`
	Bar int    `json:"bar"`
	Baz bool   `json:"baz"`
}

func TestReadGrafanaAnnotation(t *testing.T) {
	annotations := map[string]string{
		"grafana.com/string":      "foo",
		"grafana.com/int":         "42",
		"grafana.com/bool":        "true",
		"grafana.com/float":       "2.718281828459",
		"grafana.com/time":        "2024-04-25T16:27:01Z",
		"grafana.com/stringSlice": `["foo","bar","baz"]`,
		"grafana.com/intSlice":    "[3,1,4,1,5,9]",
		"grafana.com/boolSlice":   "[true,false,false,true]",
		"grafana.com/floatSlice":  "[3.141592653,2.7182818]",
		"grafana.com/struct1":     `{"Foo":"bar"}`,
		"grafana.com/struct2":     `{"foo":"string","bar":42,"baz":true}`,
		"grafana.com/structSlice": `[{"foo":"string","bar":42,"baz":true},{"foo":"bar","bar":314,"baz":false}]`,
	}

	t.Run("missing key", func(t *testing.T) {
		_, err := ReadGrafanaAnnotation[string](annotations, "missing")
		assert.Equal(t, ErrAnnotationMissing, err)
	})

	t.Run("string value", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[string](annotations, "string")
		assert.Nil(t, err)
		assert.Equal(t, "foo", res)
	})

	t.Run("int value", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[int](annotations, "int")
		assert.Nil(t, err)
		assert.Equal(t, 42, res)
	})

	t.Run("bool value", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[bool](annotations, "bool")
		assert.Nil(t, err)
		assert.Equal(t, true, res)
	})

	t.Run("float value", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[float64](annotations, "float")
		assert.Nil(t, err)
		assert.Equal(t, 2.718281828459, res)
	})

	t.Run("time value", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[time.Time](annotations, "time")
		assert.Nil(t, err)
		assert.Equal(t, time.Date(2024, time.April, 25, 16, 27, 01, 0, time.UTC), res)
	})

	t.Run("string slice", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[[]string](annotations, "stringSlice")
		assert.Nil(t, err)
		assert.Equal(t, []string{"foo", "bar", "baz"}, res)
	})

	t.Run("int slice", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[[]int](annotations, "intSlice")
		assert.Nil(t, err)
		assert.Equal(t, []int{3, 1, 4, 1, 5, 9}, res)
	})

	t.Run("bool slice", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[[]bool](annotations, "boolSlice")
		assert.Nil(t, err)
		assert.Equal(t, []bool{true, false, false, true}, res)
	})

	t.Run("float slice", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[[]float64](annotations, "floatSlice")
		assert.Nil(t, err)
		assert.Equal(t, []float64{3.141592653, 2.7182818}, res)
	})

	t.Run("anonymous struct", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[struct{ Foo string }](annotations, "struct1")
		assert.Nil(t, err)
		assert.Equal(t, struct{ Foo string }{"bar"}, res)
	})

	t.Run("struct", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[testStruct](annotations, "struct2")
		assert.Nil(t, err)
		assert.Equal(t, testStruct{"string", 42, true}, res)
	})

	t.Run("struct slice", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[[]testStruct](annotations, "structSlice")
		assert.Nil(t, err)
		assert.Equal(t, []testStruct{{"string", 42, true}, {"bar", 314, false}}, res)
	})

	t.Run("string pointer", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[*string](annotations, "string")
		assert.Nil(t, err)
		require.NotNil(t, res)
		assert.Equal(t, "foo", *res)
	})

	t.Run("int pointer", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[*int](annotations, "int")
		assert.Nil(t, err)
		require.NotNil(t, res)
		assert.Equal(t, 42, *res)
	})

	t.Run("bool pointer", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[*bool](annotations, "bool")
		assert.Nil(t, err)
		require.NotNil(t, res)
		assert.Equal(t, true, *res)
	})

	t.Run("float pointer", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[*float64](annotations, "float")
		assert.Nil(t, err)
		require.NotNil(t, res)
		assert.Equal(t, 2.718281828459, *res)
	})

	t.Run("struct pointer", func(t *testing.T) {
		res, err := ReadGrafanaAnnotation[*testStruct](annotations, "struct2")
		assert.Nil(t, err)
		require.NotNil(t, res)
		assert.Equal(t, testStruct{"string", 42, true}, *res)
	})

	t.Run("string type, int parse", func(t *testing.T) {
		i := 0
		castErr := json.Unmarshal([]byte("foo"), &i)
		res, err := ReadGrafanaAnnotation[int](annotations, "string")
		assert.Equal(t, castErr, err)
		assert.Empty(t, res)
	})
}

func TestWriteGrafanaAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		field       string
		value       any
		expectedErr error
		expectedMap map[string]string
	}{{
		name: "remove key",
		annotations: map[string]string{
			"grafana.com/foo": "bar",
			"grafana.com/bar": "foo",
		},
		field:       "foo",
		value:       nil,
		expectedErr: nil,
		expectedMap: map[string]string{
			"grafana.com/bar": "foo",
		},
	}, {
		name:        "string",
		annotations: map[string]string{},
		field:       "foo",
		value:       "bar",
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/foo": "bar"},
	}, {
		name:        "int",
		annotations: map[string]string{},
		field:       "intField",
		value:       34,
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/intField": "34"},
	}, {
		name:        "bool",
		annotations: map[string]string{},
		field:       "bar",
		value:       true,
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/bar": "true"},
	}, {
		name:        "float",
		annotations: map[string]string{},
		field:       "baz",
		value:       12.3456789,
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/baz": "12.3456789"},
	}, {
		name:        "time",
		annotations: map[string]string{},
		field:       "ts",
		value:       time.Date(2024, time.April, 25, 16, 27, 01, 0, time.UTC),
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/ts": "2024-04-25T16:27:01Z"},
	}, {
		name:        "struct",
		annotations: map[string]string{},
		field:       "foobar",
		value:       testStruct{"val", 91, false},
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/foobar": `{"foo":"val","bar":91,"baz":false}`},
	}, {
		name:        "string slice",
		annotations: map[string]string{},
		field:       "foo",
		value:       []string{"foo", "bar"},
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/foo": `["foo","bar"]`},
	}, {
		name:        "int slice",
		annotations: map[string]string{},
		field:       "intField",
		value:       []int{1, 2, 3},
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/intField": "[1,2,3]"},
	}, {
		name:        "bool slice",
		annotations: map[string]string{},
		field:       "bar",
		value:       []bool{true, false, true},
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/bar": "[true,false,true]"},
	}, {
		name:        "float slice",
		annotations: map[string]string{},
		field:       "baz",
		value:       []float64{12.3456789, 987.654321},
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/baz": "[12.3456789,987.654321]"},
	}, {
		name:        "struct slice",
		annotations: map[string]string{},
		field:       "foobar",
		value:       []testStruct{{"val", 91, false}},
		expectedErr: nil,
		expectedMap: map[string]string{"grafana.com/foobar": `[{"foo":"val","bar":91,"baz":false}]`},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := WriteGrafanaAnnotation(test.annotations, test.field, test.value)
			assert.Equal(t, test.expectedErr, err)
			assert.Equal(t, test.expectedMap, test.annotations)
		})
	}
}
