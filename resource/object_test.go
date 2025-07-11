package resource

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ComplexTestObject struct {
	Slice        []ComplexTestObject
	Map          map[string]ComplexTestObject
	PointerMap   map[string]*ComplexTestObject
	Pointer      *ComplexTestObject
	Child        ComplexTestObjectChild
	IntPointer   *int
	SlicePointer *[]int          // Weird, but valid, types
	MapPointer   *map[string]int // Weird, but valid, types
	Timestamp    time.Time       // Times won't be copied right if you try to copy them as a struct (they'll become 0)
	TimePointer  *time.Time
}

type ComplexTestObjectChild struct {
	Next  *ComplexTestObjectChild
	Str   string
	Int32 int32
	Int64 int64
}

func TestCopyObjectInto(t *testing.T) {
	i := 2
	si := []int{1, 2, 3}
	mi := map[string]int{
		"a": 1,
		"b": 2,
	}
	var ncto *ComplexTestObject
	tm := time.Date(2020, time.January, 1, 2, 3, 4, 5, time.UTC)
	tests := []struct {
		name string
		in   any
		out  any
		err  error
	}{{
		name: "nil in",
		in:   ncto,
		out:  &ComplexTestObject{},
		err:  errors.New("in must not be nil"),
	}, {
		name: "nil out",
		in:   &ComplexTestObject{},
		out:  ncto,
		err:  errors.New("out must not be nil"),
	}, {
		name: "non-pointer types",
		in: ComplexTestObject{
			Child: ComplexTestObjectChild{
				Str:   "string",
				Int32: 42,
			},
		},
		out: ComplexTestObject{},
		err: errors.New("out must be a pointer to a struct"),
	}, {
		name: "empty source object, filled destination",
		in:   &ComplexTestObject{},
		out: &ComplexTestObject{
			Slice: []ComplexTestObject{{
				Child: ComplexTestObjectChild{
					Str: "foobar",
				},
			}},
			Map: map[string]ComplexTestObject{
				"foo": ComplexTestObject{},
			},
			PointerMap: map[string]*ComplexTestObject{},
			Pointer:    &ComplexTestObject{},
			Child: ComplexTestObjectChild{
				Next: &ComplexTestObjectChild{
					Str: "foobar",
				},
				Str:   "string",
				Int32: 42,
				Int64: 84,
			},
			IntPointer:   &i,
			SlicePointer: &si,
			MapPointer:   &mi,
		},
	}, {
		name: "complex object",
		in: &ComplexTestObject{
			Slice: []ComplexTestObject{{
				Child: ComplexTestObjectChild{
					Str: "foo",
				},
			}},
			Map: map[string]ComplexTestObject{
				"foo": ComplexTestObject{
					Child: ComplexTestObjectChild{
						Str: "bar",
					},
				},
			},
			PointerMap: map[string]*ComplexTestObject{
				"foo": &ComplexTestObject{
					Child: ComplexTestObjectChild{
						Str: "foo",
					},
				},
			},
			Pointer: &ComplexTestObject{
				Child: ComplexTestObjectChild{
					Str: "bar",
				},
			},
			Child: ComplexTestObjectChild{
				Str:   "string",
				Int32: 42,
			},
			IntPointer:   &i,
			SlicePointer: &si,
			MapPointer:   &mi,
			Timestamp:    time.Now(),
			TimePointer:  &tm,
		},
		out: &ComplexTestObject{},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := CopyObjectInto(test.out, test.in)

			if test.err != nil {
				assert.Equal(t, test.err, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, test.in, test.out)
			}
		})
	}
}

func TestCopyObject(t *testing.T) {
	full := &TypedSpecStatusObject[ComplexTestObject, ComplexTestObjectChild]{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ComplexTestObject",
			APIVersion: "foo/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
		Spec: ComplexTestObject{
			Slice: []ComplexTestObject{{
				Child: ComplexTestObjectChild{
					Str: "foo",
				},
			}},
			Map: map[string]ComplexTestObject{
				"foo": ComplexTestObject{
					Child: ComplexTestObjectChild{
						Str: "bar",
					},
				},
			},
			PointerMap: map[string]*ComplexTestObject{
				"foo": &ComplexTestObject{
					Child: ComplexTestObjectChild{
						Str: "foo",
					},
				},
			},
			Pointer: &ComplexTestObject{
				Child: ComplexTestObjectChild{
					Str: "bar",
				},
			},
			Child: ComplexTestObjectChild{
				Str:   "string",
				Int32: 42,
			},
		},
		Status: ComplexTestObjectChild{
			Str:   "foo",
			Int32: 42,
			Next: &ComplexTestObjectChild{
				Str:   "bar",
				Int64: 84,
			},
		},
	}
	tests := []struct {
		name     string
		obj      any
		expected Object
	}{{
		name:     "nil",
		obj:      nil,
		expected: nil,
	}, {
		name:     "non-object",
		obj:      &ComplexTestObject{},
		expected: nil,
	}, {
		name:     "full object",
		obj:      full,
		expected: full,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cpy := CopyObject(test.obj)
			assert.Equal(t, test.expected, cpy)
		})
	}
}

func TestCopyObjectIntoPointerCopy(t *testing.T) {
	value := onlyCallOnce("test")
	original := &testSpec{
		Value: &value,
	}

	deepCopied := original.DeepCopy()

	// points to different addresses
	assert.True(t, original.Value != deepCopied.Value, "original address: %p, deepCopied address: %p", original.Value, deepCopied.Value)

	original.Value.Consume()

	assert.NotPanics(t, func() { deepCopied.Value.Consume() }, "if they point to different addresses, this should not panic")
}

type onlyCallOnce string

func (s *onlyCallOnce) Consume() {
	if *s == "" {
		panic("consumed")
	}
	*s = ""
}

type testSpec struct {
	Value *onlyCallOnce `json:"value,omitempty"`
}

func (s *testSpec) DeepCopy() *testSpec {
	cpy := &testSpec{}
	CopyObjectInto(cpy, s)
	return cpy
}
