package operator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/grafana-app-sdk/resource"
)

func TestFakeWatcher_Add(t *testing.T) {
	w := SimpleWatcher{}
	c := context.TODO()
	obj := &resource.TypedSpecObject[string]{
		Spec: "foo",
	}

	t.Run("nil AddFunc", func(t *testing.T) {
		err := w.Add(c, obj)
		assert.Nil(t, err)
	})

	t.Run("AddFunc error", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		w.AddFunc = func(ctx context.Context, object resource.Object) error {
			assert.Equal(t, c, ctx)
			assert.Equal(t, obj, object)
			return expected
		}
		err := w.Add(c, obj)
		assert.Equal(t, expected, err)
	})

	t.Run("AddFunc no error", func(t *testing.T) {
		w.AddFunc = func(ctx context.Context, object resource.Object) error {
			assert.Equal(t, c, ctx)
			assert.Equal(t, obj, object)
			return nil
		}
		err := w.Add(c, obj)
		assert.Nil(t, err)
	})
}

func TestFakeWatcher_Update(t *testing.T) {
	w := SimpleWatcher{}
	c := context.TODO()
	old := &resource.TypedSpecObject[string]{
		Spec: "foo",
	}
	new := &resource.TypedSpecObject[string]{
		Spec: "bar",
	}

	t.Run("nil UpdateFunc", func(t *testing.T) {
		err := w.Update(c, old, new)
		assert.Nil(t, err)
	})

	t.Run("UpdateFunc error", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		w.UpdateFunc = func(ctx context.Context, object resource.Object, object2 resource.Object) error {
			assert.Equal(t, c, ctx)
			assert.Equal(t, old, object)
			assert.Equal(t, new, object2)
			return expected
		}
		err := w.Update(c, old, new)
		assert.Equal(t, expected, err)
	})

	t.Run("AddFunc no error", func(t *testing.T) {
		w.UpdateFunc = func(ctx context.Context, object resource.Object, object2 resource.Object) error {
			assert.Equal(t, c, ctx)
			assert.Equal(t, old, object)
			assert.Equal(t, new, object2)
			return nil
		}
		err := w.Update(c, old, new)
		assert.Nil(t, err)
	})
}

func TestFakeWatcher_Delete(t *testing.T) {
	w := SimpleWatcher{}
	c := context.TODO()
	obj := &resource.TypedSpecObject[string]{
		Spec: "foo",
	}

	t.Run("nil DeleteFunc", func(t *testing.T) {
		err := w.Delete(c, obj)
		assert.Nil(t, err)
	})

	t.Run("DeleteFunc error", func(t *testing.T) {
		expected := errors.New("I AM ERROR")
		w.DeleteFunc = func(ctx context.Context, object resource.Object) error {
			assert.Equal(t, c, ctx)
			assert.Equal(t, obj, object)
			return expected
		}
		err := w.Delete(c, obj)
		assert.Equal(t, expected, err)
	})

	t.Run("DeleteFunc no error", func(t *testing.T) {
		w.DeleteFunc = func(ctx context.Context, object resource.Object) error {
			assert.Equal(t, c, ctx)
			assert.Equal(t, obj, object)
			return nil
		}
		err := w.Delete(c, obj)
		assert.Nil(t, err)
	})
}
