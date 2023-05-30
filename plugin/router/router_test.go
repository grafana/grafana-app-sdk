package router

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
)

func TestRouter_CallResource(t *testing.T) {
	called := ""
	vars := make([]string, 0)
	handler := func(name string) func(ctx context.Context, request *backend.CallResourceRequest, response backend.CallResourceResponseSender) {
		return func(ctx context.Context, request *backend.CallResourceRequest, response backend.CallResourceResponseSender) {
			vars = make([]string, 0)
			for k, v := range VarsFromCtx(ctx) {
				vars = append(vars, fmt.Sprintf("%s=%s", k, v))
			}
			called = name
		}
	}
	r := NewRouter()
	r.Handle("/foo/bar", handler("bar"))
	r.Handle("/foo/{any}", handler("any"))
	r.Handle("/{any1}/{any2}", handler("any2")).Methods([]string{"POST"})
	// Subrouter checks
	sr := r.Subrouter("/sub")
	sr.Handle("/{num:[0-9]+}", handler("sub-num"))
	sr.Handle("/{nonnum}", handler("sub-nonnum"))
	// Path params in subrouter
	r.Subrouter("/{sr:[0-9]+}").Handle("/foo", handler("sr2"))

	r.NotFoundHandler = handler("not-found")

	r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/foo/bar",
		Method: "GET",
	}, nil)
	assert.Equal(t, "bar", called)
	assert.Empty(t, vars)
	r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/foo/bar1",
		Method: "GET",
	}, nil)
	assert.Equal(t, "any", called)
	assert.ElementsMatch(t, []string{"any=bar1"}, vars)
	r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/var1/var2",
		Method: "POST",
	}, nil)
	assert.Equal(t, "any2", called)
	assert.ElementsMatch(t, []string{"any1=var1", "any2=var2"}, vars)
	r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/sub/432",
		Method: "GET",
	}, nil)
	assert.Equal(t, "sub-num", called)
	assert.ElementsMatch(t, []string{"num=432"}, vars)
	r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/sub/1234a",
		Method: "GET",
	}, nil)
	assert.Equal(t, "sub-nonnum", called)
	assert.ElementsMatch(t, []string{"nonnum=1234a"}, vars)
	r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/1/foo",
		Method: "GET",
	}, nil)
	assert.Equal(t, "sr2", called)
	assert.ElementsMatch(t, []string{"sr=1"}, vars)

	// Not found
	r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/foo/bar/foo",
		Method: "GET",
	}, nil)
	assert.Equal(t, "not-found", called)
	assert.Empty(t, vars)

	t.Run("use middleware in subrouter", func(t *testing.T) {
		mCall := 0
		rCall := false
		r := NewRouter()
		r.Use(MiddlewareFunc(func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, request *backend.CallResourceRequest, sender backend.CallResourceResponseSender) {
				mCall++
				next(ctx, request, sender)
			}
		}))
		sr := r.Subrouter("/foo")
		sr.Handle("/bar", func(ctx context.Context, request *backend.CallResourceRequest, sender backend.CallResourceResponseSender) {
			rCall = true
		})
		err := r.CallResource(context.Background(), &backend.CallResourceRequest{
			Path:   "/foo/bar",
			Method: http.MethodGet,
		}, nil)
		assert.Nil(t, err)
		assert.True(t, rCall)
		assert.Equal(t, 1, mCall)
	})
}

func TestRouter_RouteByName(t *testing.T) {
	t.Run("no handlers", func(t *testing.T) {
		r := NewRouter()
		h := r.RouteByName("route")
		assert.Nil(t, h)
	})

	t.Run("name mismatch", func(t *testing.T) {
		r := NewRouter()
		r.Handle("/foo", func(ctx context.Context, request *backend.CallResourceRequest, response backend.CallResourceResponseSender) {
		}).Name("foo")
		h := r.RouteByName("bar")
		assert.Nil(t, h)
	})

	t.Run("matched name", func(t *testing.T) {
		r := NewRouter()
		handler := r.Handle("/foo", func(ctx context.Context, request *backend.CallResourceRequest, response backend.CallResourceResponseSender) {
		}).Name("foo")
		h := r.RouteByName("foo")
		assert.Equal(t, handler, h)
	})
}
