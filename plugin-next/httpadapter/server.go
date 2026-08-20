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
func NewServer(handler http.Handler) pluginv3.RouteServiceServer {
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
		for _, value := range values.GetValues() {
			httpReq.Header.Add(key, value)
		}
	}

	writer := newResponseWriter(sender)
	h.handler.ServeHTTP(writer, httpReq)
	writer.Flush()

	return writer.sendErr
}

type parentKey struct{}

// ParentFromContext returns the resource associated with a subresource route.
// It returns nil when the route request has no parent resource.
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
