package plugin

import (
	"context"
	"net/http"

	pluginv3 "github.com/grafana/grafana-app-sdk/plugin-next/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// Make sure App implements required interfaces. This is important to do
// since otherwise we will only get a not implemented error response from plugin in
// runtime. Plugin should not implement all these interfaces - only those which are
// required for a particular task.
var (
	_ backend.CallResourceHandler   = (*ManagedApp)(nil)
	_ instancemgmt.InstanceDisposer = (*ManagedApp)(nil)
	_ backend.CheckHealthHandler    = (*ManagedApp)(nil)

	_ pluginv3.AdmissionServiceServer  = (*App)(nil)
	_ pluginv3.ConversionServiceServer = (*App)(nil)
)

// Single instance for everything
type App struct{}

// Managed app is created for each namespace/instance that is seen
type ManagedApp struct {
	backend.CallResourceHandler
}

// NewManagedApp creates a new example *App instance.
func NewManagedApp(_ context.Context, _ backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	var app ManagedApp

	// Use a httpadapter (provided by the SDK) for resource calls. This allows us
	// to use a *http.ServeMux for resource calls, so we can map multiple routes
	// to CallResource without having to implement extra logic.
	mux := http.NewServeMux()
	app.registerRoutes(mux)
	app.CallResourceHandler = httpadapter.New(mux)

	return &app, nil
}

// Dispose here tells plugin SDK that plugin wants to clean up resources when a new instance
// created.
func (_ *ManagedApp) Dispose() {
	// cleanup
}

// CheckHealth handles health checks sent from Grafana to the plugin.
func (_ *ManagedApp) CheckHealth(_ context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "ok",
	}, nil
}
