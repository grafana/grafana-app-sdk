# Docs

This directory is a collection of documentation about the SDK library, CLI, and concepts.

If you are looking for documentation for migrating your code when you upgrade SDK versions, see [migrations](./migrations/README.md).

The easiest way to get a handle on the SDK is to run through the [sample project: issue tracker tutorial](./tutorials/issue-tracker/README.md). 
Docs in this directory will help further your understanding of the concepts touched on in the tutorial.

Godocs on exported library package code (such as `resource`, `operator`, `plugin`, and `k8s`) are also considered documentation.

### Table of Contents

| Document                                              | Description |
|-------------------------------------------------------|-------------|
| [Application Design](./application-design/README.md)  | The typical design patterns of an app built with the SDK |
| [Custom Kinds](./custom-kinds/README.md)              | What are kinds, how to write them, and how to use them |
| [Resource Objects](./resource-objects.md)             | Describes the function and usage of the `resource.Object` interface |
| [Resource Stores](./resource-stores.md)               | Describes the various "Store" types in the `resource` package, and why you may want to use one or another |
| [Operators & Event-Based Design](./operators.md)      | A brief primer on what operators/controllers are and working with event-based code |
| [Informers](./informers.md)                           | Where an operator's events come from, which informer to use, and how to configure it |
| [Code Generation](./code-generation.md)               | How to use CUE and the CLI for code generation. |
| [Local Dev Environment Setup](./local-development.md) | How to use the CLI to set up a local development & testing environment |
| [Admission Control](./admission-control.md)           | How to add validation and mutation logic to your kinds |

## Base Concepts of the SDK

The kernel at the center of most of what the SDK can be used for is a [Kind](custom-kinds/README.md), expressed in-code using the `resource.Object` interface and `resource.Kind` type, and typically written in CUE as a source-of-truth. 

Instances of a kind, referred to as "resources" or "objects," are stored and served by the Grafana App Platform. Your app does not run its own API server or storage; 
it declares its kinds in its [manifest](./app-manifest.md), and the platform exposes standard CRUD, list, and watch APIs for them.

To work directly with resources, you can use an instance of a `resource.Client`, or one of the [Store](./resource-stores.md)-type objects which simplifies interactions by allowing you to treat the system as a key-value store.

An important component of the SDK is the use of the operator pattern. Using the `operator` package, you can set up an operator to watch for changes to one or more kinds, and take actions based on the nature of those changes. This allows behavior to be decoupled from the actual input API, which is especially useful because users can create, modify, or delete resources through many paths (your UI, `kubectl`, gitops tooling, other apps). For more details on this, see [Design Patterns](./application-design/README.md).

Where those change events come from, and what the informer holds in memory to produce them, is configurable: a kubernetes watch by default, or — experimentally, through the `resourcebus` package and `operator.BusInformer` — a message bus the storage layer publishes to, which lets an app whose reconcilers read the object they act on keep no per-object cache at all. See [Informers](./informers.md).

Finally, the experimental `plugin` package runs an `app.App` inside a grafana app plugin's backend (see [the plugin protocol v3 example](../examples/plugin/README.md)).
Frontends should call the platform API directly, and use custom routes if they need non-standard endpoints (see [Applications with Custom APIs](./application-design/README.md#applications-with-custom-apis)).

## Documentation Improvements

If you notice places where documentation is lacking, confusing, or is out of date, please do not hesitate to create an [issue](https://github.com/grafana/grafana-app-sdk/issues) in this repository noting the specifics of what you found, and we will work to improve it. We also welcome documentation contributions as [Pull Requests](https://github.com/grafana/grafana-app-sdk/pulls).

