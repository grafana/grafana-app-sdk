package appadapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/health"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/resource"
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

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"route not found", app.ErrCustomRouteNotFound},
		{"wrapped route not found", fmt.Errorf("lookup route: %w", app.ErrCustomRouteNotFound)},
	} {
		t.Run(tc.name+" returns an HTTP 404 response", func(t *testing.T) {
			a := NewRouteAdapter(&fakeApp{
				callCustomRoute: func(context.Context, app.CustomRouteResponseWriter, *app.CustomRouteRequest) error {
					return tc.err
				},
			})
			req := &pluginv3.CallRouteRequest{}
			req.SetUrl("https://example.com/foo")

			stream := newStream()
			require.NoError(t, a.CallRoute(req, stream))
			require.Len(t, stream.sent, 1)
			require.Equal(t, int32(http.StatusNotFound), stream.sent[0].GetCode())
			require.Equal(t, "not found", string(stream.sent[0].GetBody()))
			require.Empty(t, stream.sent[0].GetHeaders())
		})
	}

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
		require.ErrorIs(t, err, app.ErrCustomRouteNotFound)
		// An already-streamed response cannot be replaced with an HTTP error.
		require.Len(t, stream.sent, 1)
		require.Equal(t, int32(http.StatusOK), stream.sent[0].GetCode())
		require.Equal(t, "partial", string(stream.sent[0].GetBody()))
	})

	t.Run("returns an HTTP 400 response for an unparsable URL", func(t *testing.T) {
		a := NewRouteAdapter(&fakeApp{
			callCustomRoute: func(context.Context, app.CustomRouteResponseWriter, *app.CustomRouteRequest) error {
				t.Fatal("app must not be called for an invalid URL")
				return nil
			},
		})

		req := &pluginv3.CallRouteRequest{}
		req.SetUrl("://not-a-url")

		stream := newStream()
		require.NoError(t, a.CallRoute(req, stream))
		require.Len(t, stream.sent, 1)
		require.Equal(t, int32(http.StatusBadRequest), stream.sent[0].GetCode())
		require.Contains(t, string(stream.sent[0].GetBody()), "://not-a-url")
		require.Contains(t, string(stream.sent[0].GetBody()), "missing protocol scheme")
		require.Empty(t, stream.sent[0].GetHeaders())
	})
}

func TestRouteAdapter_ErrorResponseSendFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
	}{
		{"invalid URL", "://not-a-url"},
		{"route not found", "https://example.com/missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewRouteAdapter(&fakeApp{
				callCustomRoute: func(context.Context, app.CustomRouteResponseWriter, *app.CustomRouteRequest) error {
					return app.ErrCustomRouteNotFound
				},
			})
			req := &pluginv3.CallRouteRequest{}
			req.SetUrl(tc.url)
			stream := newStream()
			stream.sendErr = errors.New("stream closed")
			require.ErrorIs(t, a.CallRoute(req, stream), stream.sendErr)
			require.Empty(t, stream.sent)
		})
	}
}

func TestNotFoundAdapter_CallRoute(t *testing.T) {
	adapter := &notFoundAdapter{}
	stream := newStream()
	require.NoError(t, adapter.CallRoute(&pluginv3.CallRouteRequest{}, stream))
	require.Len(t, stream.sent, 1)
	require.Equal(t, int32(http.StatusNotFound), stream.sent[0].GetCode())
	require.Empty(t, stream.sent[0].GetBody())
	require.Empty(t, stream.sent[0].GetHeaders())

	stream = newStream()
	stream.sendErr = errors.New("stream closed")
	require.ErrorIs(t, adapter.CallRoute(&pluginv3.CallRouteRequest{}, stream), stream.sendErr)
	require.Empty(t, stream.sent)
}

func TestRouteAdapter_RequestInfo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		parent bool
		secure map[string]string
	}{
		{name: "no parent"},
		{name: "parent without secure values", parent: true},
		{name: "parent with secure values", parent: true, secure: map[string]string{"token": "secret", "password": "another-secret"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &pluginv3.CallRouteRequest{}
			req.SetGroup("test.grafana.app")
			req.SetVersion("v1alpha1")
			req.SetNamespace("default")
			want := &resource.RouteRequestInfo{
				FullIdentifier: resource.FullIdentifier{Group: "test.grafana.app", Version: "v1alpha1", Namespace: "default"},
			}
			if tc.parent {
				parent := &pluginv3.RouteResource{}
				parent.SetResource("things")
				parent.SetName("example")
				parent.SetRv("42")
				parent.SetRaw([]byte(`{"spec":{"value":"example"}}`))
				parent.SetDecryptedSecureValues(tc.secure)
				req.SetParent(parent)
				want.Plural = "things"
				want.Name = "example"
				want.ResourceVersion = "42"
				want.Parent = parent.GetRaw()
				if tc.secure != nil {
					want.DecryptedSecureValues = resource.DecryptedSecureValues{
						"token": "secret", "password": "another-secret",
					}
				}
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			called := false
			a := NewRouteAdapter(&fakeApp{
				callCustomRoute: func(ctx context.Context, _ app.CustomRouteResponseWriter, req *app.CustomRouteRequest) error {
					called = true
					require.Equal(t, want, resource.RouteRequestInfoFrom(ctx))
					require.Equal(t, want.FullIdentifier, req.ResourceIdentifier)
					require.Equal(t, want.DecryptedSecureValues, req.DecryptedSecureValues)
					if tc.parent {
						require.NotNil(t, req.Parent)
						require.Equal(t, []byte(want.Parent), req.Parent.Raw)
					} else {
						require.Nil(t, req.Parent)
					}
					cancel()
					require.ErrorIs(t, ctx.Err(), context.Canceled)
					return nil
				},
			})
			stream := newStream()
			stream.ctx = ctx
			require.NoError(t, a.CallRoute(req, stream))
			require.True(t, called)
			require.Len(t, stream.sent, 1)
			require.Equal(t, int32(http.StatusOK), stream.sent[0].GetCode())
		})
	}
}

func TestRouteAdapter_StatusCodeBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int64
		want int32
	}{
		{"negative", -1, http.StatusInternalServerError},
		{"maximum int32", math.MaxInt32, math.MaxInt32},
		{"overflow", int64(math.MaxInt32) + 1, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.code > math.MaxInt {
				t.Skip("status code cannot be represented by int on this platform")
			}
			a := NewRouteAdapter(&fakeApp{
				callCustomRoute: func(_ context.Context, w app.CustomRouteResponseWriter, _ *app.CustomRouteRequest) error {
					w.WriteHeader(int(tc.code))
					_, err := w.Write([]byte("response"))
					return err
				},
			})
			stream := newStream()
			require.NoError(t, a.CallRoute(&pluginv3.CallRouteRequest{}, stream))
			require.Len(t, stream.sent, 1)
			require.Equal(t, tc.want, stream.sent[0].GetCode())
			require.Equal(t, "response", string(stream.sent[0].GetBody()))
		})
	}
}

func TestRouteAdapter_RequestHeaders(t *testing.T) {
	called := false
	a := NewRouteAdapter(&fakeApp{
		callCustomRoute: func(_ context.Context, writer app.CustomRouteResponseWriter, req *app.CustomRouteRequest) error {
			called = true
			require.Equal(t, "application/json", req.Headers.Get("Content-Type"))
			require.Equal(t, []string{"one", "two"}, req.Headers.Values("X-Test"))
			require.NotContains(t, req.Headers, "X-Nil")
			// Mutating the handler's headers must not change the protocol request.
			req.Headers["Content-Type"][0] = "text/plain"
			req.Headers["X-Test"][0] = "changed"
			writer.WriteHeader(http.StatusNoContent)
			return nil
		},
	})
	header := &pluginv3.StringList{}
	header.SetValues([]string{"application/json"})
	values := &pluginv3.StringList{}
	values.SetValues([]string{"one", "two"})
	req := &pluginv3.CallRouteRequest{}
	req.SetHeaders(map[string]*pluginv3.StringList{
		"content-type": header,
		"x-test":       values,
		"X-Nil":        nil,
	})
	stream := newStream()
	require.NoError(t, a.CallRoute(req, stream))
	require.True(t, called)
	require.Equal(t, []string{"application/json"}, header.GetValues())
	require.Equal(t, []string{"one", "two"}, values.GetValues())
	require.Len(t, stream.sent, 1)
	require.Equal(t, int32(http.StatusNoContent), stream.sent[0].GetCode())
}

func TestRouteAdapter_ResponseCommit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		write  func(http.ResponseWriter)
		status int
	}{
		{"explicit status", func(w http.ResponseWriter) { w.WriteHeader(http.StatusCreated) }, http.StatusCreated},
		{"implicit status", func(w http.ResponseWriter) { _, _ = w.Write([]byte("body")) }, http.StatusOK},
		{"flush", func(w http.ResponseWriter) { w.(http.Flusher).Flush() }, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewRouteAdapter(&fakeApp{
				callCustomRoute: func(_ context.Context, w app.CustomRouteResponseWriter, _ *app.CustomRouteRequest) error {
					w.Header().Set("X-Test", "original")
					tc.write(w)
					w.Header().Set("X-Test", "changed")
					w.WriteHeader(http.StatusInternalServerError)
					return nil
				},
			})
			stream := newStream()
			require.NoError(t, a.CallRoute(&pluginv3.CallRouteRequest{}, stream))
			require.Len(t, stream.sent, 1)
			require.Equal(t, int32(tc.status), stream.sent[0].GetCode())
			require.Equal(t, []string{"original"}, stream.sent[0].GetHeaders()["X-Test"].GetValues())
		})
	}
}
