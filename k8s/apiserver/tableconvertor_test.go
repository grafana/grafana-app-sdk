package apiserver

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/grafana/grafana-app-sdk/resource"
)

type testSpec struct {
	StringField string `json:"stringField"`
	IntField    int    `json:"intField"`
}

func newTestObj(name, rv string, spec testSpec) *resource.TypedSpecObject[testSpec] {
	obj := &resource.TypedSpecObject[testSpec]{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: rv,
		},
		Spec: spec,
	}
	return obj
}

// testSpecList implements runtime.Object for a list of TypedSpecObject[testSpec].
type testSpecList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []resource.TypedSpecObject[testSpec] `json:"items"`
}

func (t *testSpecList) DeepCopyObject() runtime.Object {
	clone := *t
	clone.Items = make([]resource.TypedSpecObject[testSpec], len(t.Items))
	copy(clone.Items, t.Items)
	return &clone
}

func TestNewTableConvertor(t *testing.T) {
	t.Run("empty columns", func(t *testing.T) {
		tc := newTableConvertor(nil)
		require.NotNil(t, tc)

		conv := tc.(*additionalColumnsTableConvertor)
		assert.Len(t, conv.headers, 1)
		assert.Equal(t, "Name", conv.headers[0].Name)
	})

	t.Run("valid columns", func(t *testing.T) {
		tc := newTableConvertor([]resource.TableColumn{
			{
				Name: "String Field", Type: "string", JSONPath: ".spec.stringField",
				ValueFunc: func(o resource.Object) (any, error) {
					return o.(*resource.TypedSpecObject[testSpec]).Spec.StringField, nil
				},
			},
			{
				Name: "Count", Type: "integer", JSONPath: ".spec.intField",
				Priority: 1, Description: "The count",
				ValueFunc: func(o resource.Object) (any, error) {
					return o.(*resource.TypedSpecObject[testSpec]).Spec.IntField, nil
				},
			},
		})
		require.NotNil(t, tc)

		conv := tc.(*additionalColumnsTableConvertor)
		assert.Len(t, conv.headers, 3)
		assert.Equal(t, "Name", conv.headers[0].Name)
		assert.Equal(t, "String Field", conv.headers[1].Name)
		assert.Equal(t, "string", conv.headers[1].Type)
		assert.Equal(t, "Count", conv.headers[2].Name)
		assert.Equal(t, "integer", conv.headers[2].Type)
		assert.Equal(t, int32(1), conv.headers[2].Priority)
		assert.Equal(t, "The count", conv.headers[2].Description)
	})
}

func TestConvertToTable_SingleObject(t *testing.T) {
	tc := newTableConvertor([]resource.TableColumn{
		{
			Name: "String Field", Type: "string", JSONPath: ".spec.stringField",
			ValueFunc: func(o resource.Object) (any, error) {
				return o.(*resource.TypedSpecObject[testSpec]).Spec.StringField, nil
			},
		},
		{
			Name: "Int Field", Type: "integer", JSONPath: ".spec.intField",
			ValueFunc: func(o resource.Object) (any, error) {
				return o.(*resource.TypedSpecObject[testSpec]).Spec.IntField, nil
			},
		},
	})

	obj := newTestObj("test-obj", "123", testSpec{StringField: "hello", IntField: 42})

	table, err := tc.ConvertToTable(context.Background(), obj, &metav1.TableOptions{})
	require.NoError(t, err)
	require.NotNil(t, table)

	assert.Len(t, table.ColumnDefinitions, 3)
	assert.Equal(t, "Name", table.ColumnDefinitions[0].Name)
	assert.Equal(t, "String Field", table.ColumnDefinitions[1].Name)
	assert.Equal(t, "Int Field", table.ColumnDefinitions[2].Name)

	require.Len(t, table.Rows, 1)
	cells := table.Rows[0].Cells
	assert.Equal(t, "test-obj", cells[0])
	assert.Equal(t, "hello", cells[1])
	assert.Equal(t, 42, cells[2])
}

func TestConvertToTable_NoHeaders(t *testing.T) {
	tc := newTableConvertor([]resource.TableColumn{
		{
			Name: "Field", Type: "string", JSONPath: ".spec.stringField",
			ValueFunc: func(o resource.Object) (any, error) {
				return o.(*resource.TypedSpecObject[testSpec]).Spec.StringField, nil
			},
		},
	})

	obj := newTestObj("test", "", testSpec{StringField: "val"})

	table, err := tc.ConvertToTable(context.Background(), obj, &metav1.TableOptions{NoHeaders: true})
	require.NoError(t, err)
	assert.Empty(t, table.ColumnDefinitions)
	require.Len(t, table.Rows, 1)
	assert.Equal(t, "test", table.Rows[0].Cells[0])
}

func TestConvertToTable_ValueFuncError(t *testing.T) {
	tc := newTableConvertor([]resource.TableColumn{
		{
			Name: "Failing", Type: "string", JSONPath: ".spec.field",
			ValueFunc: func(_ resource.Object) (any, error) {
				return nil, errors.New("value extraction failed")
			},
		},
	})

	obj := newTestObj("test", "", testSpec{})

	table, err := tc.ConvertToTable(context.Background(), obj, &metav1.TableOptions{})
	require.NoError(t, err)
	require.Len(t, table.Rows, 1)
	assert.Equal(t, "test", table.Rows[0].Cells[0])
	assert.Nil(t, table.Rows[0].Cells[1])
}

func TestConvertToTable_NilValue(t *testing.T) {
	tc := newTableConvertor([]resource.TableColumn{
		{
			Name: "Optional", Type: "string", JSONPath: ".spec.optional",
			ValueFunc: func(_ resource.Object) (any, error) {
				return nil, nil
			},
		},
	})

	obj := newTestObj("test", "", testSpec{})

	table, err := tc.ConvertToTable(context.Background(), obj, &metav1.TableOptions{})
	require.NoError(t, err)
	require.Len(t, table.Rows, 1)
	assert.Equal(t, "test", table.Rows[0].Cells[0])
	assert.Nil(t, table.Rows[0].Cells[1])
}

func TestConvertToTable_ResourceVersionPropagation(t *testing.T) {
	tc := newTableConvertor(nil)

	obj := newTestObj("test", "456", testSpec{})

	table, err := tc.ConvertToTable(context.Background(), obj, &metav1.TableOptions{})
	require.NoError(t, err)
	assert.Equal(t, "456", table.ResourceVersion)
}

func TestConvertToTable_List(t *testing.T) {
	tc := newTableConvertor([]resource.TableColumn{
		{
			Name: "String Field", Type: "string", JSONPath: ".spec.stringField",
			ValueFunc: func(o resource.Object) (any, error) {
				return o.(*resource.TypedSpecObject[testSpec]).Spec.StringField, nil
			},
		},
		{
			Name: "Int Field", Type: "integer", JSONPath: ".spec.intField",
			ValueFunc: func(o resource.Object) (any, error) {
				return o.(*resource.TypedSpecObject[testSpec]).Spec.IntField, nil
			},
		},
	})

	list := &testSpecList{
		ListMeta: metav1.ListMeta{
			ResourceVersion: "789",
		},
		Items: []resource.TypedSpecObject[testSpec]{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "obj-1"},
				Spec:       testSpec{StringField: "alpha", IntField: 1},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "obj-2"},
				Spec:       testSpec{StringField: "beta", IntField: 2},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "obj-3"},
				Spec:       testSpec{StringField: "gamma", IntField: 3},
			},
		},
	}

	table, err := tc.ConvertToTable(context.Background(), list, &metav1.TableOptions{})
	require.NoError(t, err)
	require.NotNil(t, table)

	assert.Equal(t, "789", table.ResourceVersion)
	assert.Len(t, table.ColumnDefinitions, 3)

	require.Len(t, table.Rows, 3)
	for i, expected := range []struct {
		name        string
		stringField string
		intField    int
	}{
		{"obj-1", "alpha", 1},
		{"obj-2", "beta", 2},
		{"obj-3", "gamma", 3},
	} {
		cells := table.Rows[i].Cells
		assert.Equal(t, expected.name, cells[0], "row %d name", i)
		assert.Equal(t, expected.stringField, cells[1], "row %d string field", i)
		assert.Equal(t, expected.intField, cells[2], "row %d int field", i)
	}
}
