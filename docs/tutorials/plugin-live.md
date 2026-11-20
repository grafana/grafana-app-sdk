# Serve Grafana Live alongside App SDK routes

An existing backend app plugin can keep its Grafana Live channels while adopting an App SDK manifest and custom routes. Set `"backend": true` and `"streaming": true` in plugin.json. Pass the existing backend instance factory and the manifest provider to the same entry point:

```go
err := plugin.Run(provider,
    plugin.WithPluginID("example-live-app"),
    plugin.WithAppFunc(newLiveInstance),
)
```

Here `plugin` is `github.com/grafana/grafana-app-sdk/plugin`. The provider handles the manifest's v3 routes. The instance returned by the factory implements the Go plugin SDK's `backend.StreamHandler`. Grafana calls its three methods:

- `SubscribeStream` checks whether this caller may receive the channel, including when another subscriber has already opened it.
- `RunStream` sends frames as they become available and releases upstream resources when its context ends.
- `PublishStream` authorizes each input and returns the data that Grafana may publish back to subscribers. Return only data intended for those subscribers.

The [working backend](testdata/live-plugin/main.go) exposes a namespaced `GET /channel` route that returns the same relative address in each namespace. Grafana scopes Live channels to the caller’s organization. Run `go test ./docs/tutorials/testdata/live-plugin` to exercise route discovery, stream output, acknowledgement, and cancellation.

To build a backend bundle for a local plugin operator, run the following from the SDK repository root. The operator loads the plugin metadata and the versioned AppManifest from the bundle directory. The backend embeds the same AppManifest for its route handler.

```sh
bundle=$(mktemp -d)/example-live-app
mkdir -p "$bundle"
cp docs/tutorials/testdata/live-plugin/plugin.json docs/tutorials/testdata/live-plugin/app-sdk-manifest.json "$bundle/"
cp -R docs/tutorials/testdata/live-plugin/img "$bundle/"
go build -o "$bundle/live_example_$(go env GOOS)_$(go env GOARCH)" ./docs/tutorials/testdata/live-plugin
```

Use an operator build containing v2 Live forwarding ([Enterprise #15081](https://github.com/grafana/grafana-enterprise/pull/15081)). In the operator configuration, add the example to the configured plugin list, set its plugins path to the bundle’s parent directory and enable forwarding:

```ini
[cloud_plugin_operator]
plugins = example-live-app
plugins_path = /path/to/bundles
dev_mode = true

[cloud_plugin_operator.plugins.example-live-app]
serve_live = true
```

In the calling Grafana instance, install the plugin metadata and configure its remote backend. Substitute the operator address reachable from Grafana:

```ini
[plugin.example-live-app]
remote_server_addr = <operator-host>:<grpc-port>
```

Use this setup for isolated local development. Shared deployments require a connection that authenticates Grafana and binds it to the permitted stack.

For channel `plugin/example-live-app/events`, use the same address for receiving and sending:

```ts
import { isLiveChannelStatusEvent, LiveChannelConnectionState, LiveChannelScope } from '@grafana/data';
import { getGrafanaLiveSrv } from '@grafana/runtime';

const address = { scope: LiveChannelScope.Plugin, stream: 'example-live-app', path: 'events' };
const live = getGrafanaLiveSrv();
let connected = false;
const subscription = live.getStream(address).subscribe(event => {
  if (isLiveChannelStatusEvent(event)) {
    connected = event.state === LiveChannelConnectionState.Connected;
  }
});
const onClick = () => {
  if (connected) return live.publish(address, { type: 'ping' }, { useSocket: true });
};
const onUnmount = () => subscription.unsubscribe();
```

Check the Live service type in your target Grafana release for the socket publish option. Remote hosting requires an operator that forwards the v2 Stream service as well as the v3 services. The Cloud operator configuration gives gRPC connections a five-minute maximum age and a thirty-second grace period. Grafana can restart `RunStream` after rotation or shutdown. Release resources on cancellation and design for gaps across reconnects.

With multiple replicas, subscribe, run and publish may reach different processes. Keep handlers stateless or store shared state outside the plugin process. A single replica with autoscaling disabled can support process-local state during a trial, but a restart still loses that state.
