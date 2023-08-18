package k8s

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

// SetTracer sets the tracer used for generating spans for this package
func SetTracer(t trace.Tracer) {
	tracer = t
}

// GetTracer returns the trace.Tracer set by SetTracer, or a tracer generated from
// otel.GetTracerProvider().Tracer("k8s") if none has been set.
func GetTracer() trace.Tracer {
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer("k8s")
	}
	return tracer
}

// TODO: metrics, logging interface
