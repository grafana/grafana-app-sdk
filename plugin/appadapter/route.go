package appadapter

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"

	"google.golang.org/grpc"

	"github.com/grafana/grafana-app-sdk/app"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/resource"
)

// RouteAdapter implements the v3 route service in terms of an app-sdk App.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
type RouteAdapter struct {
	app app.App
}

// NewRouteAdapter returns a [pluginv3.RouteServiceServer] backed by a.
func NewRouteAdapter(a app.App) pluginv3.RouteServiceServer {
	return &RouteAdapter{app: a}
}

// CallRoute implements [pluginv3.RouteServiceServer] by translating the
// request into an app.CustomRouteRequest and delegating to the app-sdk App's
// CallCustomRoute.
func (a *RouteAdapter) CallRoute(req *pluginv3.CallRouteRequest, stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	u, err := url.Parse(req.GetUrl())
	if err != nil {
		return sendError(stream, http.StatusBadRequest, err.Error())
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

	info := &resource.RouteRequestInfo{
		FullIdentifier: customReq.ResourceIdentifier,
	}

	if parent := req.GetParent(); parent != nil {
		customReq.Parent = &app.RawObject{
			Raw: parent.GetRaw(),
		}

		sv := parent.GetDecryptedSecureValues()
		if len(sv) > 0 {
			customReq.DecryptedSecureValues = make(resource.DecryptedSecureValues, len(sv))
			for key, value := range sv {
				customReq.DecryptedSecureValues[key] = resource.RawSecureValue(value)
			}
		}

		info.Parent = parent.GetRaw()
		info.ResourceVersion = parent.GetRv()
		info.DecryptedSecureValues = customReq.DecryptedSecureValues
	}

	ctx := resource.WithRouteRequestInfo(stream.Context(), info)
	if err := a.app.CallCustomRoute(ctx, rec, customReq); err != nil {
		if errors.Is(err, app.ErrCustomRouteNotFound) && !rec.sentHeader {
			return sendError(stream, http.StatusNotFound, "not found")
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

// routeHeaders flattens the v3 StringList header representation into an http.Header.
func routeHeaders(headers map[string]*pluginv3.StringList) http.Header {
	h := make(http.Header, len(headers))
	for k, v := range headers {
		if v == nil {
			continue
		}
		for _, value := range v.GetValues() {
			h.Add(k, value)
		}
	}
	return h
}

// responseRecorder is an app.CustomRouteResponseWriter that translates the
// response into CallRouteResponse messages. It buffers the body until Flush is
// called, then sends it as one message. The status code and headers are sent
// only in the first message. They are fixed by the first WriteHeader, Write, or Flush.
type responseRecorder struct {
	stream          grpc.ServerStreamingServer[pluginv3.CallRouteResponse]
	header          http.Header
	committedHeader http.Header
	wroteHeader     bool
	body            bytes.Buffer
	statusCode      int
	sentHeader      bool
	sendErr         error
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
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.committedHeader = r.header.Clone()
	r.statusCode = statusCode
}

// Flush implements [http.Flusher] by sending the buffered body as one
// CallRouteResponse message.
func (r *responseRecorder) Flush() {
	if r.sendErr != nil || r.sentHeader && r.body.Len() == 0 {
		return
	}
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
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

	headers := make(map[string]*pluginv3.StringList, len(r.committedHeader))
	for k, v := range r.committedHeader {
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

type notFoundAdapter struct{}

func (*notFoundAdapter) CallRoute(_ *pluginv3.CallRouteRequest, stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) error {
	return sendError(stream, int32(http.StatusNotFound), "")
}

func sendError(stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse], code int32, msg string) error {
	rsp := &pluginv3.CallRouteResponse{}
	rsp.SetCode(code)
	if len(msg) > 0 {
		rsp.SetBody([]byte(msg))
	}
	return stream.Send(rsp)
}
