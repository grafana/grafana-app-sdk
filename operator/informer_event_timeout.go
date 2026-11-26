package operator

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type eventTimeoutContextKey struct{}

type eventTimeout struct {
	parent   context.Context
	duration time.Duration
}

func informerEventTimeoutDuration(ctx context.Context) time.Duration {
	if budget, ok := ctx.Value(eventTimeoutContextKey{}).(eventTimeout); ok {
		return budget.duration
	}
	return 0
}

func withInformerEventTimeout(ctx context.Context, timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel := informerEventContext(ctx, timeout)
	defer cancel()
	return fn(ctx)
}

// Deferred deliveries start a fresh timeout after the stale retries finish.
// A zero timeout keeps the deadline already supplied by the informer.
func informerEventContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	budget := eventTimeout{parent: ctx, duration: timeout}
	attempt, cancel := context.WithTimeout(ctx, timeout)
	return context.WithValue(attempt, eventTimeoutContextKey{}, budget), cancel
}

func retryInformerEvent(ctx, live context.Context, fn func(context.Context) (*time.Duration, error)) func() (*time.Duration, error) {
	return func() (*time.Duration, error) {
		if live != nil && live.Err() != nil {
			return nil, nil
		}
		retryCtx := ctx
		var cancel context.CancelFunc
		// The first attempt's deadline is over. Use the informer run context
		// so shutdown still cancels the retry and delivery values survive.
		if budget, ok := ctx.Value(eventTimeoutContextKey{}).(eventTimeout); ok {
			if err := budget.parent.Err(); err != nil {
				return nil, err
			}
			parent := trace.ContextWithSpan(budget.parent, trace.SpanFromContext(ctx))
			retryCtx, cancel = context.WithTimeout(parent, budget.duration)
			defer cancel()
		} else if live != nil && ctx.Err() != nil {
			retryCtx = trace.ContextWithSpan(live, trace.SpanFromContext(ctx))
		} else if live != nil {
			retryCtx, cancel = context.WithCancel(ctx)
			defer cancel()
		}
		if live != nil && cancel != nil {
			// The attempt's parent is the informer or event context. Removing
			// the finalizer owner must also cancel the in-flight callback.
			stop := context.AfterFunc(live, cancel)
			defer stop()
		}
		retryCtx, span := GetTracer().Start(retryCtx, "controller-retry")
		defer span.End()
		return fn(retryCtx)
	}
}
