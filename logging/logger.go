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
	// Debug logs a message at the DEBUG level, with optional arguments as a sequence of key/value pairs
	// (e.g. Debug("message", "key1", "val1", "key2", "val2"))
	Debug(msg string, args ...any)
	// Info logs a message at the INFO level, with optional arguments as a sequence of key/value pairs
	// (e.g. Info("message", "key1", "val1", "key2", "val2"))
	Info(msg string, args ...any)
	// Warn logs a message at the WARN level, with optional arguments as a sequence of key/value pairs
	// (e.g. Warn("message", "key1", "val1", "key2", "val2"))
	Warn(msg string, args ...any)
	// Error logs a message at the ERROR level, with optional arguments as a sequence of key/value pairs
	// (e.g. Error("message", "key1", "val1", "key2", "val2"))
	Error(msg string, args ...any)
}

// Logger extends the BasicLogger interface with methods that accept a context as well as the message and key/val args.
type Logger interface {
	BasicLogger
	// DebugContext logs a message at the DEBUG level, and provides the context for processing as part of log handling.
	// Generally, when a context is present, prefer DebugContext to Debug.
	// Optional arguments can be passed along with the message and context as a sequence of key/value pairs
	// (e.g. DebugContext(ctx, "message", "key1", "val1", "key2", "val2"))
	DebugContext(ctx context.Context, msg string, args ...any)
	// InfoContext logs a message at the INFO level, and provides the context for processing as part of log handling.
	// Generally, when a context is present, prefer InfoContext to Info.
	// Optional arguments can be passed along with the message and context as a sequence of key/value pairs
	// (e.g. InfoContext(ctx, "message", "key1", "val1", "key2", "val2"))
	InfoContext(ctx context.Context, msg string, args ...any)
	// WarnContext logs a message at the WARN level, and provides the context for processing as part of log handling.
	// Generally, when a context is present, prefer WarnContext to Warn.
	// Optional arguments can be passed along with the message and context as a sequence of key/value pairs
	// (e.g. WarnContext(ctx, "message", "key1", "val1", "key2", "val2"))
	WarnContext(ctx context.Context, msg string, args ...any)
	// ErrorContext logs a message at the ERROR level, and provides the context for processing as part of log handling.
	// Generally, when a context is present, prefer ErrorContext to Debug.
	// Optional arguments can be passed along with the message and context as a sequence of key/value pairs
	// (e.g. ErrorContext(ctx, "message", "key1", "val1", "key2", "val2"))
	ErrorContext(ctx context.Context, msg string, args ...any)
	// With returns a Logger with the supplied key/value pair arguments attached to any messages it logs.
	// This is syntactically equivalent to adding args to every call to a log method on the logger.
	With(args ...any) Logger
}

// BasicLoggerWrapper wraps BasicLogger with Logger methods that drop the context and call the corresponding Logger methods instead
type BasicLoggerWrapper struct {
	BasicLogger
	attrs []any
}

func (l *BasicLoggerWrapper) DebugContext(ctx context.Context, msg string, args ...any) {
	l.Debug(msg, append(l.attrs, args...)...)
}
func (l *BasicLoggerWrapper) InfoContext(ctx context.Context, msg string, args ...any) {
	l.Info(msg, append(l.attrs, args...)...)
}
func (l *BasicLoggerWrapper) WarnContext(ctx context.Context, msg string, args ...any) {
	l.Warn(msg, append(l.attrs, args...)...)
}
func (l *BasicLoggerWrapper) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.Error(msg, append(l.attrs, args...)...)
}
func (l *BasicLoggerWrapper) With(args ...any) Logger {
	return &BasicLoggerWrapper{
		BasicLogger: l.BasicLogger,
		attrs:       args,
	}
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
func (n *NoOpLogger) With(...any) Logger {
	return n
}

var (
	_ Logger = &NoOpLogger{}
	_ Logger = &BasicLoggerWrapper{}
)
