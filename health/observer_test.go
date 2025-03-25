package health

import (
	"context"
	"sync"
	"testing"
	"time"
	// "github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCheck struct {
}

func (c *testCheck) HealthCheckName() string {
	return "test-check"
}

func (c *testCheck) HealthCheck(context.Context) error {
	return nil
}

func TestObserverAdd(t *testing.T) {
	runInterval := time.Second * 1
	o := NewObserver(runInterval)

	/* tt := []struct {
		name string
	}{
		{name: "add checks concurrently manages the internal checks slice"},
	} */

	numChecks := 10
	wg := sync.WaitGroup{}

	// Simulate concurrent access
	for i := 0; i < numChecks; i++ {
		wg.Add(1)
		go func() {
			o.AddChecks(&testCheck{})
			wg.Done()
		}()
	}
	wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())

	time.AfterFunc(runInterval*2, func() {
		cancel()
	})

	wg.Add(1)
	go func() {
		err := o.Run(ctx)
		assert.Equal(t, err, context.Canceled, "could not start the observer")
		wg.Done()
	}()

	wg.Wait()

	status := o.Status()
	require.NotNil(t, status, "status shouldn't be nil if the observer was started")
	require.Len(t, status.Results, numChecks, "number of results didn't match the number of checks added")
}
