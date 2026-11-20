package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
)

type packets chan []byte

func (p packets) Send(packet *backend.StreamPacket) error { p <- packet.Data; return nil }

func TestChannelRouteAndStream(t *testing.T) {
	p, err := provider()
	require.NoError(t, err)
	require.NoError(t, p.Manifest().ManifestData.Validate())
	a, err := p.NewApp(app.Config{ManifestData: *p.Manifest().ManifestData})
	require.NoError(t, err)
	h, err := a.(app.RouteHandlerProvider).ProvideRouteHandler()
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/apis/liveexample.ext.grafana.app/v1alpha1/namespaces/demo/channel", nil))
	require.Equal(t, 200, rec.Code)
	require.JSONEq(t, `"plugin/example-live-app/events"`, rec.Body.String())
	e := events{}
	pc := backend.PluginContext{User: &backend.User{Login: "learner"}}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	sub, err := e.SubscribeStream(ctx, &backend.SubscribeStreamRequest{Path: path, PluginContext: pc})
	require.NoError(t, err)
	require.Equal(t, backend.SubscribeStreamStatusOK, sub.Status)
	sink := make(packets, 1)
	done := make(chan error, 1)
	go func() {
		done <- e.RunStream(ctx, &backend.RunStreamRequest{PluginContext: pc, Path: path}, backend.NewStreamSender(sink))
	}()
	select {
	case pkt := <-sink:
		require.JSONEq(t, `{"event":"ready"}`, string(pkt))
	case <-ctx.Done():
		t.Fatal("ready packet missing")
	}
	pub, err := e.PublishStream(ctx, &backend.PublishStreamRequest{PluginContext: pc, Path: path, Data: []byte(`"hello"`)})
	require.NoError(t, err)
	require.JSONEq(t, `{"event":"ack"}`, string(pub.Data))
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	denied, err := e.PublishStream(ctx, &backend.PublishStreamRequest{Path: path})
	require.NoError(t, err)
	require.Equal(t, backend.PublishStreamStatusPermissionDenied, denied.Status)
}

func TestLatencySubscription(t *testing.T) {
	e := events{}
	pc := backend.PluginContext{User: &backend.User{Login: "viewer"}}
	req := &backend.SubscribeStreamRequest{Path: metrics, PluginContext: pc}
	sub, err := e.SubscribeStream(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, backend.SubscribeStreamStatusOK, sub.Status)
	require.NotNil(t, sub.InitialData)
	var frame struct{ Data struct{ Values [][]float64 } }
	require.NoError(t, json.Unmarshal(sub.InitialData.Data(), &frame))
	require.Len(t, frame.Data.Values, 3)
	for _, values := range frame.Data.Values {
		require.Len(t, values, 120)
	}
	for i := 1; i < len(frame.Data.Values[0]); i++ {
		require.Equal(t, float64(sampleInterval.Milliseconds()), frame.Data.Values[0][i]-frame.Data.Values[0][i-1])
		require.Greater(t, frame.Data.Values[2][i], frame.Data.Values[1][i])
	}
	pub, err := e.PublishStream(t.Context(), &backend.PublishStreamRequest{Path: metrics, PluginContext: pc})
	require.NoError(t, err)
	require.Equal(t, backend.PublishStreamStatusPermissionDenied, pub.Status)
}
