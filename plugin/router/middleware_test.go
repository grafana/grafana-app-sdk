package router_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/grafana/grafana-app-sdk/plugin/router"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"
)

type ctxKey struct{}

func TestMiddleware_InOrderExecution(t *testing.T) {
	r := router.NewRouter()
	middleware1 := router.MiddlewareFunc(func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx context.Context, req *backend.CallResourceRequest, res backend.CallResourceResponseSender) {
			next(context.WithValue(ctx, ctxKey{}, "1"), req, res)
		}
	})
	middleware2 := router.MiddlewareFunc(func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx context.Context, req *backend.CallResourceRequest, res backend.CallResourceResponseSender) {
			current := ctx.Value(ctxKey{}).(string)
			next(context.WithValue(ctx, ctxKey{}, fmt.Sprintf("%s2", current)), req, res)
		}
	})
	r.Use(middleware1, middleware2)

	r.Handle("/something", func(ctx context.Context, req *backend.CallResourceRequest, res backend.CallResourceResponseSender) {
		middlewareInjectedValue := ctx.Value(ctxKey{})
		require.NotNil(t, middlewareInjectedValue, "expected middleware to have affected the context")
		res.Send(&backend.CallResourceResponse{
			Status: http.StatusOK,
			Body:   []byte(fmt.Sprintf("%shi!", middlewareInjectedValue)),
		})
	}, "GET")

	res := &router.CapturingSender{}
	err := r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/something",
		Method: "GET",
	}, res)
	require.NoError(t, err, "no error expected from calling router")
	require.Equal(t, "12hi!", string(res.Response.Body))
}

func TestMiddleware_CapturingMiddleware(t *testing.T) {
	r := router.NewRouter()
	// Each middleware will add a number to the payload, before and after
	middleware1 := router.NewCapturingMiddleware(func(ctx context.Context, req *backend.CallResourceRequest, next router.NextFunc) {
		res := next(context.WithValue(ctx, ctxKey{}, "1"))
		res.Body = []byte(fmt.Sprintf("%s1", res.Body))
	})
	middleware2 := router.NewCapturingMiddleware(func(ctx context.Context, req *backend.CallResourceRequest, next router.NextFunc) {
		current := ctx.Value(ctxKey{}).(string)
		res := next(context.WithValue(ctx, ctxKey{}, fmt.Sprintf("%s2", current)))
		res.Body = []byte(fmt.Sprintf("%s2", res.Body))
	})
	r.Use(middleware1, middleware2)

	r.Handle("/something", func(ctx context.Context, req *backend.CallResourceRequest, res backend.CallResourceResponseSender) {
		middlewareInjectedValue := ctx.Value(ctxKey{})
		require.NotNil(t, middlewareInjectedValue, "expected middleware to have affected the context")
		res.Send(&backend.CallResourceResponse{
			Status: http.StatusOK,
			Body:   []byte(fmt.Sprintf("%shi!", middlewareInjectedValue)),
		})
	}, "GET")

	res := &router.CapturingSender{}
	err := r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/something",
		Method: "GET",
	}, res)
	require.NoError(t, err, "no error expected from calling router")
	require.Equal(t, "12hi!21", string(res.Response.Body))
}
