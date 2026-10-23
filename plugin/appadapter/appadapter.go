// Package appadapter implements the process-wide grafana.plugin.v3 services
// (admission, conversion, and routes) in terms of an app-sdk App. These
// services are registered once per plugin process via grpcplugin.ServeOpts.
//
// Experimental: Plugin protocol v3 is a work in progress and may change or be
// removed without notice.
package appadapter

import (
	"github.com/grafana/authlib/authn"
	plugin "github.com/hashicorp/go-plugin"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"
)

// New is a convenience function to create the three service adapters,
// and return the plugin.PluginSet for use in the plugin backend ManageOpts.
// The authenticator verifies each request's access token (see
// grpcplugin.ServeOpts.Authenticator). If it is nil, every request is rejected.
//
//	ManageOpts{ExtraPlugins: appadapter.New(a, authenticator)}
func New(a app.App, authenticator authn.Authenticator) plugin.PluginSet {
	opts := ServeOpts(a)
	opts.Authenticator = authenticator
	return opts.PluginSet()
}

// ServeOpts returns grpcplugin.ServeOpts serving the three service adapters for a.
// Callers must set its Authenticator, or explicitly skip authentication.
func ServeOpts(a app.App) grpcplugin.ServeOpts {
	return grpcplugin.ServeOpts{
		RouteServer:      NewRouteAdapter(a),
		AdmissionServer:  NewAdmissionAdapter(a),
		ConversionServer: NewConversionAdapter(a),
	}
}
