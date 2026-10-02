package appadapter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/emicklei/go-restful/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/health"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// fakeApp is a minimal app.App used to exercise CallRoute without depending
// on a real app-sdk App implementation.
type fakeApp struct {
	callCustomRoute func(ctx context.Context, writer app.CustomRouteResponseWriter, req *app.CustomRouteRequest) error
}

func (*fakeApp) PrometheusCollectors() []prometheus.Collector          { return nil }
func (*fakeApp) HealthChecks() []health.Check                          { return nil }
func (*fakeApp) Validate(context.Context, *app.AdmissionRequest) error { return nil }
func (*fakeApp) Mutate(context.Context, *app.AdmissionRequest) (*app.MutatingResponse, error) {
	return nil, app.ErrNotImplemented
}
func (*fakeApp) Convert(context.Context, app.ConversionRequest) (*app.RawObject, error) {
	return nil, app.ErrNotImplemented
}
func (a *fakeApp) CallCustomRoute(ctx context.Context, writer app.CustomRouteResponseWriter, req *app.CustomRouteRequest) error {
	return a.callCustomRoute(ctx, writer, req)
}
func (*fakeApp) ManagedKinds() []resource.Kind { return nil }
func (*fakeApp) Runner() app.Runnable          { return nil }

var _ app.App = (*fakeApp)(nil)

// fakeStream is a minimal grpc.ServerStreamingServer[*pluginv3.CallRouteResponse]
// that records the responses sent to it.
type fakeStream struct {
	ctx     context.Context
	sent    []*pluginv3.CallRouteResponse
	sendErr error
}

func (s *fakeStream) Send(rsp *pluginv3.CallRouteResponse) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent = append(s.sent, rsp)
	return nil
}
func (s *fakeStream) Context() context.Context   { return s.ctx }
func (*fakeStream) SendMsg(any) error            { return nil }
func (*fakeStream) RecvMsg(any) error            { return nil }
func (*fakeStream) SetHeader(metadata.MD) error  { return nil }
func (*fakeStream) SendHeader(metadata.MD) error { return nil }
func (*fakeStream) SetTrailer(metadata.MD)       {}

func newStream() *fakeStream {
	return &fakeStream{ctx: context.Background()}
}

func TestRouteAdapter_CallRoute(t *testing.T) {
	t.Run("delegates to app.CallCustomRoute and streams the response", func(t *testing.T) {
		var gotReq *app.CustomRouteRequest
		a := NewRouteAdapter(&fakeApp{
			callCustomRoute: func(_ context.Context, writer app.CustomRouteResponseWriter, req *app.CustomRouteRequest) error {
				gotReq = req
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusCreated)
				_, err := writer.Write([]byte(`{"ok":true}`))
				return err
			},
		})

		req := &pluginv3.CallRouteRequest{}
		req.SetGroup("test.grafana.app")
		req.SetVersion("v1alpha1")
		req.SetNamespace("default")
		req.SetPath("foo")
		req.SetMethod(http.MethodPost)
		req.SetUrl("https://example.com/apis/test.grafana.app/v1alpha1/namespaces/default/foo?x=1")
		req.SetBody([]byte(`{"hello":"world"}`))

		parent := &pluginv3.RouteResource{}
		parent.SetResource("foos")
		parent.SetName("bar")
		req.SetParent(parent)

		stream := newStream()
		if err := a.CallRoute(req, stream); err != nil {
			t.Fatalf("CallRoute returned error: %v", err)
		}

		if gotReq == nil {
			t.Fatal("expected app.CallCustomRoute to be invoked")
		}
		if gotReq.ResourceIdentifier.Group != "test.grafana.app" || gotReq.ResourceIdentifier.Version != "v1alpha1" ||
			gotReq.ResourceIdentifier.Namespace != "default" || gotReq.ResourceIdentifier.Plural != "foos" ||
			gotReq.ResourceIdentifier.Name != "bar" {
			t.Fatalf("unexpected resource identifier: %+v", gotReq.ResourceIdentifier)
		}
		if gotReq.Path != "foo" || gotReq.Method != http.MethodPost {
			t.Fatalf("unexpected path/method: %q %q", gotReq.Path, gotReq.Method)
		}
		if gotReq.URL == nil || gotReq.URL.Query().Get("x") != "1" {
			t.Fatalf("unexpected URL: %+v", gotReq.URL)
		}
		body := new(bytes.Buffer)
		if _, err := body.ReadFrom(gotReq.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}
		if body.String() != `{"hello":"world"}` {
			t.Fatalf("unexpected body: %s", body.String())
		}

		if len(stream.sent) != 1 {
			t.Fatalf("expected exactly one streamed response, got %d", len(stream.sent))
		}
		rsp := stream.sent[0]
		if rsp.GetCode() != http.StatusCreated {
			t.Fatalf("unexpected status code: %d", rsp.GetCode())
		}
		if string(rsp.GetBody()) != `{"ok":true}` {
			t.Fatalf("unexpected response body: %s", rsp.GetBody())
		}
		if got := rsp.GetHeaders()["Content-Type"].GetValues(); len(got) != 1 || got[0] != "application/json" {
			t.Fatalf("unexpected headers: %+v", rsp.GetHeaders())
		}
	})

	t.Run("maps ErrCustomRouteNotFound to a NotFound gRPC status", func(t *testing.T) {
		a := NewRouteAdapter(&fakeApp{
			callCustomRoute: func(context.Context, app.CustomRouteResponseWriter, *app.CustomRouteRequest) error {
				return app.ErrCustomRouteNotFound
			},
		})

		req := &pluginv3.CallRouteRequest{}
		req.SetUrl("https://example.com/foo")

		stream := newStream()
		err := a.CallRoute(req, stream)
		if err == nil {
			t.Fatal("expected an error")
		}
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound status, got %v", err)
		}
		if len(stream.sent) != 0 {
			t.Fatalf("expected no streamed response, got %d", len(stream.sent))
		}
	})

	t.Run("propagates other errors from CallCustomRoute", func(t *testing.T) {
		wantErr := errors.New("boom")
		a := NewRouteAdapter(&fakeApp{
			callCustomRoute: func(context.Context, app.CustomRouteResponseWriter, *app.CustomRouteRequest) error {
				return wantErr
			},
		})

		req := &pluginv3.CallRouteRequest{}
		req.SetUrl("https://example.com/foo")

		stream := newStream()
		err := a.CallRoute(req, stream)
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected wrapped %v, got %v", wantErr, err)
		}
	})

	t.Run("sends a message for each flush", func(t *testing.T) {
		a := NewRouteAdapter(&fakeApp{
			callCustomRoute: func(_ context.Context, writer app.CustomRouteResponseWriter, _ *app.CustomRouteRequest) error {
				writer.Header().Set("Content-Type", "text/event-stream")
				writer.WriteHeader(http.StatusAccepted)
				_, _ = writer.Write([]byte("data: one\n\n"))
				writer.(http.Flusher).Flush()
				// Flushing with nothing buffered sends nothing.
				writer.(http.Flusher).Flush()
				// The status and headers were sent with the first message.
				writer.Header().Set("X-Late", "ignored")
				writer.WriteHeader(http.StatusTeapot)
				_, _ = writer.Write([]byte("data: two\n\n"))
				writer.(http.Flusher).Flush()
				_, err := writer.Write([]byte("data: three\n\n"))
				return err
			},
		})

		req := &pluginv3.CallRouteRequest{}
		req.SetUrl("https://example.com/foo")

		stream := newStream()
		if err := a.CallRoute(req, stream); err != nil {
			t.Fatalf("CallRoute returned error: %v", err)
		}

		if len(stream.sent) != 3 {
			t.Fatalf("expected 3 streamed responses, got %d", len(stream.sent))
		}
		first := stream.sent[0]
		if first.GetCode() != http.StatusAccepted || string(first.GetBody()) != "data: one\n\n" {
			t.Fatalf("unexpected first response: %d %q", first.GetCode(), first.GetBody())
		}
		if got := first.GetHeaders()["Content-Type"].GetValues(); len(got) != 1 || got[0] != "text/event-stream" {
			t.Fatalf("unexpected headers: %+v", first.GetHeaders())
		}
		for i, want := range []string{"data: two\n\n", "data: three\n\n"} {
			rsp := stream.sent[i+1]
			if rsp.GetCode() != 0 || len(rsp.GetHeaders()) != 0 || string(rsp.GetBody()) != want {
				t.Fatalf("unexpected response %d: %d %+v %q", i+1, rsp.GetCode(), rsp.GetHeaders(), rsp.GetBody())
			}
		}
	})

	t.Run("sends headers when the handler writes no body", func(t *testing.T) {
		a := NewRouteAdapter(&fakeApp{
			callCustomRoute: func(_ context.Context, writer app.CustomRouteResponseWriter, _ *app.CustomRouteRequest) error {
				writer.WriteHeader(http.StatusNoContent)
				return nil
			},
		})

		req := &pluginv3.CallRouteRequest{}
		req.SetUrl("https://example.com/foo")

		stream := newStream()
		if err := a.CallRoute(req, stream); err != nil {
			t.Fatalf("CallRoute returned error: %v", err)
		}
		if len(stream.sent) != 1 || stream.sent[0].GetCode() != http.StatusNoContent || len(stream.sent[0].GetBody()) != 0 {
			t.Fatalf("unexpected responses: %+v", stream.sent)
		}
	})

	t.Run("returns send errors to the handler and the caller", func(t *testing.T) {
		sendErr := errors.New("stream closed")
		var writeErr error
		a := NewRouteAdapter(&fakeApp{
			callCustomRoute: func(_ context.Context, writer app.CustomRouteResponseWriter, _ *app.CustomRouteRequest) error {
				_, _ = writer.Write([]byte("data: one\n\n"))
				writer.(http.Flusher).Flush()
				_, writeErr = writer.Write([]byte("data: two\n\n"))
				return nil
			},
		})

		req := &pluginv3.CallRouteRequest{}
		req.SetUrl("https://example.com/foo")

		stream := newStream()
		stream.sendErr = sendErr
		if err := a.CallRoute(req, stream); !errors.Is(err, sendErr) {
			t.Fatalf("expected %v, got %v", sendErr, err)
		}
		if !errors.Is(writeErr, sendErr) {
			t.Fatalf("expected Write to return %v, got %v", sendErr, writeErr)
		}
	})

	t.Run("does not map ErrCustomRouteNotFound after the response has started", func(t *testing.T) {
		a := NewRouteAdapter(&fakeApp{
			callCustomRoute: func(_ context.Context, writer app.CustomRouteResponseWriter, _ *app.CustomRouteRequest) error {
				_, _ = writer.Write([]byte("partial"))
				writer.(http.Flusher).Flush()
				return app.ErrCustomRouteNotFound
			},
		})

		req := &pluginv3.CallRouteRequest{}
		req.SetUrl("https://example.com/foo")

		stream := newStream()
		err := a.CallRoute(req, stream)
		if !errors.Is(err, app.ErrCustomRouteNotFound) || status.Code(err) == codes.NotFound {
			t.Fatalf("expected the unmapped error, got %v", err)
		}
	})

	t.Run("rejects an unparsable URL", func(t *testing.T) {
		a := NewRouteAdapter(&fakeApp{})

		req := &pluginv3.CallRouteRequest{}
		req.SetUrl("://not-a-url")

		stream := newStream()
		err := a.CallRoute(req, stream)
		require.NoError(t, err)
		require.Len(t, stream.sent, 1)
		require.Equal(t, int32(http.StatusBadRequest), stream.sent[0].GetCode())
	})
}

func TestRestfulRouteAdapter_FullURL(t *testing.T) {
	for _, tt := range []struct {
		name      string
		route     string
		url       string
		path      string
		namespace string
		parent    *pluginv3.RouteResource
	}{
		{name: "cluster", route: "/foo", url: "/apis/test.grafana.app/v1alpha1/foo?query=a%2Bb&query=c", path: "foo"},
		{name: "namespaced", route: "/namespaces/{namespace}/bar", url: "/apis/test.grafana.app/v1alpha1/namespaces/default/bar?query=a%2Bb&query=c", path: "bar", namespace: "default"},
		{name: "resource", route: "/namespaces/{namespace}/things/{name}/baz", url: "/apis/test.grafana.app/v1alpha1/namespaces/default/things/example/baz?query=a%2Bb&query=c", path: "baz", namespace: "default", parent: pluginv3.RouteResource_builder{Resource: new("things"), Name: new("example")}.Build()},
		{name: "escaped path", route: "/foo/{value}", url: "/apis/test.grafana.app/v1alpha1/foo/a%2Bb?query=a%2Bb&query=c", path: "foo/{value}"},
		{name: "absolute URL", route: "/foo", url: "https://example.com/apis/test.grafana.app/v1alpha1/foo?query=a%2Bb&query=c", path: "foo"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			ws := new(restful.WebService).Path("/apis/test.grafana.app/v1alpha1")
			ws.Route(ws.POST(tt.route).To(func(r *restful.Request, w *restful.Response) {
				called = true
				require.Equal(t, tt.url, r.Request.URL.String())
				require.Equal(t, []string{"a+b", "c"}, r.Request.URL.Query()["query"])
				require.Equal(t, "example", r.Request.Header.Get("X-Test"))
				body, err := io.ReadAll(r.Request.Body)
				require.NoError(t, err)
				require.Equal(t, "payload", string(body))
				info := resource.RouteRequestInfoFrom(r.Request.Context())
				require.NotNil(t, info)
				require.Equal(t, "test.grafana.app", info.Group)
				require.Equal(t, "v1alpha1", info.Version)
				require.Equal(t, tt.namespace, info.Namespace)
				if tt.parent != nil {
					require.Equal(t, tt.parent.GetName(), info.Name)
					require.Equal(t, tt.parent.GetResource(), info.Plural)
				}
				w.WriteHeader(http.StatusCreated)
				_, err = w.Write([]byte("ok"))
				require.NoError(t, err)
			}))
			adapter := &restfulRouteAdapter{handler: restful.NewContainer().Add(ws)}
			req := pluginv3.CallRouteRequest_builder{
				Group: new("test.grafana.app"), Version: new("v1alpha1"),
				Namespace: &tt.namespace, Parent: tt.parent, Path: &tt.path,
				Method: new(http.MethodPost), Url: &tt.url, Body: []byte("payload"),
				Headers: map[string]*pluginv3.StringList{"X-Test": pluginv3.StringList_builder{Values: []string{"example"}}.Build()},
			}.Build()
			stream := newStream()
			require.NoError(t, adapter.CallRoute(req, stream))
			require.True(t, called, "full API route was not reached")
			require.Len(t, stream.sent, 1)
			require.Equal(t, int32(http.StatusCreated), stream.sent[0].GetCode())
			require.Equal(t, "ok", string(stream.sent[0].GetBody()))
		})
	}
}
