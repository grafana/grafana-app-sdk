package operator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
	clientfeatures "k8s.io/client-go/features"
)

func TestMain(m *testing.M) {
	// Have to turn off WatchListClient because it doesn't use the custom cache, and the CustomCacheInformer tests fail
	// TODO: @IfSentient is this intentional? reflector.go constructs an in-memory store in watchList that is uses instead of the provided store
	os.Setenv(fmt.Sprintf("KUBE_FEATURE_%s", clientfeatures.WatchListClient), "false")

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
	var expectedErr = errors.New("I AM ERROR")

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
				return expectedErr
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := o.Run(ctx)
		assert.Equal(t, expectedErr, err)
		_, open := <-done
		assert.False(t, open)
	})

	t.Run("two failing controllers don't leak goroutines", func(t *testing.T) {
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
		assert.Equal(t, expectedErr, err)
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
