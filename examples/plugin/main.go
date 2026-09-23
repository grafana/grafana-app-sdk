package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/grafana/grafana-app-sdk/examples/plugin/plugin"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"
	"github.com/grafana/grafana-app-sdk/plugin/httpadapter"

	"github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

func main() {
	// Serve every v3 route with a small handler that echoes request metadata.
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg := map[string]any{
			"method": r.Method,
			"url":    r.URL.String(),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(msg); err != nil {
			log.DefaultLogger.Error("write route response", "error", err)
		}
	})

	// Admission and conversion are process-wide services; the managed app below
	// remains scoped to each Grafana app instance.
	mt := &plugin.App{}
	opts := grpcplugin.ServeOpts{
		RouteServer:      httpadapter.NewServer(echo),
		AdmissionServer:  mt,
		ConversionServer: mt,
	}

	// Start both the managed app and the process-wide v3 services.
	if err := app.Manage("sdk-example-app", plugin.NewManagedApp, app.ManageOpts{
		ExtraPlugins: opts.PluginSet(), // attach the extra plugins
	}); err != nil {
		log.DefaultLogger.Error(err.Error())
		os.Exit(1)
	}
}
