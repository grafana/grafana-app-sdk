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

func withInformerEventTimeout(ctx context.Context, timeout time.Duration, handler ResourceWatcher, fn func(context.Context) error) error {
	if timeout <= 0 {
		return fn(ctx)
	}
	budget := eventTimeout{parent: ctx, duration: timeout}
	if _, queued := handler.(*concurrentWatcher); queued {
		// The queue carries the budget. The worker starts its deadline.
		return fn(context.WithValue(ctx, eventTimeoutContextKey{}, budget))
	}
	attempt, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return fn(context.WithValue(attempt, eventTimeoutContextKey{}, budget))
}

func retryInformerEvent(ctx context.Context, fn func(context.Context) (*time.Duration, error)) func() (*time.Duration, error) {
	return func() (*time.Duration, error) {
		retryCtx := ctx
		// The first attempt's deadline is over. Use the informer run context
		// so shutdown still cancels the retry and delivery values survive.
		if budget, ok := ctx.Value(eventTimeoutContextKey{}).(eventTimeout); ok {
			if err := budget.parent.Err(); err != nil {
				return nil, err
			}
			parent := trace.ContextWithSpan(budget.parent, trace.SpanFromContext(ctx))
			attempt, cancel := context.WithTimeout(parent, budget.duration)
			defer cancel()
			retryCtx = attempt
		}
		retryCtx, span := GetTracer().Start(retryCtx, "controller-retry")
		defer span.End()
		return fn(retryCtx)
	}
}
