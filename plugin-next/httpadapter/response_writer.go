package httpadapter

import (
	"bytes"
	"net/http"

	"google.golang.org/grpc"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

// callRouteResponseWriter is an implementation of http.ResponseWriter that
// streams pluginv3.CallRouteResponse messages.
type callRouteResponseWriter struct {
	stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]

	code        int
	wroteHeader bool
	header      http.Header
	body        bytes.Buffer
	sentHeader  bool
	sendErr     error
}

func newResponseWriter(stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) *callRouteResponseWriter {
	return &callRouteResponseWriter{
		stream: stream,
		header: make(http.Header),
		code:   http.StatusOK,
	}
}

// Header implements [http.ResponseWriter].
func (rw *callRouteResponseWriter) Header() http.Header {
	if rw.header == nil {
		rw.header = make(http.Header)
	}
	return rw.header
}

func (rw *callRouteResponseWriter) detectContentType(body []byte) {
	if rw.wroteHeader {
		return
	}

	header := rw.Header()
	_, hasContentType := header["Content-Type"]
	if !hasContentType && header.Get("Transfer-Encoding") == "" {
		header.Set("Content-Type", http.DetectContentType(body))
	}
}

// Write implements [http.ResponseWriter].
func (rw *callRouteResponseWriter) Write(buf []byte) (int, error) {
	if rw.sendErr != nil {
		return 0, rw.sendErr
	}
	if !rw.wroteHeader {
		rw.detectContentType(buf)
		rw.WriteHeader(http.StatusOK)
	}
	return rw.body.Write(buf)
}

// WriteHeader implements [http.ResponseWriter]. Only the first call has an
// effect, matching the behavior of the standard library HTTP server.
func (rw *callRouteResponseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	if code < 100 || code > 999 {
		panic("invalid WriteHeader code")
	}

	rw.code = code
	rw.wroteHeader = true
}

// Flush implements [http.Flusher]. It sends the current body as one gRPC
// response chunk. The status and headers are included only in the first chunk.
// Any send error is returned by the adapter after the HTTP handler exits.
func (rw *callRouteResponseWriter) Flush() {
	if rw.sendErr != nil {
		return
	}
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}

	resp := rw.chunk()
	if resp != nil {
		rw.sendErr = rw.stream.Send(resp)
	}
	rw.body.Reset()
}

func (rw *callRouteResponseWriter) chunk() *pluginv3.CallRouteResponse {
	rsp := &pluginv3.CallRouteResponse{}

	if rw.body.Len() > 0 {
		rsp.SetBody(rw.body.Bytes())
		if rw.sentHeader {
			return rsp
		}
	}

	if !rw.sentHeader {
		rsp.SetCode(int32(rw.code)) //nolint:gosec // WriteHeader limits HTTP status codes to 100-999.

		headers := make(map[string]*pluginv3.StringList, len(rw.Header()))
		for key, values := range rw.Header() {
			headers[key] = pluginv3.StringList_builder{Values: values}.Build()
		}
		rsp.SetHeaders(headers)

		rw.sentHeader = true
		return rsp
	}

	return nil
}
