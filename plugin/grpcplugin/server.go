package grpcplugin

import (
	"context"

	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
)

// ServeOpts contains options for serving plugins. When at least one service is
// configured, PluginSet registers all v3 service names and uses unimplemented
// stubs for omitted services so clients can negotiate the v3 protocol as a
// unit.
type ServeOpts struct {
	AdmissionServer  pluginv3.AdmissionServiceServer
	ConversionServer pluginv3.ConversionServiceServer
	RouteServer      pluginv3.RouteServiceServer
}

const (
	pluginKeyAdmission  = "v3-admit"
	pluginKeyConversion = "v3-convert"
	pluginKeyRouter     = "v3-route"
)

// PluginSet returns the go-plugin server registrations configured in opts.
// Pass the result as backend.ServeOpts.ExtraPlugins or
// backend/app.ManageOpts.ExtraPlugins.
func (opts ServeOpts) PluginSet() plugin.PluginSet {
	pSet := make(plugin.PluginSet)
	if opts.AdmissionServer == nil && opts.ConversionServer == nil && opts.RouteServer == nil {
		return pSet
	}

	fallback := &UnimplementedV3Server{}
	admissionServer := opts.AdmissionServer
	if admissionServer == nil {
		admissionServer = fallback
	}
	conversionServer := opts.ConversionServer
	if conversionServer == nil {
		conversionServer = fallback
	}
	routeServer := opts.RouteServer
	if routeServer == nil {
		routeServer = fallback
	}

	pSet[pluginKeyAdmission] = &admissionGRPCPlugin{server: admissionServer}
	pSet[pluginKeyConversion] = &conversionGRPCPlugin{server: conversionServer}
	pSet[pluginKeyRouter] = &routeGRPCPlugin{server: routeServer}

	return pSet
}

// ClientPluginSet returns the client-side go-plugin registrations needed to
// negotiate and dispense all grafana.plugin.v3 services.
func ClientPluginSet() plugin.PluginSet {
	return plugin.PluginSet{
		pluginKeyAdmission:  &admissionGRPCPlugin{},
		pluginKeyConversion: &conversionGRPCPlugin{},
		pluginKeyRouter:     &routeGRPCPlugin{},
	}
}

// V3Server is implemented by plugins that serve the grafana.plugin.v3 API — the
// modern successor to the legacy genproto/pluginv2 (backend.proto) contract.
//
// Unlike the legacy services (Data, Resource, Diagnostics, ...), the V3 service
// contracts are the generated gRPC interfaces themselves: there is
// intentionally no hand-written Go wrapper translating between the protobuf
// types and an SDK-native type. Implementations embed UnimplementedV3Server and
// override the RPCs they support.
type V3Server interface {
	pluginv3.AdmissionServiceServer
	pluginv3.ConversionServiceServer
	pluginv3.RouteServiceServer
}

// UnimplementedV3Server is the stub that plugin authors embed to implement the
// grafana.plugin.v3 API. Embedding it makes the V3 opt-in explicit and supplies
// default (gRPC "Unimplemented") handlers for every V3 RPC, so a plugin only
// needs to override the RPCs it actually serves.
type UnimplementedV3Server struct {
	pluginv3.UnimplementedAdmissionServiceServer
	pluginv3.UnimplementedConversionServiceServer
	pluginv3.UnimplementedRouteServiceServer
}

// Compile-time assurance that the stub satisfies the V3 contract.
var _ V3Server = UnimplementedV3Server{}

// The types below are thin go-plugin adapters. go-plugin dispenses plugins by
// name and requires each to implement plugin.GRPCPlugin; the generated code
// only provides Register*Server / New*Client. Each adapter registers the
// generated gRPC service directly — no wrapping server type is inserted.

type admissionGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	plugin.GRPCPlugin
	server pluginv3.AdmissionServiceServer
}

func (p *admissionGRPCPlugin) GRPCServer(_ *plugin.GRPCBroker, s *grpc.Server) error {
	pluginv3.RegisterAdmissionServiceServer(s, p.server)
	return nil
}

func (*admissionGRPCPlugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, c *grpc.ClientConn) (any, error) {
	return pluginv3.NewAdmissionServiceClient(c), nil
}

type conversionGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	plugin.GRPCPlugin
	server pluginv3.ConversionServiceServer
}

func (p *conversionGRPCPlugin) GRPCServer(_ *plugin.GRPCBroker, s *grpc.Server) error {
	pluginv3.RegisterConversionServiceServer(s, p.server)
	return nil
}

func (*conversionGRPCPlugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, c *grpc.ClientConn) (any, error) {
	return pluginv3.NewConversionServiceClient(c), nil
}

type routeGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	plugin.GRPCPlugin
	server pluginv3.RouteServiceServer
}

func (p *routeGRPCPlugin) GRPCServer(_ *plugin.GRPCBroker, s *grpc.Server) error {
	pluginv3.RegisterRouteServiceServer(s, p.server)
	return nil
}

func (*routeGRPCPlugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, c *grpc.ClientConn) (any, error) {
	return pluginv3.NewRouteServiceClient(c), nil
}
