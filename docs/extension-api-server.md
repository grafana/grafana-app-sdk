# Extension API Server

The Extension API Server pattern gives your application full control over how API requests are processed. Instead of relying solely on the platform's built-in storage and hooks, your backend acts as a Kubernetes aggregated API server, handling storage, authorization, and custom endpoints directly. This is the most flexible architecture the SDK supports, suitable for apps that need custom subresources, non-standard storage, or endpoints beyond basic CRUD.

For background on when to choose this pattern over simpler alternatives, see [Application Design Patterns](./application-design/README.md).

## When to Use It

Most applications should start with the [operator-based pattern](./operators.md), which covers validation, mutation, conversion, and reconciliation with less complexity. Choose the Extension API Server when you need:

- **Custom subresources** (e.g. `/rollback`, `/scale`) that go beyond standard CRUD
- **Custom storage backends** where the platform-provided storage does not fit your data model
- **Version-level routes** that are not tied to a specific kind (e.g. namespace-scoped or cluster-scoped utility endpoints)
- **Full control over request handling**, including authorization and response formatting

## Architecture Overview

When running as an Extension API Server, the Kubernetes API server proxies requests for your API group to your backend. Your app handles storage via `etcd` (the default) or any backend you configure, while the API server still handles authentication, rate-limiting, and discovery.

```
Client Request
    |
    v
Kubernetes API Server (authn, rate-limiting, discovery)
    |
    v  (proxy for your API group)
Extension API Server (your app)
    |-- Storage (etcd or custom)
    |-- Admission control (validation, mutation)
    |-- Conversion (multi-version support)
    |-- Custom routes & subresources
    |-- Reconciliation (async business logic)
```

## Setting Up an Extension API Server

### 1. Define Your App and Manifest

Like all SDK apps, you start with a manifest describing your kinds, versions, and routes. The manifest can be embedded in code or loaded from disk.

```go
provider := simple.NewAppProvider(apis.LocalManifest(), nil, NewApp)
config := app.Config{
    KubeConfig:   rest.Config{}, // replaced by the apiserver loopback config
    ManifestData: *apis.LocalManifest().ManifestData,
}
```

### 2. Create an AppInstaller

The `AppInstaller` is the bridge between your app and the Kubernetes API server machinery. It registers your kinds in the runtime scheme, installs API endpoints, and wires up admission control.

Use `apiserver.NewDefaultAppInstaller` with an `app.Provider`, an `app.Config`, and a `GoTypeResolver` that maps kind names and versions to Go types:

```go
installer, err := apiserver.NewDefaultAppInstaller(provider, config, &apis.GoTypeAssociator{})
```

The `GoTypeResolver` interface has four methods your code must implement:

| Method | Purpose |
|--------|---------|
| `KindToGoType(kind, version)` | Maps a kind name and version to a `resource.Kind` |
| `CustomRouteReturnGoType(kind, version, path, verb)` | Maps a custom route to its response Go type |
| `CustomRouteQueryGoType(kind, version, path, verb)` | Maps a custom route to its query parameter Go type |
| `CustomRouteRequestBodyGoType(kind, version, path, verb)` | Maps a custom route to its request body Go type |

The installer automatically handles scheme registration, OpenAPI definition generation, storage setup, and admission plugin creation based on your manifest.

### 3. Configure Server Options

Create `Options` with your installer(s) and configure the underlying Kubernetes API server options:

```go
opts := apiserver.NewOptions([]apiserver.AppInstaller{installer})
```

`Options` wraps `genericoptions.RecommendedOptions`, giving you control over:

| Option | Description |
|--------|-------------|
| `RecommendedOptions.Etcd` | etcd connection and storage configuration |
| `RecommendedOptions.SecureServing` | TLS, bind address, and port |
| `RecommendedOptions.Authentication` | Authentication configuration (set to `nil` to disable) |
| `RecommendedOptions.Authorization` | Authorization configuration (set to `nil` to disable) |
| `RecommendedOptions.Admission` | Admission plugin configuration |
| `RecommendedOptions.CoreAPI` | Core API server connection (set to `nil` for standalone mode) |
| `RecommendedOptions.Features` | Feature gates (e.g. priority and fairness) |

For local development, you can disable authentication, authorization, and core API access:

```go
opts.RecommendedOptions.Authentication = nil
opts.RecommendedOptions.Authorization = nil
opts.RecommendedOptions.CoreAPI = nil
```

### 4. Start the Server

Use the `server.NewCommandStartServer` helper to create a Cobra command that validates options, builds the config, and runs the server:

```go
ctx := genericapiserver.SetupSignalContext()
cmd := server.NewCommandStartServer(ctx, opts)
code := cli.Run(cmd)
os.Exit(code)
```

Under the hood, `Options.Config()` calls `Config.NewServer()`, which:

1. Initializes each `AppInstaller` with the loopback client config
2. Calls `InstallAPIs` to register all kinds, subresources, and custom routes
3. Adds a post-start hook that runs each app's `Runner` (for reconciliation and other async work)

## Custom Routes and Subresources

The SDK supports two types of custom routes:

### Kind Subresource Routes

These are routes attached to a specific kind, accessible at `/apis/<group>/<version>/namespaces/<ns>/<plural>/<name>/<route>`. Define them in your `simple.AppConfig`:

```go
simple.AppConfig{
    ManagedKinds: []simple.AppManagedKind{{
        Kind: v1alpha1.TestKindKind(),
        CustomRoutes: map[simple.AppCustomRoute]simple.AppCustomRouteHandler{
            {
                Method: simple.AppCustomRouteMethodGet,
                Path:   "foo",
            }: func(ctx context.Context, w app.CustomRouteResponseWriter, r *app.CustomRouteRequest) error {
                w.WriteHeader(http.StatusOK)
                return json.NewEncoder(w).Encode(myResponse)
            },
        },
    }},
}
```

### Version-Level Routes

These are routes not tied to a specific kind, accessible at the version level. They can be namespaced or cluster-scoped:

```go
simple.AppConfig{
    VersionedCustomRoutes: map[string]simple.AppVersionRouteHandlers{
        "v1alpha1": {
            {
                Namespaced: true,
                Path:       "foobar",
                Method:     "GET",
            }: func(ctx context.Context, w app.CustomRouteResponseWriter, r *app.CustomRouteRequest) error {
                return json.NewEncoder(w).Encode(myResponse)
            },
        },
    },
}
```

Namespaced routes are served at `/apis/<group>/<version>/namespaces/<ns>/<path>`, and cluster-scoped routes at `/apis/<group>/<version>/<path>`.

Route handlers receive a `context.Context`, an `app.CustomRouteResponseWriter` (which implements `http.ResponseWriter`), and an `app.CustomRouteRequest` containing the resource identifier, URL, method, headers, and body.

## Conversion

When your app has multiple versions of a kind, you need to handle conversion between them. Implement the `simple.Converter` interface:

```go
type Converter interface {
    Convert(obj k8s.RawKind, targetAPIVersion string) ([]byte, error)
}
```

Register converters in your `simple.AppConfig`:

```go
simple.AppConfig{
    Converters: map[schema.GroupKind]simple.Converter{
        {
            Group: config.ManifestData.Group,
            Kind:  v1alpha1.TestKindKind().Kind(),
        }: &MyConverter{},
    },
}
```

The Extension API Server handles conversion automatically through the Kubernetes scheme's conversion functions, calling your converter when objects need to be translated between versions. For more details, see [Admission Control](./admission-control.md).

## Admission Control

Validation and mutation are declared in your manifest's `AdmissionCapabilities` and implemented in your app. When running as an Extension API Server, the installer automatically creates an admission plugin from your manifest:

```go
simple.AppConfig{
    ManagedKinds: []simple.AppManagedKind{{
        Kind: v1alpha1.TestKindKind(),
        Validator: &simple.Validator{
            ValidateFunc: func(ctx context.Context, request *app.AdmissionRequest) error {
                if request.Object.GetName() == "notallowed" {
                    return errors.New("not allowed")
                }
                return nil
            },
        },
    }},
}
```

Unlike the operator pattern where webhooks are called externally by the API server, the Extension API Server runs admission control in-process as a Kubernetes admission plugin. This means there is no need to configure webhook URLs or TLS certificates for admission.

For more on admission control patterns, see [Admission Control](./admission-control.md).

## Example

A complete working example is available at [`examples/apiserver/`](../examples/apiserver/). It demonstrates:

- Multi-version kind registration (`v0alpha1`, `v1alpha1`, `v2alpha1`)
- Custom subresource routes (`/foo`, `/bar`, `/recurse`)
- Version-level routes (namespaced and cluster-scoped `/foobar`)
- Version conversion between `v0alpha1` and `v1alpha1`
- Validation via a simple validator
- Reconciliation that calls custom subresources
- Server configuration with disabled auth for local development
