package resource

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nestedCopyTestValue struct {
	Properties map[string]any
	Values     []string
	Position   *int
}

func TestCopyObjectIntoNestedStructSlice(t *testing.T) {
	type spec struct {
		Targets []nestedCopyTestValue
	}
	position := 1
	original := &spec{Targets: []nestedCopyTestValue{{
		Properties: map[string]any{"expr": "original"},
		Values:     []string{"original"},
		Position:   &position,
	}}}
	copied := &spec{}
	require.NoError(t, CopyObjectInto(copied, original))
	require.Equal(t, original, copied)

	copied.Targets[0].Properties["expr"] = "changed"
	copied.Targets[0].Values[0] = "changed"
	*copied.Targets[0].Position = 2

	assert.Equal(t, "original", original.Targets[0].Properties["expr"])
	assert.Equal(t, []string{"original"}, original.Targets[0].Values)
	assert.Equal(t, 1, *original.Targets[0].Position)
}

func TestCopyObjectNestedValues(t *testing.T) {
	tests := []struct {
		name   string
		value  func() any
		mutate func(any)
	}{
		{
			name: "slice of structs containing maps slices and pointers",
			value: func() any {
				position := 1
				return []nestedCopyTestValue{{Properties: map[string]any{"expr": "original"}, Values: []string{"original"}, Position: &position}}
			},
			mutate: func(value any) {
				values := value.([]nestedCopyTestValue)
				values[0].Properties["expr"] = "changed"
				values[0].Values[0] = "changed"
				*values[0].Position = 2
			},
		},
		{
			name:   "map of slices",
			value:  func() any { return map[string][]string{"values": {"original"}} },
			mutate: func(value any) { value.(map[string][]string)["values"][0] = "changed" },
		},
		{
			name:   "map of maps",
			value:  func() any { return map[string]map[string]string{"properties": {"expr": "original"}} },
			mutate: func(value any) { value.(map[string]map[string]string)["properties"]["expr"] = "changed" },
		},
		{
			name: "nested JSON values",
			value: func() any {
				return map[string]any{"nested": []any{nil, true, 1.5, "original", map[string]any{"expr": "original"}}}
			},
			mutate: func(value any) {
				values := value.(map[string]any)["nested"].([]any)
				values[3] = "changed"
				values[4].(map[string]any)["expr"] = "changed"
			},
		},
		{
			name:   "slice of pointers",
			value:  func() any { return []*nestedCopyTestValue{nil, {Values: []string{"original"}}} },
			mutate: func(value any) { value.([]*nestedCopyTestValue)[1].Values[0] = "changed" },
		},
		{
			name: "map of pointers",
			value: func() any {
				return map[string]*nestedCopyTestValue{"nil": nil, "value": {Values: []string{"original"}}}
			},
			mutate: func(value any) { value.(map[string]*nestedCopyTestValue)["value"].Values[0] = "changed" },
		},
		{
			name:  "nil interface overwrites populated destination",
			value: func() any { return nil },
		},
		{
			name: "nil and empty collections and interfaces",
			value: func() any {
				return map[string]any{
					"nil": nil, "nil slice": []any(nil), "empty slice": []any{},
					"nil map": map[string]any(nil), "empty map": map[string]any{},
					"nil pointer": (*nestedCopyTestValue)(nil),
				}
			},
		},
		{
			name: "times inside collections",
			value: func() any {
				return map[string][]time.Time{"times": {time.Date(2020, time.January, 1, 2, 3, 4, 5, time.UTC)}}
			},
			mutate: func(value any) { value.(map[string][]time.Time)["times"][0] = time.Time{} },
		},
	}
	copyFunctions := []struct {
		name string
		copy func(*testing.T, *TypedSpecObject[any]) *TypedSpecObject[any]
	}{
		{
			name: "CopyObject",
			copy: func(t *testing.T, source *TypedSpecObject[any]) *TypedSpecObject[any] {
				copied, ok := CopyObject(source).(*TypedSpecObject[any])
				require.True(t, ok)
				return copied
			},
		},
		{
			name: "CopyObjectInto",
			copy: func(t *testing.T, source *TypedSpecObject[any]) *TypedSpecObject[any] {
				copied := &TypedSpecObject[any]{Spec: "overwritten"}
				require.NoError(t, CopyObjectInto(copied, source))
				return copied
			},
		},
	}
	for _, copyFunction := range copyFunctions {
		t.Run(copyFunction.name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					original := &TypedSpecObject[any]{Spec: test.value()}
					copied := copyFunction.copy(t, original)
					require.Equal(t, original, copied)
					if test.mutate != nil {
						test.mutate(copied.Spec)
						assert.Equal(t, test.value(), original.Spec, "mutating the copy must not change the original")
					}
				})
			}
		})
	}
}

func BenchmarkCopyObjectNestedValues(b *testing.B) {
	position := 1
	original := &TypedSpecObject[[]nestedCopyTestValue]{Spec: []nestedCopyTestValue{{
		Properties: map[string]any{"expr": "original", "nested": []any{map[string]any{"value": "original"}}},
		Values:     []string{"original"},
		Position:   &position,
	}}}
	b.ReportAllocs()
	for b.Loop() {
		if CopyObject(original) == nil {
			b.Fatal("copy failed")
		}
	}
}
