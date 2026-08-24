package plugin

import (
	"context"
	"net/http"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// Compile-time checks keep interface mismatches from becoming runtime
// "unimplemented" responses. Plugins only need to implement the services they
// register.
var (
	_ backend.CallResourceHandler   = (*ManagedApp)(nil)
	_ instancemgmt.InstanceDisposer = (*ManagedApp)(nil)
	_ backend.CheckHealthHandler    = (*ManagedApp)(nil)

	_ pluginv3.AdmissionServiceServer  = (*App)(nil)
	_ pluginv3.ConversionServiceServer = (*App)(nil)
)

// App implements the process-wide v3 admission and conversion services.
type App struct {
	grpcplugin.UnimplementedV3Server
}

// ManagedApp is created once for each Grafana app instance.
type ManagedApp struct {
	backend.CallResourceHandler
}

// NewManagedApp creates a managed app instance.
func NewManagedApp(_ context.Context, _ backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	var app ManagedApp

	// The existing plugin SDK adapter lets a ServeMux handle CallResource.
	mux := http.NewServeMux()
	app.registerRoutes(mux)
	app.CallResourceHandler = httpadapter.New(mux)

	return &app, nil
}

// Dispose releases resources owned by this managed app instance.
func (*ManagedApp) Dispose() {
	// cleanup
}

// CheckHealth handles health checks sent from Grafana to the plugin.
func (*ManagedApp) CheckHealth(_ context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "ok",
	}, nil
}
