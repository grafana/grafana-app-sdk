package router

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/grafana-app-sdk/logging"
)

// MiddlewareFunc is a function that receives a HandlerFunc and returns another HandlerFunc.
// This allows one to intercept incoming request, before and after the actual handler execution.
type MiddlewareFunc func(HandlerFunc) HandlerFunc

// middleware interface is anything which implements a MiddlewareFunc named Middleware.
type middleware interface {
	Middleware(handlerFunc HandlerFunc) HandlerFunc
}

// Middleware allows MiddlewareFunc to implement the middleware interface.
func (m MiddlewareFunc) Middleware(handler HandlerFunc) HandlerFunc {
	return m(handler)
}

// CapturingSender is a backend.CallResourceResponseSender that captures the sent response,
// allowing other to tweak with it and send it afterwards.
type CapturingSender struct {
	Response *backend.CallResourceResponse
}

// Send captures the response res.
func (c *CapturingSender) Send(res *backend.CallResourceResponse) error {
	c.Response = res
	return nil
}

// NextFunc is the main function to call the downstream middleware when using a capturing middleware.
type NextFunc func(ctx context.Context) *backend.CallResourceResponse

// NewCapturingMiddleware creates a middleware
// that allows one to add behavior that affects both the request and the response of the call.
func NewCapturingMiddleware(f func(ctx context.Context, r *backend.CallResourceRequest, n NextFunc)) MiddlewareFunc {
	return func(handler HandlerFunc) HandlerFunc {
		return func(ctx context.Context, req *backend.CallResourceRequest, res backend.CallResourceResponseSender) {
			cs := &CapturingSender{}

			f(ctx, req, func(ctx context.Context) *backend.CallResourceResponse {
				// Execute downstream handlers, capturing the output
				handler(ctx, req, cs)
				return cs.Response
			})

			// Note the response here is mutable,
			// so the changes performed by the actual middleware func will be propagated upstream
			_ = res.Send(cs.Response)
		}
	}
}

// NewLoggingMiddleware returns a MiddleWareFunc which logs an INFO level message for each request,
// and injects the provided Logger into the context used downstream.
func NewLoggingMiddleware(logger logging.Logger) MiddlewareFunc {
	return func(handler HandlerFunc) HandlerFunc {
		return func(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) {
			start := time.Now()
			handler(logging.Context(ctx, logger), req, sender)
			lat := time.Since(start)
			// Logging latency in ms because it's easier to perceive as a human.
			// But we also attach a separate field where latency is in seconds, for e.g. Loki queries.
			logger.InfoContext(ctx, fmt.Sprintf("%s %s %dms", req.Method, req.Path, lat.Milliseconds()),
				"request.http.method", req.Method,
				"request.http.path", req.Path,
				"request.user", req.PluginContext.User.Name,
				"request.latency", lat.Seconds(),
			)
		}
	}
}

// NewTracingMiddleware returns a MiddlewareFunc which adds a tracing span for every request which lasts
// the duration of the request's handle time and includes all attributes which are a part of
// OpenTelemetry's Semantic Conventions for HTTP spans:
// https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-spans.md
func NewTracingMiddleware(tracer trace.Tracer) MiddlewareFunc {
	return NewCapturingMiddleware(func(ctx context.Context, req *backend.CallResourceRequest, next NextFunc) {
		ctx, span := tracer.Start(ctx, "middleware")
		defer span.End()
		routeInfo := MatchedRouteFromContext(ctx)

		resp := next(ctx)
		query := ""
		if u, err := url.Parse(req.URL); err == nil {
			query = u.RawQuery
		} else if s := strings.SplitN(req.URL, "?", 1); len(s) > 1 {
			// Fallback if URL can't be parsed
			query = s[1]
		}

		span.SetAttributes(
			attribute.Int("http.response.status_code", resp.Status),
			attribute.Int("http.request.body.size", len(req.Body)),
			attribute.Int("http.response.body.size", len(resp.Body)),
			attribute.String("http.request.method", req.Method),
			attribute.String("url.path", req.Path),
			attribute.String("url.query", query),
			attribute.String("http.route", routeInfo.Path))
	})
}
