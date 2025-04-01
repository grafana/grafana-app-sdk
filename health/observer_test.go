package health

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testCheck struct {
	mock.Mock
}

func (c *testCheck) HealthCheckName() string {
	args := c.Called()
	return args.Get(0).(string)
}

func (c *testCheck) HealthCheck(ctx context.Context) error {
	args := c.Called(ctx)
	return args.Error(0)
}

func TestObserverAdd(t *testing.T) {
	tt := []struct {
		allowSingleInitialRun bool
	}{
		{allowSingleInitialRun: true},
		{allowSingleInitialRun: false},
	}

	for _, tt := range tt {
		if tt.allowSingleInitialRun {
			testObserverCommon(t, time.Minute*1, time.Second*2, "only test a single initial run")
		} else {
			testObserverCommon(t, time.Second*1, time.Second*2, "test the checks after at least one initial run")
		}
	}

}

func testObserverCommon(t *testing.T, runInterval time.Duration, waitInterval time.Duration, name string) {
	o := NewObserver(runInterval)

	numChecks := 10
	wg := sync.WaitGroup{}

	// Simulate concurrent access
	for i := 0; i < numChecks; i++ {
		wg.Add(1)
		tc := &testCheck{}
		if i%2 == 0 {
			tc.On("HealthCheckName").Return("HealthCheck-pass")
			tc.On("HealthCheck", mock.Anything).Return(nil)
		} else {
			tc.On("HealthCheckName").Return("HealthCheck-fail")
			tc.On("HealthCheck", mock.Anything).Return(errors.New("test error"))
		}
		go func() {
			o.AddChecks(tc)
			wg.Done()
		}()
	}
	wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())

	time.AfterFunc(waitInterval, func() {
		cancel()
	})

	wg.Add(1)
	go func() {
		err := o.Run(ctx)
		assert.Equal(t, err, context.Canceled, fmt.Sprintf("%s: %s", name, "could not start the observer"))
		wg.Done()
	}()

	wg.Wait()

	status := o.Status()
	require.NotNil(t, status, fmt.Sprintf("%s: %s", name, "status shouldn't be nil if the observer was started"))
	require.Len(t, status.Results, numChecks, fmt.Sprintf("%s: %s", name, "number of results didn't match the number of checks added"))

	numFailed := 0
	numSuccess := 0
	for _, result := range status.Results {
		if result.Name == "HealthCheck-pass" {
			numSuccess++
			assert.Nil(t, result.Error, fmt.Sprintf("%s: %s", name, "error should be nil"))
		} else if result.Name == "HealthCheck-fail" {
			numFailed++
			assert.NotNil(t, result.Error, fmt.Sprintf("%s: %s", name, "should have received an error"))
		} else {
			assert.Fail(t, fmt.Sprintf("%s: %s", name, "should have either received a result with name -pass or -fail"))
		}
	}

	assert.Equal(t, numChecks/2, numSuccess, fmt.Sprintf("%s: %s", name, "number of success checks didn't match expected half of total"))
	assert.Equal(t, numChecks/2, numFailed, fmt.Sprintf("%s: %s", name, "number of failed checks didn't match expected half of total"))
}
