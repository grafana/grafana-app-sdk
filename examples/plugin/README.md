# Plugin protocol v3 example

> [!WARNING]
> Plugin protocol v3 is a work in progress. Its APIs and wire format may change
> or be removed without notice. Use it only for experimentation.

This example runs the existing per-instance Grafana app backend together with
the `grafana.plugin.v3` services in one `go-plugin` process.

It demonstrates:

- attaching additional gRPC services through `app.ManageOpts.ExtraPlugins`;
- adapting an `http.Handler` to the streaming v3 route service; and
- implementing admission and conversion services with generated protobuf types.

Run the example from this directory:

```sh
go run .
```

The process uses the Grafana plugin protocol and is normally launched by
Grafana, rather than invoked directly by an end user.
