package appadapter

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

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
	ctx  context.Context
	sent []*pluginv3.CallRouteResponse
}

func (s *fakeStream) Send(rsp *pluginv3.CallRouteResponse) error {
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

	t.Run("rejects an unparsable URL", func(t *testing.T) {
		a := NewRouteAdapter(&fakeApp{})

		req := &pluginv3.CallRouteRequest{}
		req.SetUrl("://not-a-url")

		stream := newStream()
		err := a.CallRoute(req, stream)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument status, got %v", err)
		}
	})
}
