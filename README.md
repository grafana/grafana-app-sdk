# grafana-app-sdk

![Build Status](https://github.com/grafana/grafana-app-sdk/actions/workflows/main.yml/badge.svg) 
![Release Status](https://github.com/grafana/grafana-app-sdk/actions/workflows/release.yml/badge.svg)

<hr/>

**This repository is currently *experimental*, which means that interfaces and behavior may change as it evolves.**

<hr/>

The `grafana-app-sdk` is an SDK used for developing apps/extensions for grafana. It encompasses the following:
* Code generation and project boilerplate generation from a CLI
* Storage and version management of unique schemas (kinds)
* An reconciler/event-based model for handling changes to data
* Both simple building blocks and opinionated solutions for building operators and plugins
* An interface layer over your storage system, allowing any compatible API-server-like system to be plugged in

## Quickstart

If you want to try out using the SDK, there is a [tutorial to build a simple issue tracker](docs/tutorials/issue-tracker/README.md) which starts you from zero and brings you through to deploying a simple app built using the SDK.

## Installation of the CLI

### go install
The simplest way to install the CLI is with `go install`. To get the latest version, use:
```bash
go install github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk@latest
```
(ensure that your `GOPATH/bin` (typically `$HOME/go/bin`) is in your `PATH`)

### Binary
If you prefer to download a binary and add it to your `PATH`, you can install a binary from the releases page:

1. [Visit the latest release page](https://github.com/grafana/grafana-app-sdk/releases/latest)
2. Find the appropriate artifact for your OS and architecture
3. Download the artifact and untar it into your PATH

Once you have a version of the CLI installed, you can test it with:
```
grafana-app-sdk version
```
Which will print the version of the CLI which you have installed.

## App Design

An agnotic view of an app using the SDK looks like:

![Application Using SDK Diagram](docs/diagrams/app_logic.png)

The SDK handles interaction with the storage system, and surfacing simple interfaces for performing normal operations on resources in storage, as well as creating controller/operator loops that react to changes in the resources in the storage system.

A typical grafana app deployment might look more like:

![Application Using SDK Diagram](docs/diagrams/design_pattern_simple.png)

For more details on application design, see [App Design](docs/app_design.md).

## CLI Usage

Full CLI usage is covered in [CLI docs page](docs/cli.md), but for a brief overview of the commands:

| Command | Description |
|---------|-------------|
| `version` | Prints the version (use `-v` for a verbose print) |
| `generate` | Generates code from your CUE kinds (defaults to CUE in `schemas`, use `-c`/`--cuepath` to speficy a different CUE path) |
| `project init <module name>` | Creates a project template, including directory structure, go module, CUE module, and Makefile |
| `project component add <component>` | Add boilerplate code for a component to your project. `<component>` options are `frontend`, `backend`, and `operator` |
| `project local init` | Initialize the `./local` directory for a local development environment (done automatically by `project init`) |
| `project local generate` | Generate a YAML bundle for local deployment, based on your CUE kinds and `./local/config.yaml` |

## Library Usage

Read more about the library usage in the [docs](docs/README.md) directory.

### `resource`

Package exposing interfaces for resources and client, and the various kind of `Store` objects, which allow you to use key/value store-like objects for handling your data. 

See [Resource Objects](docs/resource-objects.md) and [Resource Stores](docs/resource-stores.md) for more information on how to work with resource `Objects` and use `Store` objects.

### `operator`

Package containing operator code, for building out event-based and/or reconciler-based operators.


See [Operators & Event-Based Design](docs/operators.md) for more details on building operators.

### `k8s`

Implementation of a storage layer using a kubernetes-compatible API server.

### `plugin`

Wrapper for the grafana backend plugin go SDK which adds layers on top for dealing with lazy-loading, encoding kube configs in secure JSON data, and HTTP-like routing.

See [Writing Back-End Plugins](docs/plugin-backend.md) for more details on using the `plugin` package.

## Dependencies

The grafana-app-sdk code generation uses [kindsys](https://github.com/grafana/grafana-app-sdk/kindsys) for it's CUE kind definitions, and [thema](https://github.com/grafana/thema) for the generated code's unmarshaling.

If you use the generated code, you must take a project dependency on [thema](https://github.com/grafana/thema), as it is used as a dependency in the generated code (kindsys is only used in the generation process, and is not needed in your project).

## Further Reading

Please see the [/docs](docs/README.md) directory for full documentation,
or take a look at the [Design Patterns](docs/design-patterns.md), [Kubernetes Concepts](docs/kubernetes.md), or the [tutorial](docs/tutorials/issue-tracker/README.md).

The `examples` directory contains runnable example projects that use different SDK components.

Each package also contains a README.md detailing package usage and simple examples.

## Contributing

See our [contributing guide](CONTRIBUTING) for instructions on how to contribute to the development of the SDK.
