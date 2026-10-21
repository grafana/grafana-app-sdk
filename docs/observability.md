# Observability

The grafana-app-sdk provides built-in support for the three pillars of observability -- metrics, tracing, and logging -- as well as health checks for liveness and readiness probes. These features are designed to work together out of the box when using `simple.App` and the operator `Runner`, but each component can also be used independently.

## Metrics

The SDK uses [Prometheus](https://prometheus.io/) for metrics collection and exposition. The `metrics` package provides the core types for registering and exposing metrics, while individual SDK components (clients, informers, watchers) expose their own collectors via the `metrics.Provider` interface.

### `metrics.Provider`

Any SDK component that produces metrics implements the `Provider` interface:

```go
type Provider interface {
    PrometheusCollectors() []prometheus.Collector
}
```

Components that implement `Provider` include:
- `k8s.Client` and `k8s.SchemalessClient` (request latency, counts)
- `k8s.ClientRegistry` (aggregates collectors from all generated clients)
- `operator.InformerController` (aggregates collectors from informers, watchers, and reconcilers)
- `operator.CustomCacheInformer` (cache metrics)
- `operator.OpinionatedWatcher` (watcher metrics)
- `operator.MemcachedStore` (cache hit/miss metrics)
- `simple.App` (aggregates collectors from all managed components)
- `app.MultiRunner` and `app.SingletonRunner` (aggregate collectors from wrapped runnables)

### `metrics.Exporter`

The `Exporter` handles registering Prometheus collectors and serving the `/metrics` HTTP endpoint:

```go
exporter := metrics.NewExporter(metrics.ExporterConfig{
    Registerer: prometheus.DefaultRegisterer, // optional, defaults to prometheus.DefaultRegisterer
    Gatherer:   prometheus.DefaultGatherer,   // optional, defaults to prometheus.DefaultGatherer
})

// Register collectors from any Provider
err := exporter.RegisterCollectors(myClient.PrometheusCollectors()...)

// Get an HTTP handler for the /metrics endpoint
handler := exporter.HTTPHandler()
```

### `metrics.Config`

The `Config` struct provides general configuration for creating Prometheus collectors, such as namespace and native histogram settings:

```go
cfg := metrics.DefaultConfig("myapp")
// cfg.Namespace = "myapp"
// cfg.NativeHistogramBucketFactor = 1
// cfg.NativeHistogramMaxBucketNumber = 10
```

### Latency Buckets

The SDK defines a default set of histogram buckets for latency measurements in `metrics.LatencyBuckets`. If you need different bucket boundaries, set this variable before creating any components that produce latency histograms:

```go
metrics.LatencyBuckets = []float64{.001, .005, .01, .05, .1, .5, 1, 5, 10}
```

### `MetricsServer`

The `operator.MetricsServer` is an HTTP server that hosts the `/metrics` endpoint alongside health check endpoints (`/livez`, `/readyz`). It is created and managed automatically by the operator `Runner`, but can also be used standalone:

```go
server := operator.NewMetricsServer(operator.MetricsServerConfig{
    Port:                9090,           // default: 9090
    HealthCheckInterval: 1 * time.Minute, // default: 1 minute
})

server.RegisterMetricsHandler(exporter.HTTPHandler())
server.RegisterHealthChecks(myCheck)

// Optionally enable pprof profiling endpoints
server.RegisterProfilingHandlers()

// Run blocks until the context is cancelled
err := server.Run(ctx)
```

When profiling is enabled, the server exposes Go's standard `pprof` handlers at `/debug/pprof/`.

### `RunnerMetricsConfig`

When using the operator `Runner`, metrics are configured through `RunnerMetricsConfig`:

```go
runner, err := operator.NewRunner(operator.RunnerConfig{
    KubeConfig: kubeConfig,
    MetricsConfig: operator.RunnerMetricsConfig{
        Enabled:          true,
        ProfilingEnabled: false,        // set true to enable /debug/pprof/ endpoints
        Namespace:        "myapp",
        // Embedded MetricsServerConfig
        MetricsServerConfig: operator.MetricsServerConfig{
            Port:                9090,
            HealthCheckInterval: time.Minute,
        },
        // Embedded ExporterConfig (optional overrides)
        ExporterConfig: metrics.ExporterConfig{
            Registerer: prometheus.DefaultRegisterer,
            Gatherer:   prometheus.DefaultGatherer,
        },
    },
})
```

When `Enabled` is `true`, the runner automatically:
1. Creates a `metrics.Exporter`
2. Registers Prometheus collectors from the app and its components
3. Registers the `/metrics` HTTP handler on the `MetricsServer`

## Tracing

The SDK integrates with [OpenTelemetry](https://opentelemetry.io/) for distributed tracing. Trace context is propagated through Go's `context.Context` and is automatically correlated with log entries.

### Setting Up the Trace Provider

Use `simple.SetTraceProvider` to configure and register a global OpenTelemetry `TracerProvider`:

```go
err := simple.SetTraceProvider(simple.OpenTelemetryConfig{
    Host:        "localhost",
    Port:        4317,
    ConnType:    simple.OTelConnTypeGRPC, // or simple.OTelConnTypeHTTP
    ServiceName: "my-operator",
})
```

| Field         | Description                                              |
|---------------|----------------------------------------------------------|
| `Host`        | Hostname of the OpenTelemetry collector                  |
| `Port`        | Port of the collector endpoint                           |
| `ConnType`    | Transport protocol: `OTelConnTypeGRPC` or `OTelConnTypeHTTP` |
| `ServiceName` | The `service.name` resource attribute for this application |

This sets the global `otel.TracerProvider`, which is used by default across all SDK packages. Call this early in your application's startup, before creating any SDK components.

### Connection Types

- **`OTelConnTypeGRPC`**: Connects to the collector via gRPC. Uses insecure transport by default (configure TLS separately for production).
- **`OTelConnTypeHTTP`**: Connects to the collector via HTTP using the `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable (standard OTLP HTTP exporter).

### Auto-Instrumented Spans

The SDK provides tracing middleware for plugin router requests. When using `plugin/router`, you can add tracing with `NewTracingMiddleware`:

```go
import (
    "go.opentelemetry.io/otel"
    "github.com/grafana/grafana-app-sdk/plugin/router"
)

tracer := otel.Tracer("my-plugin")
r := router.NewRouter()
r.Use(router.NewTracingMiddleware(tracer))
```

This middleware creates a span for each incoming request and records HTTP semantic convention attributes (method, path, status code, route pattern, query string).

### Correlation with Logs

When tracing is configured, trace IDs are automatically injected into log entries. See the [Logging](#logging) section for details on how this works.

## Logging

The SDK uses structured logging via the `logging` package, which provides a thin abstraction over Go's standard `log/slog` package. The logging system supports context propagation and automatic trace ID injection.

### `logging.Logger` Interface

The `Logger` interface defines four log levels and two modifier methods:

```go
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
    With(args ...any) Logger         // returns a logger with attached key/value pairs
    WithContext(context.Context) Logger // returns a logger carrying the given context
}
```

Arguments follow the `slog` convention of alternating key/value pairs:

```go
logger.Info("reconciling resource", "name", obj.GetName(), "namespace", obj.GetNamespace())
```

### `logging.SLogLogger`

`SLogLogger` is the primary `Logger` implementation, wrapping Go's `*slog.Logger`. It automatically injects OpenTelemetry trace IDs into log entries when a valid trace context is present.

Create one with `NewSLogLogger`:

```go
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
logger := logging.NewSLogLogger(handler)
```

Or use `InitializerDefaultLogger` to set up both the SDK's default logger and `klog` (used by Kubernetes client-go) in one call:

```go
err := logging.InitializerDefaultLogger(os.Stdout, logging.Options{
    Format: logging.FormatJSON, // or logging.FormatText (default)
    Level:  slog.LevelInfo,     // default: slog.LevelInfo
})
```

This function:
1. Creates an `SLogLogger` with the specified format and level
2. Sets it as `logging.DefaultLogger`
3. Configures `klog` to use the same `*slog.Logger`, so Kubernetes client-go logs are unified with your application logs

### Context Propagation

The SDK passes loggers through `context.Context`, allowing any function in the call chain to retrieve a properly-configured logger:

```go
// Store a logger in the context
ctx = logging.Context(ctx, logger)

// Retrieve the logger from the context
logger := logging.FromContext(ctx)
logger.Info("processing event", "key", eventKey)
```

`FromContext` returns the logger from the context if one was set. If none was set, it falls back to `logging.DefaultLogger`. If that is also `nil`, it returns a `NoOpLogger` so that the return value is always safe to call without nil checks.

When `FromContext` returns a logger, it calls `WithContext(ctx)` on it, ensuring the logger carries the current context (which may include trace information).

### Trace ID Injection

The `SLogLogger` wraps its handler with a `traceIDHandler` that automatically extracts the OpenTelemetry trace ID from the context and adds it to every log record. The key name defaults to `"traceID"` and can be changed:

```go
logging.TraceIDKey = "trace_id" // change before creating loggers
```

When tracing is active, log output looks like:

```
level=INFO msg="reconciling resource" name=my-resource traceID=4bf92f3577b34da6a3ce929d0e0e4736
```

### Default Logger

`logging.DefaultLogger` is the fallback logger used when no logger is set in the context. It defaults to a `NoOpLogger`. Set it during initialization:

```go
logging.DefaultLogger = logging.NewSLogLogger(
    slog.NewJSONHandler(os.Stdout, nil),
)
```

Or use `InitializerDefaultLogger` as shown above.

## Health Checks

The SDK provides a health check framework for building Kubernetes-compatible liveness and readiness probes. Health checks are aggregated from all SDK components and exposed via HTTP endpoints.

### `health.Check` Interface

Any component that has a health check implements the `Check` interface:

```go
type Check interface {
    HealthCheck(context.Context) error
    HealthCheckName() string
}
```

Return `nil` from `HealthCheck` to indicate healthy status, or an error to indicate unhealthy.

### `health.Checker` Interface

Components that aggregate multiple health checks implement `Checker`:

```go
type Checker interface {
    HealthChecks() []Check
}
```

The `InformerController`, `MultiRunner`, `SingletonRunner`, and `simple.App` all implement `Checker`, collecting health checks from their child components (informers, watchers, reconcilers, runnables).

### `health.Observer`

The `Observer` periodically runs all registered health checks and maintains the current status:

```go
observer := health.NewObserver(30 * time.Second) // check interval
observer.AddChecks(myCheck1, myCheck2)

// Start the observer (blocks until context is cancelled)
go observer.Run(ctx)

// Query the current status
status := observer.Status()
if status.Successful {
    fmt.Println("All checks passed")
} else {
    for _, result := range status.Results {
        if result.Error != nil {
            fmt.Printf("FAIL: %s: %s\n", result.Name, result.Error)
        }
    }
}
```

The `Observer` runs an initial check immediately on startup, then continues checking at the configured interval.

### HTTP Endpoints

The `operator.MetricsServer` exposes two health endpoints:

| Endpoint  | Purpose                                      | Response                                         |
|-----------|----------------------------------------------|--------------------------------------------------|
| `/livez`  | Liveness probe -- is the process alive?      | Always returns `200 OK` with body `ok`           |
| `/readyz` | Readiness probe -- is the app ready to serve? | `200` if all checks pass, `500` if any check fails |

The `/readyz` endpoint returns the string representation of all check results. If any check fails, the response includes the failure details.

### Implementing a Custom Health Check

To add a health check for your own component, implement the `health.Check` interface:

```go
type MyReconciler struct {
    ready bool
}

func (r *MyReconciler) HealthCheck(_ context.Context) error {
    if !r.ready {
        return errors.New("reconciler not ready: initial sync incomplete")
    }
    return nil
}

func (r *MyReconciler) HealthCheckName() string {
    return "my-reconciler"
}
```

When added to a `simple.App` as a reconciler, the SDK automatically discovers and registers any component that implements `health.Check` or `health.Checker`. You can also register checks manually on the `MetricsServer`:

```go
server.RegisterHealthChecks(myReconciler)
```

### How the Runner Integrates Health Checks

The operator `Runner` automatically aggregates health checks from multiple sources when `Run` is called:

1. The `Runner` itself (reports whether the app has started)
2. The `app.MultiRunner` and its wrapped runnables
3. The `app.App` and its managed components (informer controller, informers, watchers, reconcilers)

These checks are all registered with the `MetricsServer`'s `Observer`, which runs them periodically and exposes the results on the `/readyz` endpoint.

## Putting It All Together

A typical operator setup configures all observability features during initialization:

```go
func main() {
    // 1. Initialize logging
    if err := logging.InitializerDefaultLogger(os.Stdout, logging.Options{
        Format: logging.FormatJSON,
        Level:  slog.LevelInfo,
    }); err != nil {
        log.Fatal(err)
    }

    // 2. Set up tracing
    err := simple.SetTraceProvider(simple.OpenTelemetryConfig{
        Host:        "otel-collector",
        Port:        4317,
        ConnType:    simple.OTelConnTypeGRPC,
        ServiceName: "my-operator",
    })
    if err != nil {
        log.Fatal(err)
    }

    // 3. Create the runner with metrics enabled
    runner, err := operator.NewRunner(operator.RunnerConfig{
        KubeConfig: kubeConfig,
        MetricsConfig: operator.RunnerMetricsConfig{
            Enabled: true,
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // 4. Run -- metrics, health checks, and tracing are handled automatically
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()
    runner.Run(ctx, provider)
}
```

For more details on building applications, see [Writing an App](./writing-an-app.md). For operator-specific patterns including reconcilers and watchers, see [Operators](./operators.md).
