package main

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelResource "go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func SetTraceProvider(cfg OpenTelemetryConfig) error {
	var err error
	var exp trace.SpanExporter
	switch cfg.ConnType {
	case ConnTypeGRPC:
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn, err := grpc.DialContext(ctx, fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			// Note the use of insecure transport here. TLS is recommended in production.
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		if err != nil {
			return fmt.Errorf("failed to create gRPC connection to collector: %w", err)
		}

		// Set up a trace exporter
		exp, err = otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	case ConnTypeHTTP:
		// TODO: better?
		exp, err = otlptracehttp.New(context.Background())
	}
	if err != nil {
		return err
	}

	// Ensure default SDK resources and the required service name are set.
	r, err := otelResource.New(
		context.Background(),
		otelResource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
		otelResource.WithProcessRuntimeDescription(),
		otelResource.WithTelemetrySDK(),
	)

	if err != nil {
		return err
	}

	otel.SetTracerProvider(trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(r),
	))
	return nil
}
