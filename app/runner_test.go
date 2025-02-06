package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiRunner_Run(t *testing.T) {
	t.Run("early exit, one runner", func(t *testing.T) {
		r := NewMultiRunner()
		r.AddRunnable(&testRunnable{}) // Immediate exit on Run
		err := runOrTimeout(context.Background(), &testRunnable{}, time.Second*5)
		assert.Nil(t, err)
	})

	t.Run("early exit, multiple runners", func(t *testing.T) {
		r := NewMultiRunner()
		r.AddRunnable(&testRunnable{}) // Immediate exit on Run
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				time.Sleep(time.Second * 10)
				<-ctx.Done()
				return errors.New("error")
			},
		})
		err := runOrTimeout(context.Background(), r, time.Second*5)
		assert.Equal(t, testTimeoutError, err)
	})

	t.Run("context canceled", func(t *testing.T) {
		r := NewMultiRunner()
		ch := make(chan struct{})
		r.AddRunnable(&testRunnable{}) // Immediate exit on Run
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				ch <- struct{}{}
				<-ctx.Done()
				return errors.New("error") // while this error is returned, the context was canceled so MultiRunner won't return it
			},
		})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-ch
			cancel()
		}()
		err := runOrTimeout(ctx, r, time.Second*5)
		assert.Nil(t, err)
	})

	t.Run("context canceled, wait timeout exceeded", func(t *testing.T) {
		r := NewMultiRunner()
		timeout := time.Second
		r.ExitWait = &timeout
		ch := make(chan struct{})
		r.AddRunnable(&testRunnable{}) // Immediate exit on Run
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				ch <- struct{}{}
				time.Sleep(time.Minute)
				<-ctx.Done()
				return nil
			},
		})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-ch
			cancel()
		}()
		err := runOrTimeout(ctx, r, time.Second*5)
		require.NotNil(t, err)
		assert.Equal(t, "exit wait time exceeded waiting for Runners to complete", err.Error())
	})

	t.Run("context timeout", func(t *testing.T) {
		r := NewMultiRunner()
		r.AddRunnable(&testRunnable{}) // Immediate exit on Run
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				return errors.New("error") // while this error is returned, the context was canceled so MultiRunner won't return it
			},
		})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := runOrTimeout(ctx, r, time.Second*5)
		assert.Nil(t, err)
	})

	t.Run("runner error, continue", func(t *testing.T) {
		r := NewMultiRunner()
		errorHandled := false
		runnerError := errors.New("run error")
		r.ErrorHandler = func(ctx context.Context, err error) bool {
			assert.Equal(t, runnerError, err)
			errorHandled = true
			return false
		}
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				return runnerError
			},
		})
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				return errors.New("error") // while this error is returned, the context was canceled so MultiRunner won't return it
			},
		})
		err := runOrTimeout(context.Background(), r, time.Second*5)
		assert.Equal(t, testTimeoutError, err)
		assert.True(t, errorHandled)
	})

	t.Run("runner error, stop", func(t *testing.T) {
		r := NewMultiRunner()
		errorHandled := false
		runnerError := errors.New("run error")
		r.ErrorHandler = func(ctx context.Context, err error) bool {
			assert.Equal(t, runnerError, err)
			errorHandled = true
			return true
		}
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				return runnerError
			},
		})
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				return errors.New("error") // while this error is returned, the context was canceled so MultiRunner won't return it
			},
		})
		err := runOrTimeout(context.Background(), r, time.Second*5)
		assert.Equal(t, runnerError, err)
		assert.True(t, errorHandled)
	})

	t.Run("runner error, wait timeout exceeded", func(t *testing.T) {
		r := NewMultiRunner()
		timeout := time.Second
		r.ExitWait = &timeout
		errorHandled := false
		runnerError := errors.New("run error")
		r.ErrorHandler = func(ctx context.Context, err error) bool {
			assert.Equal(t, runnerError, err)
			errorHandled = true
			return true
		}
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				return runnerError
			},
		})
		r.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				time.Sleep(time.Minute)
				<-ctx.Done()
				return errors.New("error") // while this error is returned, the context was canceled so MultiRunner won't return it
			},
		})
		err := runOrTimeout(context.Background(), r, time.Second*5)
		require.NotNil(t, err)
		assert.True(t, errors.Is(err, ErrRunnerExitTimeout))
		assert.Equal(t, "exit wait time exceeded waiting for Runners to complete\nrun error", err.Error())
		assert.True(t, errorHandled)
	})
}

func TestMultiRunner_PrometheusCollectors(t *testing.T) {
	runner := NewMultiRunner()
	collectors := []prometheus.Collector{
		prometheus.NewCounter(prometheus.CounterOpts{}),
		prometheus.NewGauge(prometheus.GaugeOpts{}),
		prometheus.NewHistogram(prometheus.HistogramOpts{}),
	}
	for _, collector := range collectors {
		runner.AddRunnable(&testRunnable{
			Collectors: []prometheus.Collector{collector},
		})
	}
	assert.ElementsMatch(t, collectors, runner.PrometheusCollectors())
}

func TestSingletonRunner_Run(t *testing.T) {
	t.Run("single run, cancel", func(t *testing.T) {
		// This should behave like a normal Runnable
		runner := NewSingletonRunner(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
		}, false)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(time.Second)
			cancel()
		}()
		err := runOrTimeout(ctx, runner, time.Second*5)
		assert.Nil(t, err)
	})

	t.Run("multiple runs, one cancel, StopOnAny=false", func(t *testing.T) {
		runner := NewSingletonRunner(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
		}, false)
		wg := &sync.WaitGroup{}
		wg.Add(2)
		go func() {
			// This one isn't ever canceled, so it'll run until it hits the timeout
			err := runOrTimeout(context.Background(), runner, time.Second*5)
			assert.Equal(t, testTimeoutError, err)
			wg.Done()
		}()
		go func() {
			// We cancel this run 1 second in, so it should exit then
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(time.Second)
				cancel()
			}()
			err := runOrTimeout(ctx, runner, time.Second*5)
			assert.Nil(t, err)
			wg.Done()
		}()
		wg.Wait()
	})

	t.Run("multiple runs, once cancel, StopOnAny=true", func(t *testing.T) {
		runner := NewSingletonRunner(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
		}, true)
		wg := &sync.WaitGroup{}
		wg.Add(2)
		go func() {
			// This one isn't ever canceled, but with StopOnAny=true, the other call to Run() being canceled should cancel this
			err := runOrTimeout(context.Background(), runner, time.Second*5)
			assert.Equal(t, ErrOtherRunStopped, err)
			wg.Done()
		}()
		go func() {
			// We cancel this run 1 second in, so it should exit then
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(time.Second)
				cancel()
			}()
			err := runOrTimeout(ctx, runner, time.Second*5)
			assert.Nil(t, err)
			wg.Done()
		}()
		wg.Wait()
	})
}

func TestSingletonRunner_PrometheusCollectors(t *testing.T) {
	collectors := []prometheus.Collector{
		prometheus.NewCounter(prometheus.CounterOpts{}),
		prometheus.NewGauge(prometheus.GaugeOpts{}),
		prometheus.NewHistogram(prometheus.HistogramOpts{}),
	}
	runner := NewSingletonRunner(&testRunnable{
		Collectors: collectors,
	}, false)
	assert.ElementsMatch(t, collectors, runner.PrometheusCollectors())
}

func TestDynamicMultiRunner_Run(t *testing.T) {
	t.Run("add runner while running", func(t *testing.T) {
		runner := NewDynamicMultiRunner()
		wg := &sync.WaitGroup{}
		wg.Add(1)
		runner.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				wg.Done()
				<-ctx.Done()
				return nil
			},
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		wg2 := &sync.WaitGroup{}
		wg2.Add(1)
		go func() {
			err := runner.Run(ctx)
			assert.Nil(t, err)
			wg2.Done()
		}()
		// Verify that the first runner is running before we add a second runner
		require.False(t, waitOrTimeout(wg, time.Second*5), "timed out waiting for first runnable to run")
		// Add a second runner and make sure it also gets run
		wg.Add(1)
		runner.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				wg.Done()
				<-ctx.Done()
				return nil
			},
		})
		require.False(t, waitOrTimeout(wg, time.Second*5), "timed out waiting for second runnable to run")
		cancel()
		require.False(t, waitOrTimeout(wg2, time.Second*5), "timed out waiting for runner to exit")
	})

	t.Run("remove runner while running", func(t *testing.T) {
		runner := NewDynamicMultiRunner()
		started := make(chan struct{})
		defer close(started)
		ended := make(chan struct{})
		defer close(ended)
		runner1 := &testRunnable{
			RunFunc: func(ctx context.Context) error {
				started <- struct{}{}
				<-ctx.Done()
				ended <- struct{}{}
				return nil
			},
		}
		runner2Running := false
		runner2 := &testRunnable{
			RunFunc: func(ctx context.Context) error {
				runner2Running = true
				<-ctx.Done()
				runner2Running = false
				return nil
			},
		}
		runner.AddRunnable(runner1)
		runner.AddRunnable(runner2)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			err := runner.Run(ctx)
			assert.Nil(t, err)
			wg.Done()
		}()
		// Wait for runner1 to start
		require.False(t, waitForMessageOrTimeout(started, time.Second*5), "timed out waiting for runnable to run")
		// Remove the runner (this should also cancel runner1's context)
		runner.RemoveRunnable(runner1)
		require.False(t, waitForMessageOrTimeout(ended, time.Second*5), "timed out waiting for runnable to end")
		// runner2 should still be running
		assert.True(t, runner2Running, "runner2 stopped when runner1 was removed")
		cancel()
		require.False(t, waitOrTimeout(wg, time.Second*5), "timed out waiting for runner to exit")
	})

	t.Run("handle error without stopping other runners", func(t *testing.T) {
		runner := NewDynamicMultiRunner()
		runner1Running := false
		started := make(chan struct{})
		runner.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				started <- struct{}{}
				runner1Running = true
				<-ctx.Done()
				runner1Running = false
				return nil
			},
		})
		errCh := make(chan error)
		defer close(errCh)
		runner.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				return <-errCh
			},
		})
		myErr := fmt.Errorf("my error")
		runner.ErrorHandler = func(ctx context.Context, err error) bool {
			assert.Equal(t, myErr, err)
			return false
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			err := runner.Run(ctx)
			assert.Nil(t, err)
			wg.Done()
		}()
		// Wait for the runner to be started
		require.False(t, waitForMessageOrTimeout(started, time.Second*5), "timed out waiting for runnable to run")
		require.True(t, runner1Running)
		// Return an error from runner 1
		errCh <- myErr
		// Wait a second to make sure the other runner doesn't exit
		time.Sleep(time.Second)
		assert.True(t, runner1Running)
		cancel()
		require.False(t, waitOrTimeout(wg, time.Second*5), "timed out waiting for runner to exit")
	})

	t.Run("handle error and stop all other runners", func(t *testing.T) {
		runner := NewDynamicMultiRunner()
		started := make(chan struct{})
		defer close(started)
		ended := make(chan struct{})
		defer close(ended)
		runner.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				started <- struct{}{}
				<-ctx.Done()
				ended <- struct{}{}
				return nil
			},
		})
		errCh := make(chan error)
		defer close(errCh)
		runner.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				return <-errCh
			},
		})
		myErr := fmt.Errorf("my error")
		runner.ErrorHandler = func(ctx context.Context, err error) bool {
			assert.Equal(t, myErr, err)
			return true
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			err := runner.Run(ctx)
			assert.Equal(t, myErr, err)
			wg.Done()
		}()
		// Wait for the runner to be started
		require.False(t, waitForMessageOrTimeout(started, time.Second*5), "timed out waiting for runnable to run")
		// Return an error from runner 1
		errCh <- myErr
		// Wait for the other runner to exit (or timeout if it doesn't)
		require.False(t, waitForMessageOrTimeout(ended, time.Second*5), "timed out waiting for runnable to exit")
		cancel()
		require.False(t, waitOrTimeout(wg, time.Second*5), "timed out waiting for runner to exit")
	})

	t.Run("timeout waiting for runner to complete", func(t *testing.T) {
		runner := NewDynamicMultiRunner()
		runnerCtx, runnerCancel := context.WithCancel(context.Background())
		defer runnerCancel()
		started := make(chan struct{})
		defer close(started)
		runner.AddRunnable(&testRunnable{
			RunFunc: func(ctx context.Context) error {
				started <- struct{}{}
				<-runnerCtx.Done() // Wait for a different context so we don't exit on ctx.Done()
				return nil
			},
		})
		timeout := time.Second
		runner.ExitWait = &timeout
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			err := runner.Run(ctx)
			assert.Equal(t, ErrRunnerExitTimeout, err)
			wg.Done()
		}()
		require.False(t, waitForMessageOrTimeout(started, timeout), "timed out waiting for runnable to start")
		cancel()
		require.False(t, waitOrTimeout(wg, time.Second*5), "timed out waiting for runner to exit")
	})
}

type testRunnable struct {
	RunFunc    func(ctx context.Context) error
	Collectors []prometheus.Collector
}

func (t *testRunnable) Run(ctx context.Context) error {
	if t.RunFunc != nil {
		return t.RunFunc(ctx)
	}
	return nil
}

func (t *testRunnable) PrometheusCollectors() []prometheus.Collector {
	return t.Collectors
}

var testTimeoutError = errors.New("test timeout error")

func runOrTimeout(ctx context.Context, runnable Runnable, timeout time.Duration) error {
	var err error
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopCh := make(chan struct{})
	go func() {
		err = runnable.Run(childCtx)
		stopCh <- struct{}{}
	}()
	timer := time.NewTimer(timeout)
	select {
	case <-stopCh:
		timer.Stop()
	case <-timer.C:
		timer.Stop()
		err = testTimeoutError
	}
	return err
}

func waitForMessageOrTimeout(ch <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	select {
	case <-ch:
		return false
	case <-timer.C:
		return true
	}
}
