package grpcplugin

import (
	plugin "github.com/hashicorp/go-plugin"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
)

// ServeOpts contains options for serving plugins.
type ServeOpts struct {
	ValidationServer pluginv3.ValidateServiceServer
	MutationServer   pluginv3.MutateServiceServer
	ConversionServer pluginv3.ConversionServiceServer
	RouteServer      pluginv3.RouteServiceServer
}

// pluginSet builds the go-plugin PluginSet from the given options. It is shared
// by Serve and by tests that exercise the go-plugin dispensing path in-process.
func (opts ServeOpts) PluginSet() plugin.PluginSet {
	pSet := make(plugin.PluginSet)

	if opts.ValidationServer != nil {
		pSet["v3-validate"] = &validateGRPCPlugin{server: opts.ValidationServer}
	}
	if opts.MutationServer != nil {
		pSet["v3-mutate"] = &mutateGRPCPlugin{server: opts.MutationServer}
	}
	if opts.ConversionServer != nil {
		pSet["v3-convert"] = &conversionGRPCPlugin{server: opts.ConversionServer}
	}
	if opts.RouteServer != nil {
		pSet["v3-route"] = &routeGRPCPlugin{server: opts.RouteServer}
	}

	return pSet
}
