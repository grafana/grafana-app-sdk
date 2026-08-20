package httpadapter

import (
	"bytes"
	"net/http"

	"google.golang.org/grpc"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

// callRouteResponseWriter is an implementation of http.ResponseWriter that
// streams pluginv3.CallRouteResponse messages.
type callRouteResponseWriter struct {
	stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]

	// Code is the HTTP response code set by WriteHeader.
	//
	// Note that if a Handler never calls WriteHeader or Write,
	// this might end up being 0, rather than the implicit
	// http.StatusOK. To get the implicit value, use the Result
	// method.
	code int

	// wroteHeader is whether WriteHeader has been called. As with net/http,
	// only the first call to WriteHeader has an effect.
	wroteHeader bool

	// HeaderMap contains the headers explicitly set by the Handler.
	headerMap http.Header

	// Body is the buffer to which the Handler's Write calls are sent.
	// If nil, the Writes are silently discarded.
	body *bytes.Buffer

	// Flushed is whether the Handler called Flush.
	Flushed bool

	// Already sent the first stream
	sentFirstStream bool
}

func newResponseWriter(stream grpc.ServerStreamingServer[pluginv3.CallRouteResponse]) *callRouteResponseWriter {
	return &callRouteResponseWriter{
		stream:    stream,
		headerMap: make(http.Header),
		body:      new(bytes.Buffer),
		code:      http.StatusOK,
	}
}

// Header implements http.ResponseWriter. It returns the response
// headers to mutate within a handler. To test the headers that were
// written after a handler completes, use the Result method and see
// the returned Response value's Header.
func (rw *callRouteResponseWriter) Header() http.Header {
	if rw.headerMap == nil {
		rw.headerMap = make(http.Header)
	}
	return rw.headerMap
}

// writeContentTypeHeader writes a header if it was not written yet and
// detects Content-Type if needed.
//
// bytes or str are the beginning of the response body.
// We pass both to avoid unnecessarily generate garbage
// in rw.WriteString which was created for performance reasons.
// Non-nil bytes win.
func (rw *callRouteResponseWriter) writeContentTypeHeader(b []byte, str string) {
	if rw.sentFirstStream {
		return
	}
	if len(str) > 512 {
		str = str[:512]
	}

	m := rw.Header()

	_, hasType := m["Content-Type"]
	hasTE := m.Get("Transfer-Encoding") != ""
	if !hasType && !hasTE {
		if b == nil {
			b = []byte(str)
		}
		m.Set("Content-Type", http.DetectContentType(b))
	}

	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
}

// Write implements http.ResponseWriter. The data in buf is written to
// rw.Body, if not nil.
func (rw *callRouteResponseWriter) Write(buf []byte) (int, error) {
	rw.writeContentTypeHeader(buf, "")
	if rw.body == nil {
		rw.body = new(bytes.Buffer)
	}
	return rw.body.Write(buf)
}

// WriteHeader implements http.ResponseWriter.
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

// Flush implements http.Flusher.
func (rw *callRouteResponseWriter) Flush() {
	resp := rw.chunk()
	if resp != nil {
		if err := rw.stream.Send(resp); err != nil {
			log.DefaultLogger.Error("Failed to send resource response", "error", err)
		}
	}

	if rw.body != nil {
		rw.body.Reset()
	}
}

func (rw *callRouteResponseWriter) chunk() *pluginv3.CallRouteResponse {
	rsp := &pluginv3.CallRouteResponse{}

	if rw.body != nil && rw.body.Len() > 0 {
		rsp.SetBody(rw.body.Bytes())
		if rw.sentFirstStream {
			return rsp
		}
	}

	if !rw.sentFirstStream {
		if rw.code > 0 {
			rsp.SetCode(int32(rw.code)) //nolint:gosec // WriteHeader limits HTTP status codes to 100-999.
		} else {
			rsp.SetCode(200) // OK
		}

		// Copy the headers
		headerCopy := rw.Header().Clone()
		headers := make(map[string]*pluginv3.StringList, len(headerCopy))
		for k, vals := range headerCopy {
			headers[k] = pluginv3.StringList_builder{Values: vals}.Build()
		}
		rsp.SetHeaders(headers)

		// Don't send headers again
		rw.sentFirstStream = true
		return rsp
	}

	return nil // nothing to send
}
