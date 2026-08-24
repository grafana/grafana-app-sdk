package httpadapter

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"google.golang.org/grpc"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// New creates a RouteServiceServer adapter that handles route calls using an
// http.Handler. Reconstructed requests have an empty Host because the plugin
// route protocol intentionally does not support host-based routing.
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

	ctx := WithRouteInfo(sender.Context(), RouteInfo{
		Group:     req.GetGroup(),
		Version:   req.GetVersion(),
		Namespace: req.GetNamespace(),
		Path:      req.GetPath(),
		Parent:    req.GetParent(),
	})

	reqURL, err := url.Parse(req.GetUrl())
	if err != nil {
		return err
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

type routeInfoKey struct{}

// RouteInfo contains the App Platform routing metadata associated with an HTTP
// request. Path is relative to the route registered in the app manifest.
type RouteInfo struct {
	Group     string
	Version   string
	Namespace string
	Path      string
	Parent    *pluginv3.RouteResource
}

// WithRouteInfo returns a context carrying info. Hosts should add route
// metadata before passing an HTTP request to [HandlerFunc].
func WithRouteInfo(ctx context.Context, info RouteInfo) context.Context {
	return context.WithValue(ctx, routeInfoKey{}, info)
}

// RouteInfoFromContext returns the App Platform routing metadata associated
// with ctx. It returns false when no route metadata has been attached.
func RouteInfoFromContext(ctx context.Context) (RouteInfo, bool) {
	info, ok := ctx.Value(routeInfoKey{}).(RouteInfo)
	return info, ok
}

// ParentFromContext returns the resource associated with a subresource route.
// It returns nil when the route request has no parent resource.
func ParentFromContext(ctx context.Context) *pluginv3.RouteResource {
	info, ok := RouteInfoFromContext(ctx)
	if !ok {
		return nil
	}
	return info.Parent
}
