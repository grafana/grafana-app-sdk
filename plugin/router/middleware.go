package router

import (
	"context"
	"fmt"

	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
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
			logger.InfoContext(ctx, fmt.Sprintf("Handling %s request to path %s", req.Method, req.Path),
				"request.http.method", req.Method,
				"request.http.path", req.Path,
				"request.user", req.PluginContext.User.Name)

			handler(logging.Context(ctx, logger), req, sender)
		}
	}
}
