package operator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type mockController struct {
	RunFunc func(ctx context.Context) error
}

func (m *mockController) Run(ctx context.Context) error {
	if m.RunFunc != nil {
		return m.RunFunc(ctx)
	}
	return nil
}

func TestOperator_AddController(t *testing.T) {
	o := &Operator{}
	// Ensure an operator created without `New` doesn't panic when a controller is added
	o.AddController(&mockController{})
	// Ensure that controllers are added to the internal list
	assert.Equal(t, 1, len(o.controllers))
	o = New()
	assert.Equal(t, 0, len(o.controllers))
	o.AddController(&mockController{})
	assert.Equal(t, 1, len(o.controllers))
}

func TestOperator_Run(t *testing.T) {
	t.Run("controller run error propagates up", func(t *testing.T) {
		done := make(chan struct{}, 1)
		o := New()
		o.AddController(&mockController{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				close(done)
				return nil
			},
		})
		o.AddController(&mockController{
			RunFunc: func(_ context.Context) error {
				time.Sleep(time.Second)
				return errors.New("I AM ERROR")
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := o.Run(ctx)
		assert.Equal(t, errors.New("I AM ERROR"), err)
		_, open := <-done
		assert.False(t, open)
	})

	t.Run("two failing controllers don't leak goroutines", func(t *testing.T) {
		expectedErr := errors.New("I AM ERROR")

		o := New()
		o.AddController(&mockController{
			RunFunc: func(ctx context.Context) error {
				return expectedErr
			},
		})
		o.AddController(&mockController{
			RunFunc: func(_ context.Context) error {
				return expectedErr
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := o.Run(ctx)
		assert.Equal(t, errors.New("I AM ERROR"), err)
	})

	t.Run("stopping operator stops controllers", func(t *testing.T) {
		done := make(chan struct{}, 1)
		o := New()
		o.AddController(&mockController{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				close(done)
				return nil
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err := o.Run(ctx)
		assert.Nil(t, err)
		_, open := <-done
		assert.False(t, open)
	})
}
