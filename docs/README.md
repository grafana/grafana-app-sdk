# Docs

This directory is a collection of documentation about the SDK library, CLI, and concepts.

If you are looking for documentation for migrating your code when you upgrade SDK versions, see [migrations](./migrations/README.md).

The easiest way to get a handle on the SDK is to run through the [sample project: issue tracker tutorial](./tutorials/issue-tracker/README.md). 
Docs in this directory will help further your understanding of the concepts touched on in the tutorial.

Godocs on exported library package code (such as `resource`, `operator`, `plugin`, and `k8s`) are also considered documentation.

### Table of Contents

| Document                                                          | Description |
|-------------------------------------------------------------------|-------------|
| [Application Design](./application-design/README.md)              | The typical design patterns of an app built with the SDK |
| [Custom Kinds](./custom-kinds/README.md)                          | What are kinds, how to write them, and how to use them |
| [Resource Objects](./resource-objects.md)                          | Describes the function and usage of the `resource.Object` interface |
| [Resource Stores](./resource-stores.md)                           | Describes the various "Store" types in the `resource` package, and why you may want to use one or another |
| [Kubernetes Concepts](./kubernetes.md)                             | A primer on some kubernetes concepts which are relevant to using the SDK backed by a kubernetes API server |
| [Writing an App](./writing-an-app.md)                              | How to build an application using the SDK |
| [Writing a Reconciler](./writing-a-reconciler.md)                  | How to write a reconciler for event-based processing |
| [Operators & Event-Based Design](./operators.md)                   | A brief primer on what operators/controllers are and working with event-based code |
| [Running as an Operator](./running-as-operator.md)                 | Operator runner lifecycle, configuration, and production deployment |
| [Watching Unowned Resources](./watching-unowned-resources.md)      | How to watch resources your operator doesn't own |
| [Admission Control](./admission-control.md)                        | How to set up admission control on your kinds for an API server |
| [Extension API Server](./extension-api-server.md)                  | Building a custom API server with the SDK |
| [Advanced Operator Features](./advanced-operator-features.md)      | Resource clients, informer strategies, retry processing, and runner patterns |
| [Observability](./observability.md)                                | Metrics, tracing, logging, and health checks |
| [Code Generation](./code-generation.md)                            | How to use CUE and the CLI for code generation |
| [App Manifest](./app-manifest.md)                                  | App manifest format and configuration |
| [CLI Reference](./cli.md)                                          | CLI command reference |
| [Local Dev Environment Setup](./local-development.md)              | How to use the CLI to set up a local development & testing environment |
| [Reconciliation Architecture](./architecture/reconciliation.md)    | Detailed architecture of the reconciliation system |

## Base Concepts of the SDK

The kernel at the center of most of what the SDK can be used for is a [Kind](custom-kinds/README.md), expressed in-code using the `resource.Object` interface and `resource.Kind` type, and typically written in CUE as a source-of-truth. 

Instances of a kind, referred to as "resources" or "objects," are then stored in an API server. The pre-built solution the SDK gives you currently is a kubernetes API server, 
but importantly, all of the tooling outside of the `k8s` package is actually implementation-agnostic.

To work directly with resources, you can use an instance of a `resource.Client`, or one of the [Store](./resource-stores.md)-type objects which simplifies interactions by allowing you to treat the system as a key-value store.

An important component of the SDK is the use of the operator pattern. Using the `operator` package, you can set up an operator to watch for changes to one or more kinds, and take actions based on the nature of those changes. This allows behavior to be decoupled from the actual input API, which is especially useful in the context of an API server which has multiple paths for a user to enter, modify, or delete resources. For more details on this, see [Design Patterns](./application-design/README.md).

There is also layering on top of the grafana backend plugin SDK, allowing you to create a REST API backend which can tie into your kinds,
to create a proxy to the back-end API server. For applications that need additional non-standard routes,
consider using an [Extension API Server](./extension-api-server.md) (see also [Applications with Custom APIs](./application-design/README.md#applications-with-custom-apis)).

## Documentation Improvements

If you notice places where documentation is lacking, confusing, or is out of date, please do not hesitate to create an [issue](https://github.com/grafana/grafana-app-sdk/issues) in this repository noting the specifics of what you found, and we will work to improve it. We also welcome documentation contributions as [Pull Requests](https://github.com/grafana/grafana-app-sdk/pulls).

