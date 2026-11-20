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

The [working backend](live-plugin/main.go) exposes a namespaced `GET /channel` route and a shared demonstration channel. Run `go test ./docs/tutorials/live-plugin` to exercise route discovery, stream output, acknowledgement, and cancellation.

For channel `plugin/example-live-app/events`, use the same address for receiving and sending:

```ts
const address = { scope: LiveChannelScope.Plugin, stream: 'example-live-app', path: 'events' };
const live = getGrafanaLiveSrv();
const subscription = live.getStream(address).subscribe(onEvent);
// Call onClick after the subscription reports ready.
const onClick = () => live.publish(address, { type: 'ping' }, { useSocket: true });
// Call onUnmount when leaving the page.
const onUnmount = () => subscription.unsubscribe();
```

Check the Live service type in your target Grafana release for the socket publish option. Remote hosting requires an operator that forwards the v2 Stream service as well as the v3 services. For stateful plugins, route session creation, subscription, input and other session operations to the process holding that session.
