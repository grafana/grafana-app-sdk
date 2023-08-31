package logging

import (
	"context"

	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// NewSLogLogger creates a new SLogLogger which wraps an *slog.Logger that has a handler to always add a trace ID
// to the log messages if the context is provided in the log call (e.g. InfoContext())
func NewSLogLogger(handler slog.Handler) *SLogLogger {
	return &SLogLogger{
		Logger: *slog.New(&traceIDHandler{next: handler}),
	}
}

// SLogLogger wraps slog.Logger to override the With() method to return a Logger (*SLogLogger), rather than *slog.Logger,
// implementing the Logger interface.
type SLogLogger struct {
	slog.Logger
}

// With returns a new *SLogLogger with the provided key/value pairs attached
func (s *SLogLogger) With(args ...any) Logger {
	return &SLogLogger{
		Logger: *s.Logger.With(args...),
	}
}

// Compile-time interface compliance check
var _ Logger = &SLogLogger{}

type traceIDHandler struct {
	next slog.Handler
}

func (t *traceIDHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return t.next.Enabled(ctx, lvl)
}

func (t *traceIDHandler) Handle(ctx context.Context, rec slog.Record) error {
	if traceID := trace.SpanContextFromContext(ctx).TraceID(); traceID.IsValid() {
		rec.AddAttrs(slog.String("traceID", traceID.String()))
	}
	return t.next.Handle(ctx, rec)
}

func (t *traceIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceIDHandler{
		next: t.next.WithAttrs(attrs),
	}
}

func (t *traceIDHandler) WithGroup(name string) slog.Handler {
	return &traceIDHandler{
		next: t.next.WithGroup(name),
	}
}
