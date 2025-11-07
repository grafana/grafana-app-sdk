# Key Concepts in grafana-app-sdk

This document lists the novel concepts and terminology introduced by the grafana-app-sdk. These are specific to this SDK and not general-purpose concepts like Kubernetes.

## Application Architecture Patterns

- **CRD-based applications** - Apps with UI and basic validation, no backend logic
- **Operator-based applications** - Apps with backend operators for validation, mutation, conversion, and reconciliation
- **Custom API applications** - Apps with extension API servers for custom endpoints

## Core Abstractions

- **Kind** - A blueprint/schema for a type of custom resource, consisting of a name, group, version, and schema
- **resource.Object** - The fundamental interface representing an instance of a Kind
- **resource.Kind** - Type containing Kind metadata and marshaling capabilities
- **resource.Schema** - Interface defining Group/Version/Kind metadata
- **resource.Client** - Generic interface for CRUD operations on resources
- **resource.Store** - Key-value store abstraction for working with resources
- **TypedStore** - Type-safe wrapper around Store for a specific Kind
- **resource.Codec** - Handles serialization/deserialization of resource objects

## Operator/Controller Components

- **Informer** - Watches for changes to resources and emits events (CustomCache, Kubernetes native, concurrent variants)
- **Reconciler** - Single function that handles all event types for asynchronous state reconciliation
- **Watcher** - Separate Add/Update/Delete functions for handling resource events
- **OpinionatedWatcher** - Watcher with finalizer logic to prevent missed events during downtime
- **OpinionatedReconciler** - Reconciler with finalizer logic for event reliability
- **InformerController** - Controller managing multiple informers and watchers/reconcilers
- **DeltaFIFO** - Queue for resource deltas (from Kubernetes client-go)
- **Reflector** - Component that performs LIST and WATCH operations
- **watch-list protocol** - Streaming LIST that transitions to WATCH for memory efficiency

## Admission Control

- **ValidatingAdmissionController** - Interface for validating incoming requests (synchronous)
- **MutatingAdmissionController** - Interface for altering incoming requests before storage (synchronous)
- **Conversion webhook** - Handles conversion between different versions of a Kind
- **WebhookServer** - Server exposing validation, mutation, and conversion webhooks

## Resource Structure

- **Spec** - User-editable main body of a resource
- **Subresources** - Additional sub-objects (typically operator-modified), like `status` and `scale`
- **StaticMetadata** - Immutable metadata used to uniquely identify a resource
- **CommonMetadata** - General metadata including app-platform specific fields (updateTimestamp, createdBy, etc.)
- **Generation** - Metadata property that increments when spec changes (used to track reconciliation)
- **ResourceVersion** - Version number that changes on any update (for optimistic locking)

## API Structure

- **Group** - API grouping, typically correlates to app name (e.g., `myapp.ext.grafana.com`)
- **Version** - API version (stable like `v1` or unstable like `v1alpha1`, `v2beta1`)
- **GVK (GroupVersionKind)** - Complete identifier for a resource schema
- **GVR (GroupVersionResource)** - Group/Version/Resource identifier
- **GroupKind** - Group and Kind combination
- **Plural** - Plural name of a kind used in API paths

## SDK-Specific Components

- **App manifest** - CUE/YAML file describing app's API versions, capabilities, and permissions
- **ManagedKinds** - Kinds owned and managed by your app
- **UnmanagedKinds** - Kinds from other apps that your app watches
- **simple.App** - High-level opinionated application builder
- **app.Runner** - Multi-component application runner
- **operator.Runner** - Operator lifecycle manager

## Code Generation

- **Kind code generation** - Generate Go/TypeScript from CUE kinds (run repeatedly)
- **Project component generation** - One-time scaffold generation (frontend, backend, operator)
- **Jennies** - Individual code generators in the codegen pipeline
- **Golden files** - Reference files for testing code generation output

## Development Tools

- **ClientRegistry** - Manages multiple clients for different resource types
- **ClientGenerator** - Factory for creating resource clients
- **TypedReconciler** - Type-safe reconciler that avoids manual casting
- **ReconcileRequest** - Request object passed to reconcilers containing the resource and action
- **ReconcileResult** - Response from reconciler including optional RequeueAfter time
- **RetryPolicy** - Policy dictating reconciliation retry behavior

## Local Development

- **Tiltfile** - Configuration for Tilt (local development orchestrator)
- **K3D config** - Configuration for local Kubernetes cluster
- **mounted-files** - Directory for files mounted into K3D cluster
- **dev-bundle.yaml** - Generated bundle for local deployment

## Metadata & Tracking

- **Finalizers** - Markers preventing deletion until cleanup is complete
- **LastAppliedGeneration** - Status field tracking which spec generation was last reconciled
- **updateTimestamp** - Custom metadata tracking last update time
- **createdBy/updatedBy** - Custom metadata tracking user identity

