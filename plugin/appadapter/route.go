package appadapter

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/grafana-app-sdk/app"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/resource"
)

// Make sure RouteAdapter implements the service interface. This is important to
// do since otherwise we will only get a not implemented error response from
// the plugin at runtime.
var _ pluginv3.RouteServiceServer = (*RouteAdapter)(nil)

// RouteAdapter implements the v3 route service in terms of an app-sdk App.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type RouteAdapter struct {
	pluginv3.UnimplementedRouteServiceServer

	app app.App
}

// NewRouteAdapter returns a [pluginv3.RouteServiceServer] backed by a.
func NewRouteAdapter(a app.App) *RouteAdapter {
	return &RouteAdapter{app: a}
}

// CallRoute implements [pluginv3.RouteServiceServer] by translating the
// request into an app.CustomRouteRequest and delegating to the app-sdk App's
// CallCustomRoute.
func (a *RouteAdapter) CallRoute(req *pluginv3.CallRouteRequest, stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	u, err := url.Parse(req.GetUrl())
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	rec := newResponseRecorder()
	customReq := &app.CustomRouteRequest{
		ResourceIdentifier: routeResourceIdentifier(req),
		Path:               req.GetPath(),
		URL:                u,
		Method:             req.GetMethod(),
		Headers:            routeHeaders(req.GetHeaders()),
		Body:               io.NopCloser(bytes.NewReader(req.GetBody())),
	}

	if err := a.app.CallCustomRoute(stream.Context(), rec, customReq); err != nil {
		if errors.Is(err, app.ErrCustomRouteNotFound) {
			return status.Error(codes.NotFound, err.Error())
		}
		return err
	}

	return stream.Send(rec.toCallRouteResponse())
}

// routeResourceIdentifier builds a resource.FullIdentifier from the parts of
// a CallRouteRequest the v3 protocol provides.
func routeResourceIdentifier(req *pluginv3.CallRouteRequest) resource.FullIdentifier {
	id := resource.FullIdentifier{
		Group:     req.GetGroup(),
		Version:   req.GetVersion(),
		Namespace: req.GetNamespace(),
	}
	if parent := req.GetParent(); parent != nil {
		id.Plural = parent.GetResource()
		id.Name = parent.GetName()
	}
	return id
}

// routeHeaders flattens the v3 StringList header representation into an
// http.Header.
func routeHeaders(headers map[string]*pluginv3.StringList) http.Header {
	h := make(http.Header, len(headers))
	for k, v := range headers {
		if v == nil {
			continue
		}
		h[k] = v.GetValues()
	}
	return h
}

// responseRecorder is a minimal app.CustomRouteResponseWriter that buffers the
// status code, headers, and body so they can be translated into a single
// CallRouteResponse.
type responseRecorder struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

func (r *responseRecorder) toCallRouteResponse() *pluginv3.CallRouteResponse {
	headers := make(map[string]*pluginv3.StringList, len(r.header))
	for k, v := range r.header {
		sl := &pluginv3.StringList{}
		sl.SetValues(v)
		headers[k] = sl
	}

	code := r.statusCode
	if code < 0 || code > math.MaxInt32 {
		code = http.StatusInternalServerError
	}

	rsp := &pluginv3.CallRouteResponse{}
	rsp.SetCode(int32(code))
	rsp.SetHeaders(headers)
	rsp.SetBody(r.body.Bytes())
	return rsp
}
