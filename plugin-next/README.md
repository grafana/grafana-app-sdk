# Plugin protocol v3

> [!WARNING]
> This package is experimental. Its API and wire protocol may change before it
> replaces the existing `plugin` module.

`plugin-next` connects Grafana App Platform services to a Go backend plugin over
gRPC. It currently defines admission, conversion, and HTTP-style route services.

## Packages

- `genproto/grafana/plugin/v3` contains generated protobuf and gRPC types.
- `grpcplugin` integrates the v3 services with HashiCorp `go-plugin`.
- `httpadapter` adapts an `http.Handler` to the streaming v3 route service.

## Serving v3 services

Implement the generated service interfaces directly. Embedding
`grpcplugin.UnimplementedV3Server` supplies forward-compatible unimplemented
methods for services your plugin does not override.

Attach the services to the Grafana plugin SDK through `ExtraPlugins`:

```go
type server struct {
	grpcplugin.UnimplementedV3Server
}

srv := &server{}
extra := (grpcplugin.ServeOpts{
	AdmissionServer:  srv,
	ConversionServer: srv,
	RouteServer:      httpadapter.New(http.DefaultServeMux),
}).PluginSet()

err := app.Manage("my-app", newApp, app.ManageOpts{
	ExtraPlugins: extra,
})
```

The `httpadapter` preserves the request context supplied by gRPC. For a
subresource route, call `httpadapter.ParentFromContext` to access the parent
resource.

## Creating clients

Use `grpcplugin.ClientPluginSet()` in the host's `go-plugin` client
configuration. After protocol negotiation, pass the resulting
`plugin.ClientProtocol` to `grpcplugin.NewClientV3` to dispense typed clients for
all three services.

The protobuf sources and regeneration instructions are in [`../proto`](../proto/README.md).
