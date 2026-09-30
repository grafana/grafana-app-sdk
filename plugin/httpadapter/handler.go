package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	clientv3 "github.com/grafana/grafana-app-sdk/plugin/client/v3"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// HandlerFunc creates an HTTP handler that forwards requests to a
// RouteClient, such as the client returned by grpcplugin.NewClientV3. Use [WithRouteInfo] when the URL path alone does not
// contain the App Platform routing metadata. The HTTP Host is intentionally
// not forwarded; plugin route handlers must not rely on host-based routing.
// Credential headers, such as Authorization, Cookie and X-Grafana-Id, are not
// forwarded either: the caller's identity reaches the plugin only as the
// access token the client adds to each request.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
func HandlerFunc(client clientv3.RouteClient) http.HandlerFunc {
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

		stream, err := client.CallRoute(r.Context(), req)
		if err != nil {
			k8s, ok := err.(apierrors.APIStatus)
			if ok {
				status := k8s.Status()
				w.Header().Add("Content-Type", "application/json")
				w.WriteHeader(int(status.Code))
				_ = json.NewEncoder(w).Encode(status)
				return
			}
			http.Error(w, "call route: "+err.Error(), http.StatusInternalServerError)
			return
		}

		forwardResponse(w, stream)
	}
}

// credentialHeaders carry the caller's credentials to the host. Forwarding them
// would let a plugin act as the caller outside the delegated access token.
var credentialHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"X-Grafana-Id",
	"X-Id-Token",
	"X-Access-Token",
}

// isCredentialHeader matches case-insensitively, as hosts may build header
// maps with non-canonical keys.
func isCredentialHeader(key string) bool {
	for _, name := range credentialHeaders {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
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
		if isCredentialHeader(key) {
			continue
		}
		headers[key] = pluginv3.StringList_builder{Values: values}.Build()
	}

	req := &pluginv3.CallRouteRequest{}
	req.SetMethod(r.Method)
	// Use the URL path as a generic fallback. App Platform hosts override this
	// below with the path relative to the registered route.
	req.SetPath(r.URL.Path)
	req.SetUrl(r.URL.String())
	req.SetHeaders(headers)
	if len(body) > 0 {
		req.SetBody(body)
	}
	if info, ok := RouteInfoFromContext(r.Context()); ok {
		req.SetGroup(info.Group)
		req.SetVersion(info.Version)
		req.SetNamespace(info.Namespace)
		req.SetPath(info.Path)
		if info.Parent != nil {
			req.SetParent(info.Parent)
		}
	}
	return req, nil
}

func forwardResponse(w http.ResponseWriter, stream pluginv3.RouteService_CallRouteClient) {
	wroteHeader := false
	// ResponseController also finds a Flusher behind wrappers that only
	// implement Unwrap, so flushes aren't dropped by middleware.
	controller := http.NewResponseController(w)
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
		// Flushing is best effort: a writer that can't flush still receives
		// the whole response, just not incrementally.
		_ = controller.Flush()
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
