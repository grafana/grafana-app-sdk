package operator

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockController struct {
	RunFunc func(<-chan struct{}) error
}

func (m *mockController) Run(ch <-chan struct{}) error {
	if m.RunFunc != nil {
		return m.RunFunc(ch)
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
			RunFunc: func(i <-chan struct{}) error {
				<-i
				close(done)
				return nil
			},
		})
		o.AddController(&mockController{
			RunFunc: func(<-chan struct{}) error {
				time.Sleep(time.Second)
				return errors.New("I AM ERROR")
			},
		})

		stopCh := make(chan struct{}, 1)

		err := o.Run(stopCh)
		assert.Equal(t, errors.New("I AM ERROR"), err)
		_, open := <-done
		assert.False(t, open)
	})

	t.Run("stopping operator stops controllers", func(t *testing.T) {
		done := make(chan struct{}, 1)
		o := New()
		o.AddController(&mockController{
			RunFunc: func(i <-chan struct{}) error {
				<-i
				close(done)
				return nil
			},
		})

		stopCh := make(chan struct{}, 1)
		go func() {
			time.Sleep(time.Second)
			close(stopCh)
		}()

		err := o.Run(stopCh)
		assert.Nil(t, err)
		_, open := <-done
		assert.False(t, open)
	})
}
