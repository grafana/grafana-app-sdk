package resource

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestUpdateObject(t *testing.T) {
	ctx := context.Background()
	getObject := func(name string) *UntypedObject {
		return &UntypedObject{
			TypeMeta: metav1.TypeMeta{
				Kind:       "UntypedObject",
				APIVersion: "test.grafana.app",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       "namespace",
				ResourceVersion: "1",
				Labels: map[string]string{
					"foo": "bar",
				},
			},
			Spec: map[string]any{
				"foo": "bar",
			},
		}
	}
	t.Run("nil client", func(t *testing.T) {
		resp, err := UpdateObject[Object](ctx, nil, Identifier{}, nil, UpdateOptions{})
		assert.Nil(t, resp)
		assert.EqualError(t, err, "client must not be nil")
	})
	t.Run("nil updateFunc", func(t *testing.T) {
		resp, err := UpdateObject[Object](ctx, &mockClient{}, Identifier{}, nil, UpdateOptions{})
		assert.Nil(t, resp)
		assert.EqualError(t, err, "updateFunc must not be nil")
	})
	t.Run("get error", func(t *testing.T) {
		getErr := errors.New("I AM ERROR")
		resp, err := UpdateObject[Object](ctx, &mockClient{
			GetFunc: func(context.Context, Identifier) (Object, error) {
				return nil, getErr
			},
		}, Identifier{}, func(obj Object, isRetry bool) (Object, error) {
			t.Fatalf("should not call updateFunc")
			return nil, nil
		}, UpdateOptions{})
		assert.Nil(t, resp)
		assert.EqualError(t, err, getErr.Error())
	})
	t.Run("updateFunc error", func(t *testing.T) {
		funcErr := errors.New("I AM ERROR")
		getObj := getObject("foo")
		resp, err := UpdateObject[Object](ctx, &mockClient{
			GetFunc: func(context.Context, Identifier) (Object, error) {
				return getObj, nil
			},
			UpdateIntoFunc: func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions, into Object) error {
				t.Fatalf("should not call UpdateInto")
				return nil
			},
		}, Identifier{}, func(obj Object, isRetry bool) (Object, error) {
			assert.Equal(t, getObj, obj)
			return nil, funcErr
		}, UpdateOptions{})
		assert.Nil(t, resp)
		assert.EqualError(t, err, funcErr.Error())
	})
	t.Run("missing RV after update", func(t *testing.T) {
		getObj := getObject("foo")
		resp, err := UpdateObject(ctx, &mockClient{
			GetFunc: func(context.Context, Identifier) (Object, error) {
				return getObj, nil
			},
			UpdateIntoFunc: func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions, into Object) error {
				t.Fatalf("should not call UpdateInto")
				return nil
			},
		}, Identifier{}, func(obj *UntypedObject, isRetry bool) (*UntypedObject, error) {
			assert.Equal(t, getObj, obj)
			obj.SetResourceVersion("")
			return obj, nil
		}, UpdateOptions{})
		assert.Nil(t, resp)
		assert.Equal(t, ErrMissingResourceVersion, err)
	})
	t.Run("update non-409 error", func(t *testing.T) {
		updateErr := errors.New("I AM ERROR")
		getObj := getObject("foo")
		updateIntoCalls := 0
		updateFuncCalls := 0
		resp, err := UpdateObject(ctx, &mockClient{
			GetFunc: func(context.Context, Identifier) (Object, error) {
				return getObj, nil
			},
			UpdateIntoFunc: func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions, into Object) error {
				updateIntoCalls++
				assert.Equal(t, getObj, obj)
				assert.Equal(t, "bar2", obj.GetLabels()["foo"])
				return updateErr
			},
		}, Identifier{}, func(obj *UntypedObject, isRetry bool) (*UntypedObject, error) {
			updateFuncCalls++
			assert.Equal(t, getObj, obj)
			obj.SetLabels(map[string]string{
				"foo": "bar2",
			})
			return obj, nil
		}, UpdateOptions{})
		assert.Nil(t, resp)
		assert.EqualError(t, err, updateErr.Error())
		assert.Equal(t, 1, updateFuncCalls)
		assert.Equal(t, 1, updateIntoCalls)
	})
	t.Run("update 409 (each retry)", func(t *testing.T) {
		updateErr := apierrors.NewConflict(schema.GroupResource{}, "name", errors.New("I AM ERROR"))
		getObj := getObject("foo")
		updateIntoCalls := 0
		updateFuncCalls := 0
		resp, err := UpdateObject(ctx, &mockClient{
			GetFunc: func(context.Context, Identifier) (Object, error) {
				return getObj, nil
			},
			GetIntoFunc: func(ctx context.Context, identifier Identifier, into Object) error {
				return nil
			},
			UpdateIntoFunc: func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions, into Object) error {
				updateIntoCalls++
				assert.Equal(t, getObj, obj)
				assert.Equal(t, "bar2", obj.GetLabels()["foo"])
				return updateErr
			},
		}, Identifier{}, func(obj *UntypedObject, isRetry bool) (*UntypedObject, error) {
			assert.Equal(t, updateFuncCalls > 0, isRetry)
			updateFuncCalls++
			assert.Equal(t, getObj, obj)
			obj.SetLabels(map[string]string{
				"foo": "bar2",
			})
			return obj, nil
		}, UpdateOptions{})
		assert.Nil(t, resp)
		assert.Equal(t, updateErr, err)
		assert.Equal(t, 3, updateFuncCalls)
		assert.Equal(t, 3, updateIntoCalls)
	})
	t.Run("update 409, success on retry", func(t *testing.T) {
		updateErr := apierrors.NewConflict(schema.GroupResource{}, "name", errors.New("I AM ERROR"))
		getObj := getObject("foo")
		getIntoObj := getObject("foo2")
		updatedObj := getObject("bar")
		funcObj := getObject("foobar")
		funcObj.SetLabels(map[string]string{"foo": "bar"})
		updateIntoCalls := 0
		updateFuncCalls := 0
		getCalls := 0
		getIntoCalls := 0
		resp, err := UpdateObject(ctx, &mockClient{
			GetFunc: func(context.Context, Identifier) (Object, error) {
				getCalls++
				return getObj, nil
			},
			GetIntoFunc: func(ctx context.Context, identifier Identifier, into Object) error {
				getIntoCalls++
				codec := NewJSONCodec()
				buf := &bytes.Buffer{}
				codec.Write(buf, getIntoObj)
				return codec.Read(buf, into)
			},
			UpdateIntoFunc: func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions, into Object) error {
				updateIntoCalls++
				assert.Equal(t, funcObj, obj)
				if updateIntoCalls == 2 {
					codec := NewJSONCodec()
					buf := &bytes.Buffer{}
					codec.Write(buf, updatedObj)
					return codec.Read(buf, into)
				}
				return updateErr
			},
		}, Identifier{}, func(obj *UntypedObject, isRetry bool) (*UntypedObject, error) {
			assert.Equal(t, updateFuncCalls > 0, isRetry)
			if updateFuncCalls == 0 {
				assert.Equal(t, getObj, obj)
			} else {
				assert.Equal(t, getIntoObj, obj)
			}
			updateFuncCalls++
			return funcObj, nil
		}, UpdateOptions{})
		assert.Equal(t, updatedObj, resp)
		assert.NoError(t, err)
		assert.Equal(t, 2, updateFuncCalls)
		assert.Equal(t, 2, updateIntoCalls)
	})
	t.Run("success", func(t *testing.T) {
		getObj := getObject("foo")
		getIntoObj := getObject("foo2")
		updatedObj := getObject("bar")
		updateIntoCalls := 0
		updateFuncCalls := 0
		getCalls := 0
		getIntoCalls := 0
		resp, err := UpdateObject(ctx, &mockClient{
			GetFunc: func(context.Context, Identifier) (Object, error) {
				getCalls++
				return getObj, nil
			},
			GetIntoFunc: func(ctx context.Context, identifier Identifier, into Object) error {
				getIntoCalls++
				codec := NewJSONCodec()
				buf := &bytes.Buffer{}
				codec.Write(buf, getIntoObj)
				return codec.Read(buf, into)
			},
			UpdateIntoFunc: func(ctx context.Context, identifier Identifier, obj Object, options UpdateOptions, into Object) error {
				updateIntoCalls++
				codec := NewJSONCodec()
				buf := &bytes.Buffer{}
				codec.Write(buf, updatedObj)
				return codec.Read(buf, into)
			},
		}, Identifier{}, func(obj *UntypedObject, isRetry bool) (*UntypedObject, error) {
			assert.Equal(t, updateFuncCalls > 0, isRetry)
			if updateFuncCalls == 0 {
				assert.Equal(t, getObj, obj)
			} else {
				assert.Equal(t, getIntoObj, obj)
			}
			updateFuncCalls++
			obj.SetLabels(map[string]string{
				"foo": "bar2",
			})
			return obj, nil
		}, UpdateOptions{})
		assert.Equal(t, updatedObj, resp)
		assert.NoError(t, err)
		assert.Equal(t, 1, updateFuncCalls)
		assert.Equal(t, 1, updateIntoCalls)
	})
}
