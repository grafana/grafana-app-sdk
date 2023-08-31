package logging

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicLoggerWrapper(t *testing.T) {
	outMsg := ""
	outArgs := []any{}
	var outLevel slog.Level
	l := BasicLoggerWrapper{
		BasicLogger: &TestBasicLogger{
			DebugFunc: func(m string, a ...any) {
				outMsg = m
				outArgs = a
				outLevel = slog.LevelDebug
			},
			InfoFunc: func(m string, a ...any) {
				outMsg = m
				outArgs = a
				outLevel = slog.LevelInfo
			},
			WarnFunc: func(m string, a ...any) {
				outMsg = m
				outArgs = a
				outLevel = slog.LevelWarn
			},
			ErrorFunc: func(m string, a ...any) {
				outMsg = m
				outArgs = a
				outLevel = slog.LevelError
			},
		},
	}

	// With
	lw := l.With("1", "2")

	// DebugContext
	l.DebugContext(context.Background(), "foo", "a", "b")
	assert.Equal(t, "foo", outMsg)
	assert.Equal(t, []any{"a", "b"}, outArgs)
	assert.Equal(t, slog.LevelDebug, outLevel)

	lw.DebugContext(context.Background(), "bar", "c", "d")
	assert.Equal(t, "bar", outMsg)
	assert.Equal(t, []any{"1", "2", "c", "d"}, outArgs)
	assert.Equal(t, slog.LevelDebug, outLevel)

	// InfoContext
	l.InfoContext(context.Background(), "baz", "e", "f")
	assert.Equal(t, "baz", outMsg)
	assert.Equal(t, []any{"e", "f"}, outArgs)
	assert.Equal(t, slog.LevelInfo, outLevel)

	lw.InfoContext(context.Background(), "foobar", "g", "h")
	assert.Equal(t, "foobar", outMsg)
	assert.Equal(t, []any{"1", "2", "g", "h"}, outArgs)
	assert.Equal(t, slog.LevelInfo, outLevel)

	// WarnContext
	l.WarnContext(context.Background(), "foobaz", "i", "j")
	assert.Equal(t, "foobaz", outMsg)
	assert.Equal(t, []any{"i", "j"}, outArgs)
	assert.Equal(t, slog.LevelWarn, outLevel)

	lw.WarnContext(context.Background(), "barfoo", "k", "l")
	assert.Equal(t, "barfoo", outMsg)
	assert.Equal(t, []any{"1", "2", "k", "l"}, outArgs)
	assert.Equal(t, slog.LevelWarn, outLevel)

	// ErrorContext
	l.ErrorContext(context.Background(), "bazfoo", "m", "n")
	assert.Equal(t, "bazfoo", outMsg)
	assert.Equal(t, []any{"m", "n"}, outArgs)
	assert.Equal(t, slog.LevelError, outLevel)

	lw.ErrorContext(context.Background(), "fb", "o", "p")
	assert.Equal(t, "fb", outMsg)
	assert.Equal(t, []any{"1", "2", "o", "p"}, outArgs)
	assert.Equal(t, slog.LevelError, outLevel)
}

func TestContext(t *testing.T) {
	ctx := context.Background()
	fromCtx := FromContext(ctx)
	assert.Equal(t, DefaultLogger, fromCtx)

	l := NewSLogLogger(slog.NewJSONHandler(os.Stdout, nil))
	ctx = Context(ctx, l)
	fromCtx = FromContext(ctx)
	assert.NotEqual(t, DefaultLogger, fromCtx)
	assert.Equal(t, l, fromCtx)
}

type TestBasicLogger struct {
	DebugFunc func(string, ...any)
	InfoFunc  func(string, ...any)
	WarnFunc  func(string, ...any)
	ErrorFunc func(string, ...any)
}

func (b *TestBasicLogger) Debug(msg string, args ...any) {
	if b.DebugFunc != nil {
		b.DebugFunc(msg, args...)
	}
}

func (b *TestBasicLogger) Info(msg string, args ...any) {
	if b.InfoFunc != nil {
		b.InfoFunc(msg, args...)
	}
}

func (b *TestBasicLogger) Warn(msg string, args ...any) {
	if b.WarnFunc != nil {
		b.WarnFunc(msg, args...)
	}
}

func (b *TestBasicLogger) Error(msg string, args ...any) {
	if b.ErrorFunc != nil {
		b.ErrorFunc(msg, args...)
	}
}
