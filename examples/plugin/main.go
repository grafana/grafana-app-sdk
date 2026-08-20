package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/grafana/grafana-app-sdk/examples/plugin/plugin"
	"github.com/grafana/grafana-app-sdk/plugin-next/grpcplugin"
	"github.com/grafana/grafana-app-sdk/plugin-next/httpadapter"
	"github.com/grafana/grafana-plugin-sdk-go/backend/app"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

func main() {
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// multi-tenant app
	mt := &plugin.App{} // stubs for admission and conversion
	opts := grpcplugin.ServeOpts{
		RouteServer:      httpadapter.New(echo),
		AdmissionServer:  mt,
		ConversionServer: mt,
	}
	fmt.Printf("ooo %v\n", opts)

	// This starts BOTH a managed app, and standalone MT handlers
	if err := app.Manage("sdk-example-app", plugin.NewManagedApp, app.ManageOpts{
		ExtraPlugins: opts.PluginSet(), // attach the extra plugins
	}); err != nil {
		log.DefaultLogger.Error(err.Error())
		os.Exit(1)
	}
}
