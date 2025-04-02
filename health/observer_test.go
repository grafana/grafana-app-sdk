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

type ObserverTestOptions struct {
	name                   string
	expectedRuns           int
	runInterval            time.Duration
	waitInterval           time.Duration
	alternateFailureChecks bool
}

func TestObserverAdd(t *testing.T) {
	tt := []ObserverTestOptions{
		{
			name:                   "only test a single initial run",
			expectedRuns:           1,
			runInterval:            time.Minute * 1,
			waitInterval:           time.Second * 2,
			alternateFailureChecks: true,
		},
		{
			name:                   "test the checks after at least one initial run with alternate checks failing",
			expectedRuns:           3, // initial + 2
			runInterval:            time.Second * 1,
			waitInterval:           time.Second*2 + time.Millisecond*100,
			alternateFailureChecks: true,
		},
		{
			name:                   "test the checks after at least one initial run with all passing checks",
			expectedRuns:           3, // initial + 2
			runInterval:            time.Second * 1,
			waitInterval:           time.Second*2 + time.Millisecond*100,
			alternateFailureChecks: false,
		},
	}

	for _, tc := range tt {
		if tc.expectedRuns == 1 {
			testObserverCommon(t, tc)
		} else {
			testObserverCommon(t, tc)
		}
	}

}

func testObserverCommon(t *testing.T, options ObserverTestOptions) {
	o := NewObserver(options.runInterval)
	checks := []*testCheck{}

	numChecks := 10
	wg := sync.WaitGroup{}

	// Simulate concurrent access
	for i := 0; i < numChecks; i++ {
		wg.Add(1)
		tc := &testCheck{}
		// copy the checks here so we can assert mock expectations on it
		checks = append(checks, tc)

		if i%2 == 0 || !options.alternateFailureChecks {
			tc.On("HealthCheckName").Return("HealthCheck-pass").Times(options.expectedRuns)
			tc.On("HealthCheck", mock.Anything).Return(nil).Times(options.expectedRuns)
		} else {
			tc.On("HealthCheckName").Return("HealthCheck-fail").Times(options.expectedRuns)
			tc.On("HealthCheck", mock.Anything).Return(errors.New("test error")).Times(options.expectedRuns)
		}
		go func() {
			o.AddChecks(tc)
			wg.Done()
		}()
	}
	wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())

	time.AfterFunc(options.waitInterval, func() {
		cancel()
	})

	wg.Add(1)
	go func() {
		err := o.Run(ctx)
		assert.Equal(t, err, context.Canceled, fmt.Sprintf("%s: %s", options.name, "could not start the observer"))
		wg.Done()
	}()

	wg.Wait()

	status := o.Status()
	require.NotNil(t, status, fmt.Sprintf("%s: %s", options.name, "status shouldn't be nil if the observer was started"))
	require.Len(t, status.Results, numChecks, fmt.Sprintf("%s: %s", options.name, "number of results didn't match the number of checks added"))

	numFailed := 0
	numSuccess := 0
	for _, result := range status.Results {
		if result.Name == "HealthCheck-pass" {
			numSuccess++
			assert.Nil(t, result.Error, fmt.Sprintf("%s: %s", options.name, "error should be nil"))
		} else if result.Name == "HealthCheck-fail" {
			numFailed++
			assert.NotNil(t, result.Error, fmt.Sprintf("%s: %s", options.name, "should have received an error"))
		} else {
			assert.Fail(t, fmt.Sprintf("%s: %s", options.name, "should have either received a result with name -pass or -fail"))
		}
	}

	if options.alternateFailureChecks {
		assert.Equal(t, numChecks/2, numSuccess, fmt.Sprintf("%s: %s", options.name, "number of success checks didn't match expected half of total"))
		assert.Equal(t, numChecks/2, numFailed, fmt.Sprintf("%s: %s", options.name, "number of failed checks didn't match expected half of total"))
	} else {
		assert.Equal(t, numChecks, numSuccess, fmt.Sprintf("%s: %s", options.name, "number of success checks didn't equal all checks"))
		assert.Zero(t, numFailed, fmt.Sprintf("%s: %s", options.name, "number of failed checks didn't equal zero"))
	}

	if numFailed != 0 {
		assert.False(t, status.Successful, fmt.Sprintf("%s: %s", options.name, "overall result should be false"))
	} else {
		assert.True(t, status.Successful, fmt.Sprintf("%s: %s", options.name, "overall result should be true"))
	}

	assertMockExpectationOnChecks(t, checks, options)
}

func assertMockExpectationOnChecks(t *testing.T, checks []*testCheck, options ObserverTestOptions) {
	for i, check := range checks {
		t.Logf("Checking expectations for %s: check #%d", options.name, i)
		mock.AssertExpectationsForObjects(t, check)
	}
}
