package httpadapter

import (
	"errors"
	"io"
	"net/http"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

// HandlerFunc creates an HTTP handler that forwards requests to a RouteServiceClient.
func HandlerFunc(client pluginv3.RouteServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			http.Error(w, "route service client is not configured", http.StatusInternalServerError)
			return
		}

		req, err := requestFromHTTP(r)
		if err != nil {
			http.Error(w, "read request body: "+err.Error(), http.StatusInternalServerError)
			return
		}

		stream, err := (client).CallRoute(r.Context(), req)
		if err != nil {
			http.Error(w, "call route: "+err.Error(), http.StatusInternalServerError)
			return
		}

		forwardResponse(w, stream)
	}
}

func requestFromHTTP(r *http.Request) (*pluginv3.CallRouteRequest, error) {
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
	}

	headers := make(map[string]*pluginv3.StringList, len(r.Header))
	for key, values := range r.Header {
		headers[key] = pluginv3.StringList_builder{Values: values}.Build()
	}

	req := &pluginv3.CallRouteRequest{}
	req.SetMethod(r.Method)
	req.SetPath(r.URL.Path) // TODO? remove {group}/{version}/... prefix?
	req.SetUrl(r.URL.String())
	req.SetHeaders(headers)
	if len(body) > 0 {
		req.SetBody(body)
	}
	if parent := ParentFromContext(r.Context()); parent != nil {
		req.SetParent(parent)
	}
	return req, nil
}

func forwardResponse(w http.ResponseWriter, stream pluginv3.RouteService_CallRouteClient) {
	wroteHeader := false
	flusher, canFlush := w.(http.Flusher)
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			if !wroteHeader {
				http.Error(w, "receive route response: "+err.Error(), http.StatusInternalServerError)
			}
			return
		}

		if !wroteHeader {
			if err := writeResponseHeader(w, resp); err != nil {
				http.Error(w, "receive route response: "+err.Error(), http.StatusInternalServerError)
				return
			}
			wroteHeader = true
		}

		if _, err := w.Write(resp.GetBody()); err != nil {
			return
		}
		if canFlush {
			flusher.Flush()
		}
	}
}

func writeResponseHeader(w http.ResponseWriter, resp *pluginv3.CallRouteResponse) error {
	code := int(resp.GetCode())
	if code == 0 {
		code = http.StatusOK
	}
	if code < 100 || code > 999 {
		return errors.New("invalid HTTP status code")
	}

	for key, values := range resp.GetHeaders() {
		for _, value := range values.GetValues() {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(code)
	return nil
}
