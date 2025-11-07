# Technologies Used in grafana-app-sdk

This document lists the technologies and tools used by the grafana-app-sdk, categorized by whether they are hard requirements or replaceable defaults. These technologies are fundamental to how the SDK works:

- **CUE (cuelang.org/go)** - Used as the source-of-truth for [Custom Kind schemas](../../custom-kinds/README.md); generates Go/TypeScript code, CRDs, and OpenAPI specs
- **Go 1.24.0** - Primary language for backend operators and SDK implementation
- **Kubernetes API machinery (k8s.io/client-go, k8s.io/apimachinery)** - Core dependency for CRD interactions, client implementations, and API server integration
- **Grafana App Platform** - The platform this SDK targets; apps built with SDK are designed to run within Grafana
- **k8s.io/apiserver** - Used for extension API servers in Custom API pattern; *currently required for that pattern but is an implementation detail*
