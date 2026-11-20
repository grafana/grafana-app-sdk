// Example backend for an App SDK plugin with a namespaced discovery route
// and a tenant-wide demonstration channel for signed-in users.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"

	"github.com/grafana/grafana-app-sdk/app"
	manifestv1alpha2 "github.com/grafana/grafana-app-sdk/app/appmanifest/v1alpha2"
	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/grafana/grafana-app-sdk/simple"
)

const id = "example-live-app"
const path = "events"

//go:embed app-sdk-manifest.json
var manifestJSON []byte

type events struct{}

var _ backend.StreamHandler = events{}

func allowed(pc backend.PluginContext, channel string) bool {
	return pc.User != nil && pc.User.Login != "" && channel == path
}
func (events) SubscribeStream(_ context.Context, req *backend.SubscribeStreamRequest) (*backend.SubscribeStreamResponse, error) {
	status := backend.SubscribeStreamStatusPermissionDenied
	if allowed(req.PluginContext, req.Path) {
		status = backend.SubscribeStreamStatusOK
	}
	return &backend.SubscribeStreamResponse{Status: status}, nil
}
func (events) PublishStream(_ context.Context, req *backend.PublishStreamRequest) (*backend.PublishStreamResponse, error) {
	if !allowed(req.PluginContext, req.Path) {
		return &backend.PublishStreamResponse{Status: backend.PublishStreamStatusPermissionDenied}, nil
	}
	// Broadcast a fixed acknowledgement. Input can include private data.
	return &backend.PublishStreamResponse{Status: backend.PublishStreamStatusOK, Data: json.RawMessage(`{"event":"ack"}`)}, nil
}
func (events) RunStream(ctx context.Context, req *backend.RunStreamRequest, sender *backend.StreamSender) error {
	if !allowed(req.PluginContext, req.Path) {
		return errors.New("access denied")
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
	data, err := cr.Spec.ToManifestData()
	if err != nil {
		return nil, err
	}
	manifest := app.Manifest{Location: app.ManifestLocation{Type: app.ManifestLocationEmbedded}, ManifestData: &data}
	return simple.NewAppProvider(manifest, nil, func(cfg app.Config) (app.App, error) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /apis/"+data.Group+"/v1alpha1/namespaces/{namespace}/channel", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode("plugin/" + id + "/" + path)
		})
		return simple.NewApp(simple.AppConfig{Name: data.AppName, KubeConfig: cfg.KubeConfig, Handler: mux})
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
