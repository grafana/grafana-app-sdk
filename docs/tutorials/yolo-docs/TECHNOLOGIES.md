# Technologies Used in grafana-app-sdk

This document lists the technologies and tools used by the grafana-app-sdk, categorized by whether they are hard requirements or replaceable defaults.

## Hard Requirements (Cannot be replaced)

These technologies are fundamental to how the SDK works and cannot be substituted:

- **CUE (cuelang.org/go)** - Used as the source-of-truth for Kind schemas; generates Go/TypeScript code, CRDs, and OpenAPI specs
- **Go 1.24.0** - Primary development language for backend operators and SDK implementation
- **Kubernetes API machinery (k8s.io/client-go, k8s.io/apimachinery)** - Core dependency for CRD interactions, client implementations, and API server integration
- **Grafana App Platform** - The platform this SDK targets; apps built with SDK are designed to run within Grafana

## Replaceable/Optional Technologies (Defaults that can be substituted)

These technologies are used by default but can be replaced with alternatives based on user preference or organizational requirements:

- **K3D (k3d.io)** - Used for local Kubernetes clusters; *could use Minikube, kind, or full Kubernetes cluster instead*
- **Tilt (tilt.dev)** - Used for local development orchestration; *could use kubectl/helm manually or other orchestration tools*
- **Docker** - Used for containerization; *Podman is explicitly mentioned as an alternative*
- **TypeScript** - Used for frontend plugin development; *only needed if building a frontend component*
- **Yarn** - JavaScript package manager for plugins; *could use npm instead*
- **Mage (magefile.org)** - Build tool used in generated projects; *could use make or other build tools*
- **Memcached** - Optional distributed cache for informers; *in-memory caching is the default*
- **Prometheus** - Used for metrics integration; *optional observability component*
- **OpenTelemetry** - Used for tracing support; *optional observability component*
- **testify** - Assertion library for tests; *could use other Go testing libraries*
- **golangci-lint** - Linter tool; *could use other linters*

## Implementation Choices (Internal, could change)

These are implementation details that users typically don't need to choose, but are worth knowing about:

- **k8s.io/apiserver** - Used for extension API servers in Custom API pattern; *currently required for that pattern but is an implementation detail*
- **client-go SharedInformer** - Underlying informer implementation; *SDK provides CustomCacheInformer as optimized alternative*


