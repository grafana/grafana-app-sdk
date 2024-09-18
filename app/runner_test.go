package app

import (
	"context"
	"errors"
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
		assert.Equal(t, "exit wait time exceeded waiting for Runners to complete: run error", err.Error())
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
