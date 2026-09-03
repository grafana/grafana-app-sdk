// Package appadapter implements the process-wide grafana.plugin.v3 services
// (admission, conversion, and routes) in terms of an app-sdk App. These
// services are registered once per plugin process via grpcplugin.ServeOpts.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
package appadapter

import (
	plugin "github.com/hashicorp/go-plugin"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"
)

// New is a convenience function to create the three service adapters,
// and return the plugin.PluginSet for use in the plugin backend ManageOpts.
//
//	ManageOpts{ExtraPlugins: appadapter.New(a)}
func New(a app.App) plugin.PluginSet {
	opts := grpcplugin.ServeOpts{
		RouteServer:      NewRouteAdapter(a),
		AdmissionServer:  NewAdmissionAdapter(a),
		ConversionServer: NewConversionAdapter(a),
	}

	return opts.PluginSet()
}
