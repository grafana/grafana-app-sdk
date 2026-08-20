package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

func TestHTTPRouteHandler(t *testing.T) {
	t.Run("Given HTTP route handler and calling CallRoute", func(t *testing.T) {
		testSender := newTestCallRouteResponseSender()
		httpHandler := &testHTTPHandler{
			responseHeaders: map[string][]string{
				"X-Header-Out-1": {"A", "B"},
				"X-Header-Out-2": {"C"},
			},
			responseData: map[string]interface{}{
				"message": "hello client",
			},
			responseStatus: http.StatusCreated,
		}
		resourceHandler := New(httpHandler)

		jsonMap := map[string]interface{}{
			"message": "hello server",
		}
		reqBody, err := json.Marshal(&jsonMap)
		require.NoError(t, err)

		parent := pluginv3.RouteResource_builder{
			Resource: proto.String("widgets"),
			Name:     proto.String("widget-1"),
			Rv:       proto.String("42"),
			Raw:      []byte(`{"spec":{"enabled":true}}`),
		}.Build()
		req := pluginv3.CallRouteRequest_builder{
			Group:   proto.String("my-plugin"),
			Version: proto.String("v1alpha1"),
			Parent:  parent,
			Method:  proto.String(http.MethodPost),
			Path:    proto.String("path"),
			Url:     proto.String("/api/plugins/plugin-abc/resources/path?query=1"),
			Headers: map[string]*pluginv3.StringList{
				"X-Header-In-1": pluginv3.StringList_builder{Values: []string{"D", "E"}}.Build(),
				"X-Header-In-2": pluginv3.StringList_builder{Values: []string{"F"}}.Build(),
			},
			Body: reqBody,
		}.Build()
		err = resourceHandler.CallRoute(req, testSender)
		require.NoError(t, err)
		require.Equal(t, 1, httpHandler.callerCount)

		t.Run("Should provide expected request to http handler", func(t *testing.T) {
			require.NotNil(t, httpHandler.req)
			require.Equal(t, "/path?query=1", httpHandler.req.URL.String())
			require.Equal(t, req.GetMethod(), httpHandler.req.Method)
			require.Contains(t, httpHandler.req.Header, "X-Header-In-1")
			require.Equal(t, []string{"D", "E"}, httpHandler.req.Header["X-Header-In-1"])
			require.Contains(t, httpHandler.req.Header, "X-Header-In-2")
			require.Equal(t, []string{"F"}, httpHandler.req.Header["X-Header-In-2"])
			require.NotNil(t, httpHandler.req.Body)
			defer func() { _ = httpHandler.req.Body.Close() }()
			actualBodyBytes, err := io.ReadAll(httpHandler.req.Body)
			require.NoError(t, err)
			var actualJSONMap map[string]interface{}
			err = json.Unmarshal(actualBodyBytes, &actualJSONMap)
			require.NoError(t, err)
			require.Contains(t, actualJSONMap, "message")
			require.Equal(t, "hello server", actualJSONMap["message"])
		})

		t.Run("Should return expected response from http handler", func(t *testing.T) {
			require.Len(t, testSender.respMessages, 1)
			resp := testSender.respMessages[0]
			require.NotNil(t, resp)
			require.NoError(t, httpHandler.writeErr)
			require.NotNil(t, resp)
			require.Equal(t, int32(http.StatusCreated), resp.GetCode())
			require.Contains(t, resp.GetHeaders(), "X-Header-Out-1")
			require.Equal(t, []string{"A", "B"}, resp.GetHeaders()["X-Header-Out-1"].GetValues())
			require.Contains(t, resp.GetHeaders(), "X-Header-Out-2")
			require.Equal(t, []string{"C"}, resp.GetHeaders()["X-Header-Out-2"].GetValues())
			var actualJSONMap map[string]interface{}
			err = json.Unmarshal(resp.GetBody(), &actualJSONMap)
			require.NoError(t, err)
			require.Contains(t, actualJSONMap, "message")
			require.Equal(t, "hello client", actualJSONMap["message"])
		})

		t.Run("Should provide the parent resource in the request context", func(t *testing.T) {
			require.NotNil(t, httpHandler.req)
			require.Same(t, parent, ParentFromContext(httpHandler.req.Context()))
		})
	})

	t.Run("Given streaming HTTP route handler and calling CallRoute", func(t *testing.T) {
		testSender := newTestCallRouteResponseSender()
		httpHandler := &testStreamingHTTPHandler{
			responseHeaders: map[string][]string{
				"X-Header-Out-1": {"A", "B"},
				"X-Header-Out-2": {"C"},
			},
			responseData: [][]byte{
				[]byte("hello"),
				[]byte("world"),
				[]byte("bye bye"),
			},
			responseStatus: http.StatusOK,
		}
		resourceHandler := New(httpHandler)
		req := pluginv3.CallRouteRequest_builder{
			Group:  proto.String("my-plugin"),
			Method: proto.String(http.MethodPost),
			Path:   proto.String("path"),
			Url:    proto.String("/api/plugins/plugin-abc/resources/path?query=1"),
			Headers: map[string]*pluginv3.StringList{
				"X-Header-In-1": pluginv3.StringList_builder{Values: []string{"D", "E"}}.Build(),
				"X-Header-In-2": pluginv3.StringList_builder{Values: []string{"F"}}.Build(),
			},
		}.Build()
		err := resourceHandler.CallRoute(req, testSender)
		require.NoError(t, err)
		require.Equal(t, 1, httpHandler.callerCount)

		t.Run("Should return expected response from http handler", func(t *testing.T) {
			require.Len(t, testSender.respMessages, 3)
			resp1 := testSender.respMessages[0]
			require.NotNil(t, resp1)
			require.NoError(t, httpHandler.writeErr)
			require.NotNil(t, resp1)
			require.Equal(t, int32(http.StatusOK), resp1.GetCode())
			require.Contains(t, resp1.GetHeaders(), "X-Header-Out-1")
			require.Equal(t, []string{"A", "B"}, resp1.GetHeaders()["X-Header-Out-1"].GetValues())
			require.Contains(t, resp1.GetHeaders(), "X-Header-Out-2")
			require.Equal(t, []string{"C"}, resp1.GetHeaders()["X-Header-Out-2"].GetValues())
			require.Equal(t, "hello", string(resp1.GetBody()))

			resp2 := testSender.respMessages[1]
			require.NotNil(t, resp2)
			require.Equal(t, "world", string(resp2.GetBody()))

			resp3 := testSender.respMessages[2]
			require.NotNil(t, resp3)
			require.Equal(t, "bye bye", string(resp3.GetBody()))
		})
	})
}

func TestServeMuxHandler(t *testing.T) {
	t.Run("Given HTTP route ServeMux handler and calling CallRoute", func(t *testing.T) {
		testSender := newTestCallRouteResponseSender()
		mux := http.NewServeMux()
		handlerWasCalled := false
		mux.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			handlerWasCalled = true
		})
		resourceHandler := New(mux)

		req := pluginv3.CallRouteRequest_builder{
			Group:  proto.String("my-plugin"),
			Method: proto.String(http.MethodGet),
			Path:   proto.String("test"),
			Url:    proto.String("/test?query=1"),
		}.Build()
		err := resourceHandler.CallRoute(req, testSender)
		require.NoError(t, err)
		require.True(t, handlerWasCalled)
	})
}

func TestCallRouteErrors(t *testing.T) {
	tests := []struct {
		name string
		req  *pluginv3.CallRouteRequest
	}{
		{
			name: "invalid URL",
			req: pluginv3.CallRouteRequest_builder{
				Method: proto.String(http.MethodGet),
				Url:    proto.String("%"),
			}.Build(),
		},
		{
			name: "invalid HTTP method",
			req: pluginv3.CallRouteRequest_builder{
				Method: proto.String("invalid\nmethod"),
				Url:    proto.String("/valid"),
			}.Build(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := New(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			}))

			err := handler.CallRoute(tt.req, newTestCallRouteResponseSender())

			require.Error(t, err)
			require.False(t, called)
		})
	}
}

func TestCallRouteReturnsStreamSendError(t *testing.T) {
	sender := newTestCallRouteResponseSender()
	sender.sendErr = errors.New("send failed")
	handler := New(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, err := rw.Write([]byte("response"))
		require.NoError(t, err)
	}))
	req := pluginv3.CallRouteRequest_builder{
		Method: proto.String(http.MethodGet),
		Path:   proto.String("test"),
		Url:    proto.String("/test"),
	}.Build()

	err := handler.CallRoute(req, sender)

	require.ErrorIs(t, err, sender.sendErr)
}

func TestCallRouteWithoutOptionalRequestFields(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "context value")
	sender := newTestCallRouteResponseSender()
	sender.ctx = ctx

	handler := New(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/already-absolute", req.URL.String())
		require.Equal(t, "context value", req.Context().Value(contextKey{}))
		require.Nil(t, ParentFromContext(req.Context()))
		require.Empty(t, req.Header)
		rw.WriteHeader(http.StatusNoContent)
	}))
	req := pluginv3.CallRouteRequest_builder{
		Method: proto.String(http.MethodGet),
		Path:   proto.String("/already-absolute"),
		Url:    proto.String("/already-absolute"),
	}.Build()

	err := handler.CallRoute(req, sender)

	require.NoError(t, err)
	require.Len(t, sender.respMessages, 1)
	require.Equal(t, int32(http.StatusNoContent), sender.respMessages[0].GetCode())
}

type testHTTPHandler struct {
	responseStatus  int
	responseHeaders map[string][]string
	responseData    map[string]interface{}
	callerCount     int
	req             *http.Request
	writeErr        error
}

func (h *testHTTPHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h.callerCount++
	h.req = req

	if h.responseHeaders != nil {
		for k, values := range h.responseHeaders {
			for _, v := range values {
				rw.Header().Add(k, v)
			}
		}
	}

	if h.responseStatus != 0 {
		rw.WriteHeader(h.responseStatus)
	} else {
		rw.WriteHeader(200)
	}

	if h.responseData != nil {
		body, _ := json.Marshal(&h.responseData)
		_, h.writeErr = rw.Write(body)
	}
}

type testStreamingHTTPHandler struct {
	responseStatus  int
	responseHeaders map[string][]string
	responseData    [][]byte
	callerCount     int
	req             *http.Request
	writeErr        error
}

func (h *testStreamingHTTPHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	h.callerCount++
	h.req = req

	if h.responseHeaders != nil {
		for k, values := range h.responseHeaders {
			for _, v := range values {
				rw.Header().Add(k, v)
			}
		}
	}

	if h.responseStatus != 0 {
		rw.WriteHeader(h.responseStatus)
	} else {
		rw.WriteHeader(200)
	}

	for _, bytes := range h.responseData {
		_, h.writeErr = rw.Write(bytes)
		rw.(http.Flusher).Flush()
	}
}

type testCallRouteResponseSender struct {
	grpc.ServerStream
	respMessages []*pluginv3.CallRouteResponse
	ctx          context.Context
	sendErr      error
}

func newTestCallRouteResponseSender() *testCallRouteResponseSender {
	return &testCallRouteResponseSender{
		respMessages: []*pluginv3.CallRouteResponse{},
	}
}

func (s *testCallRouteResponseSender) Send(resp *pluginv3.CallRouteResponse) error {
	s.respMessages = append(s.respMessages, proto.Clone(resp).(*pluginv3.CallRouteResponse))
	return s.sendErr
}

func (s *testCallRouteResponseSender) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}
