package plugin

import (
	"context"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"go.opentelemetry.io/otel/trace"
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
type PluginLogger struct {
	log.Logger
}

// DebugContext adds the traceID field to the underlying log.Logger, then calls Debug with the provided msg and args
func (p *PluginLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	p.Logger.With("traceID", trace.SpanContextFromContext(ctx).TraceID()).Debug(msg, args...)
}

// InfoContext adds the traceID field to the underlying log.Logger, then calls Info with the provided msg and args
func (p *PluginLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	p.Logger.With("traceID", trace.SpanContextFromContext(ctx).TraceID()).Info(msg, args...)
}

// WarnContext adds the traceID field to the underlying log.Logger, then calls Warn with the provided msg and args
func (p *PluginLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	p.Logger.With("traceID", trace.SpanContextFromContext(ctx).TraceID()).Warn(msg, args...)
}

// ErrorContext adds the traceID field to the underlying log.Logger, then calls Error with the provided msg and args
func (p *PluginLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	p.Logger.With("traceID", trace.SpanContextFromContext(ctx).TraceID()).Error(msg, args...)
}

// Compile-time interface compliance check
var _ logging.Logger = &PluginLogger{}
