# API server integration (deprecated)

The app installer and API server integration from
`github.com/grafana/grafana-app-sdk/k8s/apiserver` have moved to
[`github.com/grafana/grafana/pkg/apiserver/appsdk`](https://github.com/grafana/grafana/tree/main/pkg/apiserver/appsdk)
in the Grafana repository. Use the new package for integrating Grafana App SDK apps
with Kubernetes API servers.

The move was made in [grafana/grafana#134783](https://github.com/grafana/grafana/pull/134783)
so Grafana owns its API server integration and the SDK can drop its direct
`k8s.io/apiserver` dependency.

**This entire package, including `k8s/apiserver/cmd/server`, is deprecated and will
be removed in the next minor release of grafana-app-sdk.** This README will remain
after removal to direct users to the replacement.

For the installer, storage, admission, connector, strategy, and common OpenAPI
definitions, update imports to the new `appsdk` package before upgrading to the
next minor release. Refer to the
[new package's source and documentation](https://github.com/grafana/grafana/tree/main/pkg/apiserver/appsdk)
for its APIs and usage.

The standalone etcd server runner (`Options`, `Config`, and `cmd/server`) was
**not moved** to the new package. Code using these helpers must provide its own
server setup before upgrading; changing the import path alone will not replace
them.
