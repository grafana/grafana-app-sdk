# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The `grafana-app-sdk` is an SDK for developing apps for the Grafana App Platform. It provides:
- A CLI (`grafana-app-sdk`) for generating code and projects
- Libraries for building Kubernetes operator-based applications
- Integration with Kubernetes API machinery and client-go

The SDK follows Kubernetes patterns and conventions, treating custom resources as first-class objects with support for validation, mutation, conversion, and reconciliation.

## Build, Test, and Development Commands

### Standard Development Workflow

```bash
# Install all dependencies and run full verification
make                       # Equivalent to: deps, lint, test, build

# Run tests with coverage
make test                  # Run all tests across submodules
make coverage              # Generate HTML coverage report

# Build the CLI binary
make build                 # Output: target/grafana-app-sdk

# Install CLI to GOPATH/bin
make install

# Run linter
make lint                  # Uses golangci-lint v2.5.0

# Update go.mod and go.work files
make update-workspace
```

### Running Individual Tests

```bash
# Run a specific test
go test -v -run TestName ./package/path

# Run tests in a specific package
go test ./k8s/...
go test ./operator/...

# Run a single test with verbose output
go test -v -count=1 -run TestInformerCustomCache ./operator/
```

### Performance Benchmarking

The project has comprehensive benchmarks for performance-critical components (informers, caches, operators):

```bash
# Run all benchmarks
make bench

# Establish baseline before making optimizations
make bench-baseline

# After code changes, compare against baseline
make bench-compare         # Uses benchstat for statistical analysis

# Generate memory and CPU profiles
make bench-profile
go tool pprof target/profiles/mem.out   # Analyze memory profile
go tool pprof target/profiles/cpu.out   # Analyze CPU profile
```

**Optimization workflow**: Profile first to identify bottlenecks → Establish baseline → Make changes → Compare to validate improvement.

### Code Generation

```bash
# Regenerate code from CUE definitions (requires CLI)
make generate

# Regenerate test golden files (codegen tests)
make regenerate-codegen-test-files
```

## Architecture and Key Packages

### Core Package Structure

The SDK is organized into several key packages that mirror Kubernetes patterns:

- **`resource/`** - Core resource abstractions (Kind, Object, Schema, Client, Store)
  - Defines the fundamental types for working with custom resources
  - Provides typed and untyped object wrappers
  - Implements storage interfaces compatible with Kubernetes patterns

- **`k8s/`** - Kubernetes client implementations
  - `Client` - Main resource client for CRUD operations
  - `ClientRegistry` - Manages multiple clients for different resource types
  - Cache implementations in `k8s/cache/` (controller, reflector with watch-list support)
  - Webhook handlers (admission, validation, mutation, conversion)

- **`operator/`** - Operator/controller framework components
  - `Informer` implementations (CustomCache, Kubernetes native, concurrent)
  - Reconcilers and watchers (Simple, Opinionated, Concurrent)
  - Controller loops for reacting to resource changes
  - Caching strategies (in-memory, memcached)

- **`simple/`** - High-level simplified APIs
  - `App` - Main application builder with opinionated defaults
  - `Operator` - Simplified operator creation
  - Integrates health checks, metrics, and tracing out of the box

- **`app/`** - Application manifest and runner
  - App manifest parsing and validation (CUE-based)
  - Application lifecycle management

- **`codegen/`** - Code generation from CUE schemas
  - CLI implementation for `grafana-app-sdk generate`

### Design Patterns

The SDK supports three application architectures:

1. **Frontend-only**: Custom Kinds with basic validation, no backend
2. **Operator-based**: Backend with validation, mutation, conversion, and reconciliation hooks
3. **Custom API**: Full control with extension API server for custom endpoints

All patterns assume resources can be modified outside your UI (kubectl, gitops tools like Flux, backup tools like Velero).

### Kubernetes Integration

- Built on `k8s.io/client-go` and `k8s.io/apimachinery`
- Uses `k8s.io/apiserver` for extension API servers
- Implements controller-runtime-like informers and reconcilers
- Supports watch-list streaming via `WatchListClient` feature gate
- Compatible with CRDs (Custom Resource Definitions)

### Testing Patterns

- Tests follow table-driven test patterns (see `go.dev/wiki/TableDrivenTests`)
- Uses `testify` for assertions
- Mocks are typically implemented as interfaces in test files
- Performance benchmarks in `benchmark/` package use realistic object counts (10k-50k)

## Go Workspace Configuration

This repository uses Go workspaces (`go.work`) for multi-module development. The workspace includes:
- Main SDK module
- `logging/` submodule

When modifying dependencies, always run `make update-workspace` to sync workspace files.

## Version and Dependencies

- Go version: 1.24.0 (see go.mod)
- Primary Kubernetes version: v0.34.1
- Uses CUE for schema definitions (cuelang.org/go)
- Prometheus metrics integration
- OpenTelemetry tracing support

## Common Gotcaps and Conventions

- **Hooks timing**: Validation, mutation, and conversion hooks are synchronous and must complete quickly (called by API server before storage)
- **Reconciliation**: Should be asynchronous and idempotent
- **Resource lifecycle**: Objects have metadata (name, namespace, UID, resourceVersion) following Kubernetes conventions
- **Naming**: Uses Kubernetes GVK (Group/Version/Kind) and GVR (Group/Version/Resource) terminology
- **Watch semantics**: Informers can use either traditional paginated LIST or efficient watch-list streaming
- **Caching**: Multiple strategies available (in-memory indexing, memcached for distributed scenarios)

## Codebase Architecture Insights

This section provides a comprehensive understanding of the grafana-app-sdk codebase architecture, implementation patterns, and development practices. It is designed to help developers quickly understand and navigate this complex system.

### Project Scale & Composition

- **Total Go files**: ~7,729 (51 test files)
- **Primary language**: Go 1.24.0
- **Kubernetes version**: v0.34.1
- **Module structure**: Multi-module workspace (main SDK + logging submodule)
- **Lines of code**: Large-scale production codebase
- **Status**: Experimental (minor versions may introduce breaking changes)

### Architectural Philosophy

The SDK follows these core principles:

1. **Resource-Centric Design**: Everything revolves around `resource.Object` - the fundamental abstraction for Kubernetes-like resources combining runtime.Object, schema.ObjectKind, and metav1.Object with SDK-specific methods.

2. **Event-Driven Architecture**: Asynchronous reconciliation pattern decouples API requests from business logic. When a user creates/updates a resource, the API validates and stores it immediately, then asynchronous reconcilers react to the change event.

3. **Storage-Agnostic Abstractions**: While currently implemented with Kubernetes CRDs, the SDK's abstractions (Client, Store, Schema) work with any storage backend that supports basic CRUD + Watch operations.

4. **Code Generation First**: CUE schemas are the source of truth, generating type-safe Go code, CRDs, OpenAPI specs, and client implementations automatically.

5. **Performance-Conscious**: Active optimization efforts focusing on memory efficiency and allocation reduction, especially in hot paths (informers, codecs, object copying).

### Layered Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Application Layer                            │
│  ┌────────────┐  ┌────────────┐  ┌──────────────────────────┐  │
│  │simple.App  │  │app.Runner  │  │CLI (code generation)    │  │
│  │(high-level)│  │(lifecycle) │  │(grafana-app-sdk cmd)    │  │
│  └────────────┘  └────────────┘  └──────────────────────────┘  │
├─────────────────────────────────────────────────────────────────┤
│                     Operator Layer                               │
│  ┌──────────────────┐  ┌──────────────┐  ┌─────────────────┐  │
│  │Informers         │  │Reconcilers   │  │Watchers         │  │
│  │(cache resources) │  │(state sync)  │  │(event handlers) │  │
│  └──────────────────┘  └──────────────┘  └─────────────────┘  │
├─────────────────────────────────────────────────────────────────┤
│                     Resource Layer                               │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────┐   │
│  │Object/Kind   │  │Client/Store  │  │Schema/Codec        │   │
│  │(abstractions)│  │(CRUD ops)    │  │(type metadata)     │   │
│  └──────────────┘  └──────────────┘  └────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                Kubernetes Integration Layer                      │
│  ┌────────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │k8s.Client      │  │k8s.Cache     │  │Webhooks          │  │
│  │(API wrapper)   │  │(controller,  │  │(admission        │  │
│  │                │  │ reflector)   │  │ control)         │  │
│  └────────────────┘  └──────────────┘  └──────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Critical Package Breakdown

#### `/resource/` - Core Abstractions

**Purpose**: Defines fundamental types for working with custom resources

**Key Files**:
- `object.go` - `Object` interface (combines k8s interfaces + SDK methods)
- `kind.go` - `Kind` struct (Schema + Codecs for serialization)
- `schema.go` - `Schema` interface (Group/Version/Kind metadata)
- `client.go` - Generic `Client` interface for CRUD operations
- `store.go` - Key-value `Store` abstraction
- `admission.go` - Admission control interfaces (Validator, Mutator, Converter)

**Key Pattern**: `TypedStore[T]` - Type-safe wrapper around generic Store:
```go
store := resource.NewTypedStore[*MyKind](untypedStore)
obj, err := store.Get(ctx, identifier)  // Returns *MyKind directly
```

**Critical for**: Understanding how resources are represented and manipulated throughout the system.

#### `/operator/` - Operator Framework

**Purpose**: Implements controller/operator patterns for reacting to resource changes

**Key Files**:
- `operator.go` - Top-level `Operator` managing multiple controllers
- `informer_customcache.go` - Optimized informer with pluggable cache (PRIMARY OPTIMIZATION TARGET)
- `informer_kubernetes.go` - Standard client-go SharedInformer wrapper
- `informer_concurrent.go` - Parallel event processing for high throughput
- `reconciler.go` - `Reconciler` interface and implementations
- `simplewatcher.go` - `ResourceWatcher` interface (Add/Update/Delete hooks)
- `opinionatedwatcher.go` - Finalizer-aware watcher for event reliability
- `runner.go` - Operator lifecycle management

**Key Pattern**: Informer + Reconciler/Watcher pipeline:
```
k8s Watch Event → Reflector → DeltaFIFO → Controller → 
    CustomCacheInformer.processDeltas() → processor.distribute() →
        [Reconciler.Reconcile() | Watcher.Add/Update/Delete()]
```

**Current Focus**: Memory and CPU optimization in CustomCacheInformer hot path (see performance docs).

**Critical for**: Building operators that react to resource changes.

#### `/k8s/` - Kubernetes Integration

**Purpose**: Bridges SDK abstractions to actual Kubernetes API machinery

**Key Files**:
- `client.go` - `Client` implementation using client-go dynamic client
- `cache/reflector.go` - Watch/list implementation (supports watch-list feature)
- `cache/controller.go` - Cache controller managing reflector + DeltaFIFO
- `webhooks.go` - Admission webhook handlers
- `conversion.go` - Multi-version conversion webhook
- `apiserver/` - Extension API server support for custom endpoints

**Key Pattern**: Watch-list protocol for memory efficiency:
- Traditional: LIST all objects (high memory), then WATCH for changes
- Watch-list: Streaming LIST that seamlessly transitions to WATCH
- Requires Kubernetes 1.27+ with feature gate enabled

**Critical for**: Understanding how the SDK interacts with Kubernetes APIs.

#### `/simple/` - High-Level APIs

**Purpose**: Provides opinionated, easy-to-use application builder

**Key Files**:
- `app.go` - `App` struct with fluent configuration API
- `operator.go` - Simplified operator creation

**Key Pattern**: Declarative app configuration:
```go
app, err := simple.NewApp(simple.AppConfig{
    Name: "myapp",
    KubeConfig: kubeConfig,
    ManagedKinds: []simple.AppManagedKind{{
        Kind: myKind.Kind(),
        Reconciler: &myReconciler,
        Validator: &myValidator,
        Mutator: &myMutator,
    }},
})
```

**Critical for**: Building complete applications quickly without deep SDK knowledge.

#### `/codegen/` - Code Generation

**Purpose**: Generates type-safe Go code from CUE schemas

**Key Files**:
- `generator.go` - Generation orchestration
- `cuekind/` - CUE parsing logic
- `jennies/` - Individual code generators (Object, Schema, CRD, Client, etc.)
- `templates/` - Go code templates

**Key Pattern**: Jenny pipeline:
```
CUE Schema → Parser → Generator(JennyList) → Generated Files
```

**Generated Artifacts**:
- Go types implementing `resource.Object`
- CRD YAML manifests
- OpenAPI v3 schemas
- Typed clients and codecs
- Schema metadata

**Critical for**: Understanding how generated code relates to CUE definitions.

#### `/app/` - Application Framework

**Purpose**: Application manifest and lifecycle management

**Key Files**:
- `manifest.go` - CUE-based app manifest parsing
- `runner.go` - Multi-component application runner
- `appmanifest/v1alpha1/`, `appmanifest/v1alpha2/` - Versioned manifest types

**Key Pattern**: Provider pattern for app initialization:
```go
provider := simple.NewAppProvider(manifest, specificConfig, newAppFunc)
runner, err := app.NewRunner(provider, config)
runner.Run(ctx)
```

**Critical for**: Understanding application lifecycle and deployment.

### Application Design Patterns

The SDK supports three architectural patterns:

#### 1. Frontend-Only Applications

**Use case**: Simple apps with basic validation, no backend logic

**Components**:
- CUE kind definitions
- Frontend plugin code
- Grafana platform handles storage via CRDs

**Capabilities**:
- CRUD via Grafana UI
- kubectl/gitops support automatically
- Basic CUE schema validation

**Trade-off**: No custom business logic on resource changes

#### 2. Operator-Based Applications

**Use case**: Apps needing validation, mutation, conversion, or reconciliation

**Components**:
- CUE kind definitions
- Operator backend (validation/mutation/conversion webhooks + reconcilers)
- Optional frontend plugin

**Capabilities**:
- Custom validation beyond schema
- Default value injection via mutation
- Multi-version support via conversion
- Asynchronous business logic via reconciliation
- Status tracking via subresources

**Trade-off**: More complex, requires operator deployment

**Key Insight**: Webhooks are SYNCHRONOUS (block API requests), reconcilers are ASYNCHRONOUS (react to events).

#### 3. Custom API Applications

**Use case**: Apps needing custom endpoints or storage strategies

**Components**:
- Extension API server (implements k8s aggregation API)
- Custom routing and storage logic
- Optional frontend plugin

**Capabilities**:
- Custom subresources (e.g., `/rollback`, `/scale`)
- Custom storage backends
- Full control over authorization
- Non-standard API patterns (within RESTful constraints)

**Trade-off**: Highest complexity, most flexibility

### Event-Driven Execution Flow

Understanding the event flow is critical for debugging and optimization.

#### Initial Sync (Startup)

```
1. App.Run(ctx) called
   ↓
2. InformerController.Run() starts
   ↓
3. For each Informer:
   ├─ Start Reflector goroutine
   ├─ Start Controller goroutine  
   └─ Start Processor goroutines
   ↓
4. Reflector.ListAndWatch()
   ├─ LIST: Fetch all existing resources
   ├─ Store in DeltaFIFO as Sync events
   └─ WATCH: Open long-lived connection
   ↓
5. Controller.processLoop()
   ├─ Pop deltas from DeltaFIFO
   ├─ Update cache.Store
   └─ Send to Informer
   ↓
6. Informer.processDeltas()
   ├─ Convert to event structs
   └─ Call processor.distribute()
   ↓
7. processor.distribute()
   ├─ Lock listener list (RWMutex)
   ├─ Send to each listener's channel
   └─ Listener goroutines call Reconciler/Watcher
   ↓
8. HasSynced() returns true when initial LIST processed
```

#### Runtime Event Handling (Hot Path)

```
1. Resource changes in k8s (via kubectl, UI, API)
   ↓
2. k8s API server sends WATCH event
   ↓
3. Reflector receives event
   ├─ Add to DeltaFIFO (Added/Updated/Deleted delta)
   └─ Update lastSyncResourceVersion
   ↓
4. Controller.processLoop() processes delta
   ├─ Update cache.Store (Get/Update/Add/Delete)
   └─ Call handler (Informer.OnAdd/OnUpdate/OnDelete)
   ↓
5. CustomCacheInformer processes event
   ├─ Create event struct (informerEventAdd/Update/Delete)
   ├─ Convert via toResourceObject()
   └─ Call processor.distribute()
   ↓
6. processor.distribute() broadcasts
   ├─ RWMutex.RLock() (CONTENTION POINT)
   ├─ For each listener: send to bufferedQueue channel
   └─ RWMutex.RUnlock()
   ↓
7. Listener goroutines (one per Reconciler/Watcher)
   ├─ Receive from channel
   ├─ Create context with timeout
   └─ Call Reconcile() or Add/Update/Delete()
   ↓
8. Reconciler/Watcher executes user code
   ├─ Returns ReconcileResult or error
   └─ May enqueue retry via RetryPolicy
```

**Performance Bottlenecks** (identified in profiling):
- Line 6: `processor.distribute()` - RWMutex on every event
- Line 5: Object copying in `toResourceObject()` - reflection allocations
- Line 5: `JSONCodec.Write()` in metrics/logging - map allocations
- Line 8: Context creation per event - allocation overhead

**See detailed architecture**: `/docs/architecture/reconciliation.md` (contains complete call graphs with file:line references)

### Configuration Patterns

#### Kubeconfig Loading

```go
import "k8s.io/client-go/tools/clientcmd"

// From file
config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)

// In-cluster (when running as pod)
config, err := rest.InClusterConfig()
```

#### App Configuration

```go
appConfig := simple.AppConfig{
    Name: "myapp",
    KubeConfig: *kubeConfig,
    ManagedKinds: []simple.AppManagedKind{
        {
            Kind: myKind.Kind(),
            Reconciler: &myReconciler,
            // Optional:
            Validator: &myValidator,
            Mutator: &myMutator,
        },
    },
    // Advanced:
    InformerConfig: simple.AppInformerConfig{
        InformerSupplier: simple.OptimizedInformerSupplier,
        UseWatchList: true,
        WatchListPageSize: 500,
    },
    Converters: map[schema.GroupKind]simple.Converter{
        {Group: "mygroup", Kind: "MyKind"}: myConverter,
    },
}
```

#### Informer Configuration

Multiple strategies available:

**Default** (KubernetesBasedInformer):
```go
supplier := simple.DefaultInformerSupplier
```

**Optimized** (CustomCacheInformer with watch-list):
```go
supplier := simple.OptimizedInformerSupplier
config.UseWatchList = true
config.WatchListPageSize = 500
```

**Memcached** (distributed cache):
```go
supplier := func(kind resource.Kind, clients resource.ClientGenerator, opts operator.InformerOptions) (operator.Informer, error) {
    return operator.NewMemcachedInformer(kind, client, operator.MemcachedInformerOptions{
        ServerAddrs: []string{"memcached:11211"},
    })
}
```

### Development Workflows

#### Creating a New Kind

```bash
# In your project
grafana-app-sdk project kind add ServiceCatalog

# Edit kinds/servicecatalog.cue
# Define spec and status fields

# Generate code
grafana-app-sdk generate

# Generated files:
# - pkg/generated/resource/servicecatalog/v1/servicecatalog_object_gen.go
# - pkg/generated/resource/servicecatalog/v1/servicecatalog_schema_gen.go
# - definitions/servicecatalog.crd.yaml
```

#### Implementing a Reconciler

```go
type MyReconciler struct {
    store *resource.TypedStore[*v1.MyKind]
    logger *slog.Logger
}

func (r *MyReconciler) Reconcile(ctx context.Context, req operator.ReconcileRequest) (operator.ReconcileResult, error) {
    logger := logging.FromContext(ctx)
    
    // Cast to typed object (or use operator.TypedReconciler[T])
    obj, ok := req.Object.(*v1.MyKind)
    if !ok {
        return operator.ReconcileResult{}, fmt.Errorf("unexpected type")
    }
    
    logger.Info("Reconciling", "name", obj.GetName(), "generation", obj.GetGeneration())
    
    // Check if already reconciled
    if obj.Status.LastAppliedGeneration == obj.GetGeneration() {
        return operator.ReconcileResult{}, nil
    }
    
    // Perform business logic
    if err := r.doSomething(ctx, obj); err != nil {
        if isRetryable(err) {
            // Explicit retry after delay
            return operator.ReconcileResult{RequeueAfter: time.Minute}, nil
        }
        // Let RetryPolicy handle it
        return operator.ReconcileResult{}, err
    }
    
    // Update status subresource
    obj.Status.LastAppliedGeneration = obj.GetGeneration()
    obj.Status.ObservedTime = time.Now().Unix()
    _, err := r.store.UpdateSubresource(ctx, obj.GetStaticMetadata().Identifier(), resource.SubresourceStatus, obj)
    
    return operator.ReconcileResult{}, err
}
```

#### Debugging Techniques

**Enable verbose logging**:
```go
import "github.com/grafana/grafana-app-sdk/logging"

logger := logging.FromContext(ctx)
logger.Debug("Processing event", "key", key, "action", action)
```

**Check informer sync**:
```go
if !informer.HasSynced() {
    logger.Warn("Informer not synced, may be missing events")
}
```

**Profile hot paths**:
```bash
# Generate profiles
make bench-profile

# Analyze memory
go tool pprof -http=:8080 target/profiles/mem.out

# Analyze CPU
go tool pprof -http=:8080 target/profiles/cpu.out

# Interactive mode
go tool pprof target/profiles/mem.out
(pprof) top10        # Top 10 allocators
(pprof) list FuncName # Show annotated source
(pprof) web          # Generate call graph
```

**Trace reconciliation flow**:
```bash
# Enable OpenTelemetry tracing
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"

# Or configure in code
simple.SetTraceProvider(simple.OpenTelemetryConfig{
    Host: "localhost",
    Port: 4317,
    ConnType: simple.OTelConnTypeGRPC,
    ServiceName: "myapp",
})
```

### Testing Best Practices

#### Table-Driven Tests (Standard Pattern)

```go
func TestReconcile(t *testing.T) {
    tests := []struct{
        name          string
        input         *v1.MyKind
        wantErr       bool
        wantRequeue   bool
    }{
        {
            name: "successful reconcile",
            input: &v1.MyKind{
                ObjectMeta: metav1.ObjectMeta{Name: "test"},
                Spec: v1.MyKindSpec{Field: "value"},
            },
            wantErr: false,
        },
        {
            name: "missing required field",
            input: &v1.MyKind{
                ObjectMeta: metav1.ObjectMeta{Name: "test"},
                Spec: v1.MyKindSpec{},
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            reconciler := NewMyReconciler(...)
            result, err := reconciler.Reconcile(ctx, operator.ReconcileRequest{
                Object: tt.input,
            })
            
            if tt.wantErr {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
            
            if tt.wantRequeue {
                assert.True(t, result.RequeueAfter > 0)
            }
        })
    }
}
```

#### Benchmark Tests

```go
func BenchmarkReconcile(b *testing.B) {
    reconciler := setupReconciler()
    obj := createTestObject()
    req := operator.ReconcileRequest{Object: obj}
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        _, err := reconciler.Reconcile(context.Background(), req)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

### Common Pitfalls & Solutions

#### Pitfall 1: Infinite Reconcile Loops

**Problem**: Updating spec in reconciler triggers another reconcile event

**Solution**: Only update status subresource or check `metadata.generation`:
```go
// BAD: Updates spec, triggers loop
obj.Spec.ComputedField = calculate()
store.Update(ctx, obj)

// GOOD: Update status only
obj.Status.ComputedValue = calculate()
store.UpdateSubresource(ctx, identifier, resource.SubresourceStatus, obj)

// GOOD: Check if already processed
if obj.Status.LastAppliedGeneration == obj.GetGeneration() {
    return ReconcileResult{}, nil
}
```

#### Pitfall 2: Missing Events During Downtime

**Problem**: Operator was down, missed delete events, leaked resources

**Solution**: Use OpinionatedWatcher/OpinionatedReconciler with finalizers:
```go
watcher := operator.NewOpinionatedWatcher(
    operator.OpinionatedWatcherConfig{
        Watcher: &myWatcher,
        Finalizer: "myapp.grafana.app/finalizer",
    },
)

// Implement Sync() method
func (w *MyWatcher) Sync(ctx context.Context, obj resource.Object) error {
    // Called for objects that may have changed during downtime
    // Process as if it's a new Add event
}
```

#### Pitfall 3: Webhook Timeouts

**Problem**: Webhook calls external service, times out, blocks all API requests

**Solution**: Keep webhooks fast (<1s), defer work to reconciler:
```go
// BAD: Slow webhook
func (v *Validator) Validate(ctx context.Context, req *app.AdmissionRequest) error {
    // Calls external service (slow!)
    return externalService.Validate(req.Object)
}

// GOOD: Fast webhook, reconciler does work
func (v *Validator) Validate(ctx context.Context, req *app.AdmissionRequest) error {
    // Basic validation only
    if obj.Spec.RequiredField == "" {
        return fmt.Errorf("requiredField is required")
    }
    return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileResult, error) {
    // Slow work here (async)
    result, err := externalService.Process(req.Object)
    // Update status with result
}
```

#### Pitfall 4: Version Conflicts

**Problem**: Concurrent updates cause "resource version conflict" errors

**Solution**: Implement retry logic with fresh GET:
```go
const maxRetries = 3

for i := 0; i < maxRetries; i++ {
    // Get fresh copy
    obj, err := store.Get(ctx, identifier)
    if err != nil {
        return ReconcileResult{}, err
    }
    
    // Make changes
    obj.Status.Value = newValue
    
    // Update (may conflict)
    _, err = store.UpdateSubresource(ctx, identifier, resource.SubresourceStatus, obj)
    if err == nil {
        break // Success
    }
    
    if !isConflict(err) {
        return ReconcileResult{}, err
    }
    // Retry on conflict
}
```

#### Pitfall 5: Memory Leaks in High-Volume Scenarios

**Problem**: Informer caches all objects in memory, OOM in large clusters

**Solution**: Use CustomCacheInformer with memcached or watch-list:
```go
// Option 1: Memcached (distributed cache)
informer, err := operator.NewMemcachedInformer(kind, client, operator.MemcachedInformerOptions{
    ServerAddrs: []string{"memcached:11211"},
})

// Option 2: Watch-list (streaming, lower memory)
config.InformerConfig.UseWatchList = true
config.InformerConfig.WatchListPageSize = 500

// Option 3: Filter at source with label selectors
config.InformerConfig.ListWatchOptions = operator.ListWatchOptions{
    LabelFilters: []string{"app=myapp"},
}
```

### Performance Optimization Context

The codebase is actively being optimized for production scale (10k-50k resources):

**Current Optimization Targets** (from performance docs):

1. **Memory allocations in JSONCodec.Write** (41.85% of allocations)
   - Pooling metadata maps
   - Reducing reflection overhead
   
2. **Object copying via reflection** (28.94% of allocations)
   - Implementing custom MarshalJSON
   - Code generation for known types

3. **DeltaFIFO operations** (client-go component)
   - Considering alternative queue implementations
   - Object pooling

4. **Lock contention in processor.distribute()** (58.63% of CPU)
   - Lock-free data structures
   - Batching events

**Recent Achievements**:
- 34.8% memory reduction (503.7 MB → 328.6 MB)
- 43.5% fewer allocations (10.88M → 6.15M)
- 16% faster execution (1114ms → 936ms)

**Implications**:
- Performance-critical code may change
- Always run benchmarks before/after changes
- Consult performance docs before optimizing

### Migration & Versioning

**Status**: Experimental (v0.x)

**Breaking changes**: May occur in minor versions

**Migration guides**: `/docs/migrations/README.md`

**Retracted versions**:
- v0.20.0 (binary build errors)
- v0.18.4 (binary build errors)
- v0.18.3 (GOPROXY conflicts)

**Best practices**:
- Pin specific versions in go.mod
- Read migration guides when upgrading
- Test thoroughly after upgrades
- Subscribe to releases on GitHub

### Additional Resources

**Essential Documentation**:
- Tutorial: `/docs/tutorials/issue-tracker/README.md` (start here!)
- Platform concepts: `/docs/application-design/platform-concepts.md`
- Operators: `/docs/operators.md`
- Reconcilers: `/docs/writing-a-reconciler.md`
- Admission control: `/docs/admission-control.md`
- CUE kinds: `/docs/custom-kinds/writing-kinds.md`
- Code generation: `/docs/code-generation.md`
- Kubernetes primer: `/docs/kubernetes.md`
- Architecture diagrams: `/docs/architecture/reconciliation.md`

**Examples**:
- Simple operator: `/examples/operator/simple/`
- API server: `/examples/apiserver/`
- Resource usage: `/examples/resource/`

**Community**:
- GitHub: https://github.com/grafana/grafana-app-sdk
- Issues: https://github.com/grafana/grafana-app-sdk/issues
- Contributing: `/CONTRIBUTING`

### Quick Reference: File Paths

All file paths are absolute from repository root `/Users/igor/Code/grafana/grafana-app-sdk/`:

**Core Interfaces**:
- `resource.Object`: `/resource/object.go`
- `resource.Kind`: `/resource/kind.go`
- `operator.Informer`: `/operator/informer_*.go`
- `operator.Reconciler`: `/operator/reconciler.go`
- `simple.App`: `/simple/app.go`

**Implementations**:
- CustomCacheInformer: `/operator/informer_customcache.go`
- k8s Client: `/k8s/client.go`
- Reflector: `/k8s/cache/reflector.go`
- JSONCodec: `/resource/kind.go:113-207`

**CLI**:
- Main: `/cmd/grafana-app-sdk/main.go`
- Generate: `/cmd/grafana-app-sdk/generate.go`
- Project: `/cmd/grafana-app-sdk/project.go`

**Configuration**:
- Makefile: `/Makefile`
- Linter config: `/.golangci.yml`
- Go modules: `/go.mod`, `/go.work`

---

*This analysis was generated on 2025-10-27 based on the codebase state at commit 61bc84a (branch: chore/watchlist-page-size).*
