package httpadapter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

func TestHandlerFunc(t *testing.T) {
	parent := &pluginv3.RouteResource{}
	parent.SetResource("widgets")
	parent.SetName("widget-1")

	grpcClient := &testRouteServiceClient{
		stream: &testCallRouteResponseReceiver{responses: []*pluginv3.CallRouteResponse{
			callRouteResponse(http.StatusCreated, map[string][]string{
				"X-Response": {"one", "two"},
			}, []byte("hello ")),
			callRouteResponse(0, nil, []byte("world")),
		}},
	}
	client := pluginv3.RouteServiceClient(grpcClient)
	handler := HandlerFunc(&client)

	ctx := context.WithValue(context.Background(), parentKey{}, parent)
	req := httptest.NewRequest(http.MethodPost, "/route?query=1", strings.NewReader("request body")).WithContext(ctx)
	req.Header.Add("X-Request", "one")
	req.Header.Add("X-Request", "two")
	recorder := httptest.NewRecorder()

	handler(recorder, req)

	require.Same(t, ctx, grpcClient.ctx)
	require.Equal(t, http.MethodPost, grpcClient.req.GetMethod())
	require.Equal(t, "/route", grpcClient.req.GetPath())
	require.Equal(t, "/route?query=1", grpcClient.req.GetUrl())
	require.Equal(t, []string{"one", "two"}, grpcClient.req.GetHeaders()["X-Request"].GetValues())
	require.Equal(t, "request body", string(grpcClient.req.GetBody()))
	require.Same(t, parent, grpcClient.req.GetParent())

	result := recorder.Result()
	defer func() { _ = result.Body.Close() }()
	responseBody, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, result.StatusCode)
	require.Equal(t, []string{"one", "two"}, result.Header.Values("X-Response"))
	require.Equal(t, "hello world", string(responseBody))
	require.True(t, recorder.Flushed)
}

func TestHandlerFuncCallError(t *testing.T) {
	grpcClient := &testRouteServiceClient{err: errors.New("call failed")}
	client := pluginv3.RouteServiceClient(grpcClient)
	recorder := httptest.NewRecorder()

	HandlerFunc(&client)(recorder, httptest.NewRequest(http.MethodGet, "/route", nil))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "call failed")
}

func TestHandlerFuncErrors(t *testing.T) {
	t.Run("client is not configured", func(t *testing.T) {
		recorder := httptest.NewRecorder()

		HandlerFunc(nil)(recorder, httptest.NewRequest(http.MethodGet, "/route", nil))

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Contains(t, recorder.Body.String(), "route service client is not configured")
	})

	t.Run("request body cannot be read", func(t *testing.T) {
		grpcClient := &testRouteServiceClient{}
		client := pluginv3.RouteServiceClient(grpcClient)
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/route", nil)
		req.Body = io.NopCloser(&testErrorReader{err: errors.New("read failed")})

		HandlerFunc(&client)(recorder, req)

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Contains(t, recorder.Body.String(), "read failed")
		require.Nil(t, grpcClient.req)
	})

	t.Run("response cannot be received before headers", func(t *testing.T) {
		grpcClient := &testRouteServiceClient{
			stream: &testCallRouteResponseReceiver{err: errors.New("receive failed")},
		}
		client := pluginv3.RouteServiceClient(grpcClient)
		recorder := httptest.NewRecorder()

		HandlerFunc(&client)(recorder, httptest.NewRequest(http.MethodGet, "/route", nil))

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Contains(t, recorder.Body.String(), "receive failed")
	})

	t.Run("response cannot be received after headers", func(t *testing.T) {
		grpcClient := &testRouteServiceClient{
			stream: &testCallRouteResponseReceiver{
				responses: []*pluginv3.CallRouteResponse{callRouteResponse(http.StatusOK, nil, []byte("partial"))},
				err:       errors.New("receive failed"),
			},
		}
		client := pluginv3.RouteServiceClient(grpcClient)
		recorder := httptest.NewRecorder()

		HandlerFunc(&client)(recorder, httptest.NewRequest(http.MethodGet, "/route", nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "partial", recorder.Body.String())
	})

	t.Run("response has invalid status", func(t *testing.T) {
		grpcClient := &testRouteServiceClient{
			stream: &testCallRouteResponseReceiver{
				responses: []*pluginv3.CallRouteResponse{callRouteResponse(1000, nil, nil)},
			},
		}
		client := pluginv3.RouteServiceClient(grpcClient)
		recorder := httptest.NewRecorder()

		HandlerFunc(&client)(recorder, httptest.NewRequest(http.MethodGet, "/route", nil))

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Contains(t, recorder.Body.String(), "invalid HTTP status code")
	})
}

func TestHandlerFuncResponseWriterVariants(t *testing.T) {
	t.Run("defaults status and supports a writer without flush", func(t *testing.T) {
		grpcClient := &testRouteServiceClient{
			stream: &testCallRouteResponseReceiver{
				responses: []*pluginv3.CallRouteResponse{callRouteResponse(0, nil, []byte("response"))},
			},
		}
		client := pluginv3.RouteServiceClient(grpcClient)
		writer := newTestHTTPResponseWriter(nil)

		HandlerFunc(&client)(writer, httptest.NewRequest(http.MethodGet, "/route", nil))

		require.Equal(t, http.StatusOK, writer.status)
		require.Equal(t, "response", writer.body.String())
	})

	t.Run("stops when writing the response fails", func(t *testing.T) {
		writeErr := errors.New("write failed")
		grpcClient := &testRouteServiceClient{
			stream: &testCallRouteResponseReceiver{
				responses: []*pluginv3.CallRouteResponse{callRouteResponse(http.StatusOK, nil, []byte("response"))},
			},
		}
		client := pluginv3.RouteServiceClient(grpcClient)
		writer := newTestHTTPResponseWriter(writeErr)

		HandlerFunc(&client)(writer, httptest.NewRequest(http.MethodGet, "/route", nil))

		require.Equal(t, http.StatusOK, writer.status)
		require.True(t, writer.writeCalled)
	})
}

type testRouteServiceClient struct {
	ctx    context.Context
	req    *pluginv3.CallRouteRequest
	stream grpc.ServerStreamingClient[pluginv3.CallRouteResponse]
	err    error
}

func (c *testRouteServiceClient) CallRoute(ctx context.Context, req *pluginv3.CallRouteRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pluginv3.CallRouteResponse], error) {
	c.ctx = ctx
	c.req = req
	return c.stream, c.err
}

type testCallRouteResponseReceiver struct {
	grpc.ClientStream
	responses []*pluginv3.CallRouteResponse
	err       error
}

func (r *testCallRouteResponseReceiver) Recv() (*pluginv3.CallRouteResponse, error) {
	if len(r.responses) == 0 {
		if r.err != nil {
			return nil, r.err
		}
		return nil, io.EOF
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	return response, nil
}

type testErrorReader struct {
	err error
}

func (r *testErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}

type testHTTPResponseWriter struct {
	header      http.Header
	status      int
	body        strings.Builder
	writeErr    error
	writeCalled bool
}

func newTestHTTPResponseWriter(writeErr error) *testHTTPResponseWriter {
	return &testHTTPResponseWriter{
		header:   make(http.Header),
		writeErr: writeErr,
	}
}

func (w *testHTTPResponseWriter) Header() http.Header {
	return w.header
}

func (w *testHTTPResponseWriter) Write(body []byte) (int, error) {
	w.writeCalled = true
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.body.Write(body)
}

func (w *testHTTPResponseWriter) WriteHeader(status int) {
	w.status = status
}

func callRouteResponse(code int, headers map[string][]string, body []byte) *pluginv3.CallRouteResponse {
	response := &pluginv3.CallRouteResponse{}
	if code != 0 {
		response.SetCode(int32(code)) //nolint:gosec // Test status codes are constants in the valid HTTP range.
	}
	if headers != nil {
		protoHeaders := make(map[string]*pluginv3.StringList, len(headers))
		for key, values := range headers {
			protoHeaders[key] = pluginv3.StringList_builder{Values: values}.Build()
		}
		response.SetHeaders(protoHeaders)
	}
	if body != nil {
		response.SetBody(body)
	}
	return response
}
