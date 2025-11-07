# Key Concepts in grafana-app-sdk

This document lists the novel concepts and terminology introduced by the grafana-app-sdk. These are specific to this SDK and not general-purpose concepts like Kubernetes.

## Application Architecture Patterns

These patterns define the different ways you can structure your Grafana app depending on your requirements for validation, business logic, and API customization.

- **CRD-based applications** - Apps with UI and basic validation, no backend logic  
  *Read more: [Application Design Patterns](../../application-design/README.md#frontend-only-applications)*

- **Operator-based applications** - Apps with backend operators for validation, mutation, conversion, and reconciliation  
  *Read more: [Application Design Patterns](../../application-design/README.md#operator-based-applications)*

- **Custom API applications** - Apps with extension API servers for custom endpoints  
  *Read more: [Application Design Patterns](../../application-design/README.md#applications-with-custom-apis)*

## Core Abstractions

These are the fundamental building blocks for working with custom resources in the SDK, providing type-safe interfaces for resource manipulation and storage.

- **Kind** - A blueprint/schema for a type of custom resource, consisting of a name, group, version, and schema  
  *Read more: [Custom Kinds](../../custom-kinds/README.md) | [Implementation](../../../resource/kind.go)*

- **resource.Object** - The fundamental interface representing an instance of a Kind  
  *Read more: [Resource Objects](../../resource-objects.md) | [Implementation](../../../resource/object.go)*

- **resource.Kind** - Type containing Kind metadata and marshaling capabilities  
  *Read more: [Implementation](../../../resource/kind.go)*

- **resource.Schema** - Interface defining Group/Version/Kind metadata  
  *Read more: [Implementation](../../../resource/schema.go)*

- **resource.Client** - Generic interface for CRUD operations on resources  
  *Read more: [Implementation](../../../resource/client.go)*

- **resource.Store** - Key-value store abstraction for working with resources  
  *Read more: [Resource Stores](../../resource-stores.md) | [Implementation](../../../resource/store.go)*

- **TypedStore** - Type-safe wrapper around Store for a specific Kind  
  *Read more: [Resource Stores](../../resource-stores.md#typedstore) | [Implementation](../../../resource/typedstore.go)*

- **resource.Codec** - Handles serialization/deserialization of resource objects  
  *Read more: [Implementation](../../../resource/kind.go)*

## Operator/Controller Components

These components enable event-driven operator patterns, allowing your app to react to resource changes asynchronously without blocking API requests.

- **Informer** - Watches for changes to resources and emits events (CustomCache, Kubernetes native, concurrent variants)  
  *Read more: [Operators](../../operators.md) | [CustomCache Implementation](../../../operator/informer_customcache.go) | [Kubernetes Implementation](../../../operator/informer_kubernetes.go)*

- **Reconciler** - Single function that handles all event types for asynchronous state reconciliation  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md) | [Implementation](../../../operator/reconciler.go)*

- **Watcher** - Separate Add/Update/Delete functions for handling resource events  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md#reconciler-vs-watcher) | [Implementation](../../../operator/simplewatcher.go)*

- **OpinionatedWatcher** - Watcher with finalizer logic to prevent missed events during downtime  
  *Read more: [Operators](../../operators.md) | [Implementation](../../../operator/opinionatedwatcher.go)*

- **OpinionatedReconciler** - Reconciler with finalizer logic for event reliability  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md) | [Implementation](../../../operator/reconciler.go)*

- **InformerController** - Controller managing multiple informers and watchers/reconcilers  
  *Read more: [Implementation](../../../operator/informer_controller.go)*

- **DeltaFIFO** - Queue for resource deltas (from Kubernetes client-go)  
  *Read more: [Kubernetes Documentation](https://pkg.go.dev/k8s.io/client-go/tools/cache#DeltaFIFO)*

- **Reflector** - Component that performs LIST and WATCH operations  
  *Read more: [Implementation](../../../k8s/cache/reflector.go)*

- **watch-list protocol** - Streaming LIST that transitions to WATCH for memory efficiency  
  *Read more: [Architecture Documentation](../../architecture/reconciliation.md)*

## Admission Control

Admission control components intercept API requests synchronously to validate, mutate, or convert resources before they are stored, ensuring data quality and consistency.

- **ValidatingAdmissionController** - Interface for validating incoming requests (synchronous)  
  *Read more: [Admission Control](../../admission-control.md) | [Implementation](../../../resource/admission.go)*

- **MutatingAdmissionController** - Interface for altering incoming requests before storage (synchronous)  
  *Read more: [Admission Control](../../admission-control.md) | [Implementation](../../../resource/admission.go)*

- **Conversion webhook** - Handles conversion between different versions of a Kind  
  *Read more: [Managing Multiple Versions](../../custom-kinds/managing-multiple-versions.md) | [Implementation](../../../k8s/conversion.go)*

- **WebhookServer** - Server exposing validation, mutation, and conversion webhooks  
  *Read more: [Admission Control](../../admission-control.md) | [Implementation](../../../k8s/webhooks.go)*

## Resource Structure

Understanding resource structure is essential for properly designing your custom resources and managing their lifecycle, including user-editable data, operator state, and metadata.

- **Spec** - User-editable main body of a resource  
  *Read more: [Writing Kinds](../../custom-kinds/writing-kinds.md#schemas) | [Resource Objects](../../resource-objects.md)*

- **Subresources** - Additional sub-objects (typically operator-modified), like `status` and `scale`  
  *Read more: [Resource Objects](../../resource-objects.md) | [Writing Kinds](../../custom-kinds/writing-kinds.md#schemas)*

- **StaticMetadata** - Immutable metadata used to uniquely identify a resource  
  *Read more: [Resource Objects](../../resource-objects.md) | [Implementation](../../../resource/metadata.go)*

- **CommonMetadata** - General metadata including app-platform specific fields (updateTimestamp, createdBy, etc.)  
  *Read more: [Resource Objects](../../resource-objects.md) | [Implementation](../../../resource/metadata.go)*

- **Generation** - Metadata property that increments when spec changes (used to track reconciliation)  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md)*

- **ResourceVersion** - Version number that changes on any update (for optimistic locking)  
  *Read more: [Platform Concepts](../../application-design/platform-concepts.md#document-oriented-unified-storage)*

## API Structure

These concepts define how resources are organized and accessed through the API, following Kubernetes-style standardized RESTful API conventions.

- **Group** - API grouping, typically correlates to app name (e.g., `myapp.ext.grafana.com`)  
  *Read more: [Platform Concepts](../../application-design/platform-concepts.md#standardized-restful-apis) | [Custom Kinds](../../custom-kinds/README.md)*

- **Version** - API version (stable like `v1` or unstable like `v1alpha1`, `v2beta1`)  
  *Read more: [App Manifest](../../app-manifest.md#versions) | [Managing Multiple Versions](../../custom-kinds/managing-multiple-versions.md)*

- **GVK (GroupVersionKind)** - Complete identifier for a resource schema  
  *Read more: [Custom Kinds](../../custom-kinds/README.md) | [Implementation](../../../resource/schema.go)*

- **GVR (GroupVersionResource)** - Group/Version/Resource identifier  
  *Read more: [Custom Kinds](../../custom-kinds/README.md)*

- **GroupKind** - Group and Kind combination  
  *Read more: [Custom Kinds](../../custom-kinds/README.md)*

- **Plural** - Plural name of a kind used in API paths  
  *Read more: [Custom Kinds](../../custom-kinds/README.md)*

## SDK-Specific Components

These high-level components provide simplified APIs for building complete applications, managing application lifecycle, and declaring app capabilities.

- **App manifest** - CUE/YAML file describing app's API versions, capabilities, and permissions  
  *Read more: [App Manifest](../../app-manifest.md) | [Implementation](../../../app/manifest.go)*

- **ManagedKinds** - Kinds owned and managed by your app  
  *Read more: [Application Design](../../application-design/README.md) | [Implementation](../../../simple/app.go)*

- **UnmanagedKinds** - Kinds from other apps that your app watches  
  *Read more: [Watching Unowned Resources](../../watching-unowned-resources.md) | [Implementation](../../../simple/app.go)*

- **simple.App** - High-level opinionated application builder  
  *Read more: [Operators](../../operators.md) | [Implementation](../../../simple/app.go)*

- **app.Runner** - Multi-component application runner  
  *Read more: [Implementation](../../../app/runner.go)*

- **operator.Runner** - Operator lifecycle manager  
  *Read more: [Implementation](../../../operator/runner.go)*

## Code Generation

Code generation automates the creation of type-safe Go and TypeScript code, CRDs, and boilerplate application components from your CUE schemas, eliminating manual coding errors and ensuring consistency.

- **Kind code generation** - Generate Go/TypeScript from CUE kinds (run repeatedly)  
  *Read more: [Code Generation](../../code-generation.md) | [Writing Kinds](../../custom-kinds/writing-kinds.md#generating-code)*

- **Project component generation** - One-time scaffold generation (frontend, backend, operator)  
  *Read more: [Code Generation](../../code-generation.md#project-component-generation) | [CLI Documentation](../../cli.md)*

- **Jennies** - Individual code generators in the codegen pipeline  
  *Read more: [Implementation](../../../codegen/jennies/)*

- **Golden files** - Reference files for testing code generation output  
  *Read more: [Testing Documentation](../../../codegen/testing/)*

## Development Tools

These utilities and interfaces simplify common development tasks like managing multiple resource types, implementing type-safe reconciliation logic, and handling retry policies.

- **ClientRegistry** - Manages multiple clients for different resource types  
  *Read more: [Implementation](../../../k8s/client_registry.go)*

- **ClientGenerator** - Factory for creating resource clients  
  *Read more: [Implementation](../../../resource/client.go)*

- **TypedReconciler** - Type-safe reconciler that avoids manual casting  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md#an-example-reconciler) | [Implementation](../../../operator/reconciler.go)*

- **ReconcileRequest** - Request object passed to reconcilers containing the resource and action  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md) | [Implementation](../../../operator/reconciler.go)*

- **ReconcileResult** - Response from reconciler including optional RequeueAfter time  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md) | [Implementation](../../../operator/reconciler.go)*

- **RetryPolicy** - Policy dictating reconciliation retry behavior  
  *Read more: [Operators](../../operators.md) | [Implementation](../../../operator/informer_controller.go)*

## Local Development

These components facilitate local testing and development by providing automated cluster setup, file mounting, and deployment orchestration without requiring a full production environment.

- **Tiltfile** - Configuration for Tilt (local development orchestrator)  
  *Read more: [Local Development](../../local-development.md)*

- **K3D config** - Configuration for local Kubernetes cluster  
  *Read more: [Local Development](../../local-development.md#setup)*

- **mounted-files** - Directory for files mounted into K3D cluster  
  *Read more: [Local Development](../../local-development.md#setup)*

- **dev-bundle.yaml** - Generated bundle for local deployment  
  *Read more: [Local Development](../../local-development.md#local-deployment)*

## Metadata & Tracking

These metadata fields and mechanisms enable proper resource lifecycle management, reconciliation tracking, and audit trails for understanding resource history and ownership.

- **Finalizers** - Markers preventing deletion until cleanup is complete  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md#considerations-when-writing-a-reconciler) | [Implementation](../../../operator/finalizers.go)*

- **LastAppliedGeneration** - Status field tracking which spec generation was last reconciled  
  *Read more: [Writing a Reconciler](../../writing-a-reconciler.md#an-example-reconciler)*

- **updateTimestamp** - Custom metadata tracking last update time  
  *Read more: [Resource Objects](../../resource-objects.md) | [Admission Control](../../admission-control.md#opinionated-controllers)*

- **createdBy/updatedBy** - Custom metadata tracking user identity  
  *Read more: [Resource Objects](../../resource-objects.md) | [Admission Control](../../admission-control.md#opinionated-controllers)*

