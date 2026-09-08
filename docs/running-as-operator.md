# Running as an Operator

This document covers the lifecycle and deployment of an app built with the SDK when running as a standalone operator.
An operator runner manages webhook servers, metrics endpoints, health checks, and the main control loop for your app.
For an introduction to operator concepts, see [Operators and Event-Based Design](./operators.md).
For details on building your app, see [Writing an App](./writing-an-app.md).

## Runner Lifecycle

The `operator.Runner` is the primary entry point for running an `app.App` as a standalone operator.
It coordinates multiple subsystems -- webhooks, metrics, health checks, and the app's main loop --
and manages their lifecycle from startup through graceful shutdown.

### Startup Sequence

When you call `runner.Run(ctx, provider)`, the following happens in order:

1. **Manifest loading** -- The runner reads the app manifest (embedded, from disk, or from an API server) to determine which kinds and capabilities the app has.
2. **App instantiation** -- The provider's `NewApp` method is called with the loaded manifest data and kubernetes config.
3. **Webhook registration** -- If any kinds require validation, mutation, or conversion, the runner registers handlers on the webhook server (TLS config must be provided).
4. **Metrics setup** -- If metrics are enabled, the runner registers prometheus collectors from both the app and internal runners.
5. **Health check registration** -- The runner registers health checks from itself, the app, and all internal runners on the metrics server.
6. **Run** -- All subsystems (webhook server, app runner, metrics server) are started concurrently via `app.MultiRunner`.

```golang
runner, err := operator.NewRunner(operator.RunnerConfig{
    KubeConfig: *kubeConfig,
    MetricsConfig: operator.RunnerMetricsConfig{
        Enabled: true,
    },
})
if err != nil {
    log.Fatal(err)
}

ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
defer cancel()

err = runner.Run(ctx, provider)
```

### Graceful Shutdown

When the context passed to `Run` is canceled (for example, via `SIGINT` or `SIGTERM`), the `MultiRunner` propagates the cancellation to all subsystems.
Each component -- the webhook server, the app's control loop, and the metrics server -- receives the canceled context and shuts down.
The runner waits for all components to finish before returning.

If you need a timeout on shutdown, you can configure `ExitWait` on the underlying `MultiRunner` (accessible when building a custom runner setup).

## RunnerConfig

`operator.RunnerConfig` controls how the runner behaves. Here are the main fields:

### KubeConfig

The `KubeConfig` field is a standard `rest.Config` from `k8s.io/client-go`. It tells the runner (and the app) how to communicate with the Kubernetes API server.

```golang
import "k8s.io/client-go/tools/clientcmd"

// From a kubeconfig file
kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)

// In-cluster (when deployed as a pod)
import "k8s.io/client-go/rest"
kubeConfig, err := rest.InClusterConfig()
```

### WebhookConfig

`RunnerWebhookConfig` configures the HTTPS server for admission webhooks. This is only required if your app defines validation, mutation, or conversion capabilities for any of its kinds.

```golang
runner, err := operator.NewRunner(operator.RunnerConfig{
    KubeConfig: *kubeConfig,
    WebhookConfig: operator.RunnerWebhookConfig{
        Port: 9443,
        TLSConfig: k8s.TLSConfig{
            CertPath: "/etc/webhook/tls.crt",
            KeyPath:  "/etc/webhook/tls.key",
        },
    },
})
```

If any kind in the manifest requires webhooks but no TLS config is provided, `Run` will return an error. Certificate management is typically handled by [cert-manager](https://cert-manager.io/) or a similar tool in your cluster.

### MetricsConfig

`RunnerMetricsConfig` controls the metrics server, which hosts prometheus metrics, health check endpoints, and optional profiling.

```golang
runner, err := operator.NewRunner(operator.RunnerConfig{
    KubeConfig: *kubeConfig,
    MetricsConfig: operator.RunnerMetricsConfig{
        Enabled:          true,
        Port:             9090,   // Default
        ProfilingEnabled: false,  // Set true to expose /debug/pprof
        HealthCheckInterval: time.Minute, // Default
    },
})
```

The metrics server exposes:
- `/metrics` -- Prometheus metrics (when `Enabled` is true)
- `/livez` -- Liveness probe (always returns 200)
- `/readyz` -- Readiness probe (returns 200 when all health checks pass, 500 otherwise)
- `/debug/pprof/*` -- Go profiling endpoints (when `ProfilingEnabled` is true)

### HealthCheck

You can pass a custom `health.Check` implementation in the config. The runner itself also implements `health.Check` and reports unhealthy until `Run` is actively executing.

### Filesystem

The `Filesystem` field accepts an `fs.FS` implementation, used when loading a manifest from a file path. By default, it uses `os.DirFS(".")`.

## App Manifest

The `app.Manifest` tells the runner what kinds and capabilities your app supports. It determines which webhooks to register and how the app is configured.

### Manifest Locations

A manifest can be loaded from three sources:

```golang
// Embedded directly in code (most common for generated apps)
manifest := app.NewEmbeddedManifest(manifestData)

// From a file on disk
manifest := app.NewOnDiskManifest("path/to/manifest.json")

// From an API server resource (not yet supported)
manifest := app.NewAPIServerManifest("resource-name")
```

### ManifestData Structure

`ManifestData` describes the app's group, versions, kinds, and capabilities:

```golang
data := app.ManifestData{
    AppName: "myapp",
    Group:   "myapp.ext.grafana.com",
    Versions: []app.ManifestVersion{{
        Name:   "v1",
        Served: true,
        Kinds: []app.ManifestVersionKind{{
            Kind:   "MyResource",
            Plural: "myresources",
            Scope:  "Namespaced",
            Admission: &app.AdmissionCapabilities{
                Validation: &app.ValidationCapability{
                    Operations: []app.AdmissionOperation{app.AdmissionOperationCreate, app.AdmissionOperationUpdate},
                },
            },
        }},
    }},
}
```

When using generated code from CUE kind definitions, the manifest is typically produced by the code generator and embedded in a `LocalManifest()` function.

### Manifest Validation

`ManifestData.Validate()` checks for consistency across versions (plural names, scopes, and conversion flags must match for the same kind across different versions). The `simple.App` also provides `ValidateManifest` to verify that managed kinds fully cover the manifest's declared capabilities.

## CRD Registration

The `k8s.ResourceManager` handles creating and updating Custom Resource Definitions in the cluster.

```golang
manager, err := k8s.NewManager(kubeConfig)
if err != nil {
    log.Fatal(err)
}

// Register a schema as a CRD
err = manager.RegisterSchema(ctx, mySchema, resource.RegisterSchemaOptions{
    UpdateOnConflict:    true,  // Update if CRD already exists
    WaitForAvailability: true,  // Block until the CRD is available
})
```

The `operator.Runner` itself does not automatically register CRDs. It expects the kinds to already exist in the API server. CRD registration is typically done as a separate step in your deployment pipeline, either:
- As an init container or startup task before the operator runs
- Via a Helm chart or kustomize that applies CRD manifests
- Manually with `kubectl apply`

If you need programmatic registration, call `k8s.ResourceManager.RegisterSchema` before starting the runner.

## Running Multiple Apps

### MultiRunner

The `app.MultiRunner` runs multiple `Runnable` instances concurrently. The `operator.Runner` uses it internally, but you can also use it to compose your own multi-component setup:

```golang
multi := app.NewMultiRunner()
multi.AddRunnable(componentA)
multi.AddRunnable(componentB)

// Configure error handling
multi.ErrorHandler = func(ctx context.Context, err error) bool {
    log.Printf("component error: %v", err)
    return true // return true to stop all components on error
}

// Configure shutdown timeout
timeout := 30 * time.Second
multi.ExitWait = &timeout

err := multi.Run(ctx)
```

When any runner returns an error, the `ErrorHandler` is called. If it returns `true`, all other runners are canceled. If `ExitWait` is set, the `MultiRunner` will return `ErrRunnerExitTimeout` if the remaining runners do not stop within the timeout.

### DynamicMultiRunner

For cases where you need to add or remove components at runtime, use `app.DynamicMultiRunner`:

```golang
dynamic := app.NewDynamicMultiRunner()

// Add a runner before or after calling Run
dynamic.AddRunnable(myRunner)

go dynamic.Run(ctx)

// Later, add another runner dynamically
dynamic.AddRunnable(anotherRunner)

// Or remove one
dynamic.RemoveRunnable(myRunner)
```

### SingletonRunner

`app.SingletonRunner` wraps a single `Runnable` so that multiple calls to `Run` share the same underlying execution. This is used internally by the runner for the webhook server and metrics server, ensuring they start only once even if `Run` is called multiple times.

```golang
singleton := app.NewSingletonRunner(myServer, false)
// Multiple goroutines can call Run; only one instance of myServer will execute
go singleton.Run(ctx1)
go singleton.Run(ctx2)
```

When `StopOnAny` is true, stopping any caller also stops the underlying `Runnable`.

## Production Considerations

### Health Checks

The metrics server provides `/livez` and `/readyz` endpoints suitable for Kubernetes probes. Health checks are collected from the runner, the app, and all internal components (including informer controllers). The readiness endpoint reports unhealthy if any registered check fails.

Configure your pod's probes to point at these endpoints:

```yaml
livenessProbe:
  httpGet:
    path: /livez
    port: 9090
readinessProbe:
  httpGet:
    path: /readyz
    port: 9090
```

The `HealthCheckInterval` in `RunnerMetricsConfig` controls how often checks are evaluated (default: 1 minute). The first evaluation happens immediately on startup.

### Resource Limits and Tuning

For production deployments, consider these tuning options:

- **Informer supplier**: Use `simple.OptimizedInformerSupplier` for better memory efficiency when watching many resources. It supports watch-list streaming (Kubernetes 1.27+) and paginated list.
- **Watch-list page size**: Set `InformerOptions.WatchListPageSize` to control memory usage during initial list operations (recommended: 5000-10000 for large resource sets).
- **Retry policies**: Configure `InformerConfig.RetryPolicy` and `RetryDequeuePolicy` to control how failed reconciliations are retried.
- **Label and field selectors**: Use `ListWatchOptions.LabelFilters` and `FieldSelectors` to reduce the set of resources your operator watches.

### Error Handling

The `MultiRunner` error handler determines behavior when a component fails. The default handler logs the error and cancels all other components. For operators that should tolerate partial failures, provide a custom handler:

```golang
multi := app.NewMultiRunner()
multi.ErrorHandler = func(ctx context.Context, err error) bool {
    logging.FromContext(ctx).Error("component failed", "error", err)
    // Return false to keep other components running
    return false
}
```

### Observability

The runner integrates with OpenTelemetry for tracing and Prometheus for metrics. Set up tracing before calling `Run`:

```golang
simple.SetTraceProvider(simple.OpenTelemetryConfig{
    Host:        "localhost",
    Port:        4317,
    ConnType:    simple.OTelConnTypeGRPC,
    ServiceName: "myapp-operator",
})
```

Enable profiling in non-production environments for diagnosing performance issues:

```golang
MetricsConfig: operator.RunnerMetricsConfig{
    Enabled:          true,
    ProfilingEnabled: true, // Exposes /debug/pprof endpoints
},
```

### Admission Control

If your app implements validation, mutation, or conversion, the runner will automatically set up the webhook server. Ensure that:

1. TLS certificates are available at the configured paths
2. Kubernetes `ValidatingWebhookConfiguration` and `MutatingWebhookConfiguration` resources are deployed in the cluster pointing to your operator's service
3. For conversion webhooks, the CRD's `conversion.strategy` is set to `Webhook` with the appropriate client config

For more details, see [Admission Control](./admission-control.md).
