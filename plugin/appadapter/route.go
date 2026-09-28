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

	rec := newResponseRecorder(stream)
	customReq := &app.CustomRouteRequest{
		ResourceIdentifier: routeResourceIdentifier(req),
		Path:               req.GetPath(),
		URL:                u,
		Method:             req.GetMethod(),
		Headers:            routeHeaders(req.GetHeaders()),
		Body:               io.NopCloser(bytes.NewReader(req.GetBody())),
	}

	if err := a.app.CallCustomRoute(stream.Context(), rec, customReq); err != nil {
		if errors.Is(err, app.ErrCustomRouteNotFound) && !rec.sentHeader {
			return status.Error(codes.NotFound, err.Error())
		}
		return err
	}

	// Send whatever the handler has not flushed. For a handler that never
	// flushes, this is the whole response in a single message.
	rec.Flush()
	return rec.sendErr
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

// responseRecorder is an app.CustomRouteResponseWriter that translates the
// response into CallRouteResponse messages. It buffers the body until Flush is
// called, then sends it as one message. The status code and headers are sent
// only in the first message, so they are fixed once the response is flushed.
type responseRecorder struct {
	stream     grpc.ServerStreamingServer[pluginv3.CallRouteResponse]
	header     http.Header
	body       bytes.Buffer
	statusCode int
	sentHeader bool
	sendErr    error
}

var _ http.Flusher = (*responseRecorder)(nil)

func newResponseRecorder(stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) *responseRecorder {
	return &responseRecorder{
		stream:     stream,
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

// Write buffers b until the next Flush. It returns the error from a previous
// failed send, so a handler stops writing once the caller has gone away.
func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.sendErr != nil {
		return 0, r.sendErr
	}
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	if r.sentHeader {
		return
	}
	r.statusCode = statusCode
}

// Flush implements [http.Flusher] by sending the buffered body as one
// CallRouteResponse message.
func (r *responseRecorder) Flush() {
	if r.sendErr != nil || r.sentHeader && r.body.Len() == 0 {
		return
	}
	r.sendErr = r.stream.Send(r.toCallRouteResponse())
	r.body.Reset()
}

func (r *responseRecorder) toCallRouteResponse() *pluginv3.CallRouteResponse {
	rsp := &pluginv3.CallRouteResponse{}
	if r.body.Len() > 0 {
		rsp.SetBody(bytes.Clone(r.body.Bytes()))
	}
	if r.sentHeader {
		return rsp
	}

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

	rsp.SetCode(int32(code))
	rsp.SetHeaders(headers)
	r.sentHeader = true
	return rsp
}
