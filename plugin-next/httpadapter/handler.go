package httpadapter

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"google.golang.org/grpc"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

// New creates a RouteServiceServer adapter that handles route calls using an
// http.Handler.
func New(handler http.Handler) pluginv3.RouteServiceServer {
	return &httpRouteHandler{
		handler: handler,
	}
}

type httpRouteHandler struct {
	handler http.Handler
}

func (h *httpRouteHandler) CallRoute(req *pluginv3.CallRouteRequest, sender grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	body := req.GetBody()
	var reqBodyReader io.Reader
	if len(body) > 0 {
		reqBodyReader = bytes.NewReader(body)
	}

	ctx := sender.Context()

	// TODO.. add user to context (authlib)
	reqURL, err := url.Parse(req.GetUrl())
	if err != nil {
		return err
	}

	// Add the parent to the request
	parent := req.GetParent()
	if parent != nil {
		ctx = context.WithValue(ctx, parentKey{}, parent)
	}

	resourceURL := req.GetPath()
	if reqURL.RawQuery != "" {
		resourceURL += "?" + reqURL.RawQuery
	}

	if !strings.HasPrefix(resourceURL, "/") {
		resourceURL = "/" + resourceURL
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.GetMethod(), resourceURL, reqBodyReader)
	if err != nil {
		return err
	}

	for key, values := range req.GetHeaders() {
		httpReq.Header[key] = values.GetValues()
	}

	writer := newResponseWriter(sender)
	h.handler.ServeHTTP(writer, httpReq)
	writer.Flush()

	return nil
}

type parentKey struct{}

// ParentFromContext gets the parent from context (if it was configured)
func ParentFromContext(ctx context.Context) *pluginv3.RouteResource {
	raw := ctx.Value(parentKey{})
	if raw == nil {
		return nil
	}

	parent, ok := raw.(*pluginv3.RouteResource)
	if !ok {
		return nil
	}
	return parent
}
