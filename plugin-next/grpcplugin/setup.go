package grpcplugin

import (
	plugin "github.com/hashicorp/go-plugin"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

// ServeOpts contains options for serving plugins.
type ServeOpts struct {
	AdmissionServer  pluginv3.AdmissionServiceServer
	ConversionServer pluginv3.ConversionServiceServer
	RouteServer      pluginv3.RouteServiceServer
}

// pluginSet builds the go-plugin PluginSet from the given options. It is shared
// by Serve and by tests that exercise the go-plugin dispensing path in-process.
func (opts ServeOpts) PluginSet() plugin.PluginSet {
	pSet := make(plugin.PluginSet)

	if opts.AdmissionServer != nil {
		pSet["v3-admission"] = &admissionGRPCPlugin{server: opts.AdmissionServer}
	}
	if opts.ConversionServer != nil {
		pSet["v3-convert"] = &conversionGRPCPlugin{server: opts.ConversionServer}
	}
	if opts.RouteServer != nil {
		pSet["v3-route"] = &routeGRPCPlugin{server: opts.RouteServer}
	}

	return pSet
}
