package logging

import (
	"context"
)

var (
	// DefaultLogger is the default Logger for all SDK logging, if one hasn't been provided in the context.
	DefaultLogger Logger = &NoOpLogger{}

	contextKey = loggerContextKey{}
)

type loggerContextKey struct{}

// FromContext returns the Logger set in the context with Context(), or the DefaultLogger if no Logger is set in the context.
// If DefaultLogger is nil, it returns a *NoOpLogger so that the return is always valid to call methods on without nil-checking.
func FromContext(ctx context.Context) Logger {
	l := ctx.Value(contextKey)
	if l != nil {
		if logger, ok := l.(Logger); ok {
			return logger
		}
	}
	if DefaultLogger != nil {
		return DefaultLogger
	}
	return &NoOpLogger{}
}

// Context returns a new context built from the provided context with the provided logger in it.
// The Logger added with Context() can be retrieved with FromContext()
func Context(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, contextKey, logger)
}

// BasicLogger defines a basic interface for logging, with methods to log at various levels with a message and key/val args.
// In order to be used as a Logger, a BasicLogger either needs to also implement Logger, or it can be wrapped with BasicLoggerWrapper.
type BasicLogger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Logger extends the Logger interface with methods that accept a context as well as the message and key/val args.
type Logger interface {
	BasicLogger
	DebugContext(ctx context.Context, msg string, args ...any)
	InfoContext(ctx context.Context, msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}

// BasicLoggerWrapper wraps Logger with ContextLogger methods that drop the context and call the corresponding Logger methods instead
type BasicLoggerWrapper struct {
	BasicLogger
}

func (l *BasicLoggerWrapper) DebugContext(ctx context.Context, msg string, args ...any) {
	l.Debug(msg, args)
}
func (l *BasicLoggerWrapper) InfoContext(ctx context.Context, msg string, args ...any) {
	l.Info(msg, args)
}
func (l *BasicLoggerWrapper) WarnContext(ctx context.Context, msg string, args ...any) {
	l.Warn(msg, args)
}
func (l *BasicLoggerWrapper) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.Error(msg, args)
}

// NoOpLogger is an implementation of Logger which does nothing when its methods are called
type NoOpLogger struct{}

func (n *NoOpLogger) Debug(msg string, args ...any)                             {}
func (n *NoOpLogger) Info(msg string, args ...any)                              {}
func (n *NoOpLogger) Warn(msg string, args ...any)                              {}
func (n *NoOpLogger) Error(msg string, args ...any)                             {}
func (n *NoOpLogger) DebugContext(ctx context.Context, msg string, args ...any) {}
func (n *NoOpLogger) InfoContext(ctx context.Context, msg string, args ...any)  {}
func (n *NoOpLogger) WarnContext(ctx context.Context, msg string, args ...any)  {}
func (n *NoOpLogger) ErrorContext(ctx context.Context, msg string, args ...any) {}
