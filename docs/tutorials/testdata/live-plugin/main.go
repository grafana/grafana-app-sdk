// Example backend for an App SDK plugin with a namespaced discovery route
// and a tenant-wide demonstration channel for signed-in users.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/grafana/grafana-app-sdk/app"
	manifestv1alpha2 "github.com/grafana/grafana-app-sdk/app/appmanifest/v1alpha2"
	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/grafana/grafana-app-sdk/simple"
)

// The remote operator integration test uses this plugin ID, API group and ready/ack events.
const id = "example-live-app"
const path = "events"
const metrics = "latency"

//go:embed app-sdk-manifest.json
var manifestJSON []byte

type events struct{}

var _ backend.StreamHandler = events{}

func allowed(pc backend.PluginContext, channel string) bool {
	return pc.User != nil && pc.User.Login != "" && (channel == path || channel == metrics)
}
func (events) SubscribeStream(_ context.Context, req *backend.SubscribeStreamRequest) (*backend.SubscribeStreamResponse, error) {
	status := backend.SubscribeStreamStatusPermissionDenied
	if allowed(req.PluginContext, req.Path) {
		status = backend.SubscribeStreamStatusOK
		if req.Path == metrics {
			// Seed each subscriber with the same simulated signal the ticker emits live.
			now := time.Now().Truncate(sampleInterval)
			frame := latencyFrame()
			for i := 119; i >= 0; i-- {
				appendSample(frame, now.Add(-time.Duration(i)*sampleInterval))
			}
			initial, err := backend.NewInitialFrame(frame, data.IncludeAll)
			return &backend.SubscribeStreamResponse{Status: status, InitialData: initial}, err
		}
	}
	return &backend.SubscribeStreamResponse{Status: status}, nil
}
func (events) PublishStream(_ context.Context, req *backend.PublishStreamRequest) (*backend.PublishStreamResponse, error) {
	if req.Path != path || !allowed(req.PluginContext, req.Path) {
		return &backend.PublishStreamResponse{Status: backend.PublishStreamStatusPermissionDenied}, nil
	}
	// Broadcast a fixed acknowledgement. Input can include private data.
	return &backend.PublishStreamResponse{Status: backend.PublishStreamStatusOK, Data: json.RawMessage(`{"event":"ack"}`)}, nil
}

const sampleInterval = 250 * time.Millisecond

func latencyFrame() *data.Frame {
	return data.NewFrame("Latency", data.NewField("Time", nil, []time.Time{}),
		data.NewField("Median", nil, []float64{}), data.NewField("95th percentile", nil, []float64{}))
}

func appendSample(frame *data.Frame, t time.Time) {
	seconds := float64(t.UnixMilli()%60000) / 1000
	median := 38 + 8*math.Sin(seconds*math.Pi/5) + 3*math.Sin(seconds*math.Pi)
	tail := median + 20 + 65*math.Pow((1+math.Cos(seconds*math.Pi/6))/2, 7)
	frame.AppendRow(t, median, tail)
}

func (events) RunStream(ctx context.Context, req *backend.RunStreamRequest, sender *backend.StreamSender) error {
	if !allowed(req.PluginContext, req.Path) {
		return errors.New("access denied")
	}
	if req.Path == metrics {
		ticker := time.NewTicker(sampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case t := <-ticker.C:
				frame := latencyFrame()
				appendSample(frame, t)
				if err := sender.SendFrame(frame, data.IncludeAll); err != nil {
					return err
				}
			}
		}
	}
	if err := sender.SendJSON([]byte(`{"event":"ready"}`)); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}
func provider() (app.Provider, error) {
	var cr manifestv1alpha2.AppManifest
	if err := json.Unmarshal(manifestJSON, &cr); err != nil {
		return nil, err
	}
	spec, err := cr.Spec.ToManifestData()
	if err != nil {
		return nil, err
	}
	manifest := app.Manifest{Location: app.ManifestLocation{Type: app.ManifestLocationEmbedded}, ManifestData: &spec}
	return simple.NewAppProvider(manifest, nil, func(cfg app.Config) (app.App, error) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /apis/"+spec.Group+"/v1alpha1/namespaces/{namespace}/channel", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode("plugin/" + id + "/" + path)
		})
		return simple.NewApp(simple.AppConfig{Name: spec.AppName, KubeConfig: cfg.KubeConfig, Handler: mux})
	}), nil
}
func run() error {
	p, err := provider()
	if err != nil {
		return err
	}
	return plugin.Run(p, plugin.WithPluginID(id), plugin.WithAppFunc(func(context.Context, backend.AppInstanceSettings) (instancemgmt.Instance, error) {
		return events{}, nil
	}))
}
func main() {
	if err := run(); err != nil {
		log.DefaultLogger.Error("Live plugin stopped", "error", err)
		os.Exit(1)
	}
}
