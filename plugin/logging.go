package plugin

import (
	"context"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/grafana-app-sdk/logging"
)

// NewLogger returns a new PluginLogger that wraps the provided log.Logger
func NewLogger(l log.Logger) *PluginLogger {
	return &PluginLogger{
		Logger: l,
	}
}

// PluginLogger wraps a plugin-sdk-go log.Logger with the context methods needed to implement logging.Logger,
// and automatically adds the traceID from the context to the log.Logger's args when DebugContext,
// InfoContext, WarnContext, or ErrorContext are called.
// nolint:revive
type PluginLogger struct {
	log.Logger
	ctx context.Context
}

// Debug adds the traceID field to the underlying log.Logger from the context embedded with WithContect, then calls Debug with the provided msg and args
func (p *PluginLogger) Debug(msg string, args ...any) {
	p.Logger.With(logging.TraceIDKey, trace.SpanContextFromContext(p.ctx).TraceID()).Debug(msg, args...)
}

// Info adds the traceID field to the underlying log.Logger from the context embedded with WithContect, then calls Info with the provided msg and args
func (p *PluginLogger) Info(msg string, args ...any) {
	p.Logger.With(logging.TraceIDKey, trace.SpanContextFromContext(p.ctx).TraceID()).Info(msg, args...)
}

// Warn adds the traceID field to the underlying log.Logger from the context embedded with WithContect, then calls Warn with the provided msg and args
func (p *PluginLogger) Warn(msg string, args ...any) {
	p.Logger.With(logging.TraceIDKey, trace.SpanContextFromContext(p.ctx).TraceID()).Warn(msg, args...)
}

// Error adds the traceID field to the underlying log.Logger from the context embedded with WithContect, then calls Error with the provided msg and args
func (p *PluginLogger) Error(msg string, args ...any) {
	p.Logger.With(logging.TraceIDKey, trace.SpanContextFromContext(p.ctx).TraceID()).Error(msg, args...)
}

// With returns a new Logger with the provided key/value pairs already set
func (p *PluginLogger) With(args ...any) logging.Logger {
	return &PluginLogger{
		Logger: p.Logger.With(args...),
	}
}

// WithContext returns a new Logger with the provided context embedded
func (p *PluginLogger) WithContext(ctx context.Context) logging.Logger {
	return &PluginLogger{
		Logger: p.Logger,
		ctx:    ctx,
	}
}

// Compile-time interface compliance check
var _ logging.Logger = &PluginLogger{}
