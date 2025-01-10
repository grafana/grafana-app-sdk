# Code Generation

Code generation done by the grafana-app-sdk can be broadly split into two buckets: what we call **kind code generation** which is run to generate code from your CUE kinds which can be used in your app, and **project component generation**, which is a run-once kind of codegen that scaffolds your project with boilerplate code that you can then alter as you see fit.

**code generation** is done with the `generate` command in the CLI, and **project component generation** is done with the `project component add` command in the CLI (also note that `project init` is a special case of codegen that scaffolds your initial project with opinionated defaults). The general workflow is that component generation is run only once, while kind code generation is run whenever you update or add a kind, or whenever you update your version of the grafana-app-sdk (to ensure that your generated code works with the new library version).

## Kind Code Generation

Code generation turns kinds written in CUE into go and TypeScript code which can be used to write your app logic. 
A full breakdown on writing CUE kinds and using them with the CLI's code generation can be found in the [Writing Kinds](./custom-kinds/writing-kinds.md) document.

Kind codegen uses `grafana-app-sdk generate` as its base commands, and uses a few flags that you can leave as default values if you use the setup that `grafana-app-sdk project init` gives you. The full command looks like:
```
grafana-app-sdk generate [-s|--source=kinds] [-g|--gogenpath=pkg/generated] [-t|--tsgenpath=plugin/src/generated] [--defencoding=json] [--defpath=definitions]
```
This command scans the `source` directory for CUE files, and parses all top-level fields in all present CUE files as CUE kinds. If kind validation encounters any errors, no files will be written, and the validation error(s) will be printed out. On successful generation: 
* kind go code will be written to `gogenpath`, with a package for each unique kind-version combination
* kind TypeScript code will be written to `tsgenpath`, with a folder for each unique kind-version combination
* kind CRD files and app manifest will be written to `defpath`, encoded as JSON or YAML based on `defencoding`, with a CRD file per kind

> [!IMPORTANT]
> Because the interfaces that the grafana-app-sdk libraries use can change, be sure to run kind code generation using a version of the `grafana-app-sdk` CLI that matches the version of the dependency you use in your project. Whenever you update the dependency, make sure you re-run the kind code generation as well.

Please see [Writing Kinds](./custom-kinds/writing-kinds.md) for a more detailed look at kind code generation from CUE.

## Project Component Generation

Project component generation is used to add boilerplate code for a "component" of your app. Components understood by the SDK are:
* `frontend` - a frontend plugin for your app, written in TypeScript. This is deployed in grafana as a standard app plugin, and is coupled with the `backend` component (if it exists) when deployed.
* `backend` - a backend component to an app plugin, written in go. This is deployed in grafana as a standard app plugin, and is coupled with the `frontend` component when deployed.
* `operator` - a standalone operator, written in go (see [Operator-based applications](./application-design/README.md#operator-based-applications) in [Application design patterns](./application-design/README.md)).

Multiple components can be specified in the same command. The full syntax is:
```
grafana-app-sdk project component add <list of space-separated components> [-s|--source=kinds]
```

A list of valid components can also be found by running 
```
grafana-app-sdk project component add
```
Without any components to add

### frontend

The frontend component generates TypeScript code and configuration for a grafana plugin in `plugin`, similar to the output of the npm [grafana/create-plugin](https://www.npmjs.com/package/@grafana/create-plugin) tool (if you would like to use `create-plugin` instead, see the [Get started](https://grafana.com/developers/plugin-tools/) docs, and create an app plugin).

### backend

The backend component generates plugin backend code in `pkg/plugin` for setting up a set of HTTP handlers to proxy Create, Read, Update, Delete, and List requests to the API server, using a `TypedStore`, and pulling kube config information for the API server from the `secureJsonData` of the plugin. It also adds a `main.go` to `plugin/pkg`, which is the entrypoint for the back-end component of the grafana app plugin.

> [!NOTE]
> `backend` component generation will eventually be deprecated in favor of the frontend component communicating directly with the API server, rather than through a back-end proxy.

### operator

The operator component generates a boilerplate watcher for each kind in `source` in `pkg/watchers/`. The rest of the operator code is generated in `cmd/operator`, and includes configuration, telemetry, a `main` file that sets up an operator using the `simple` package, and a Dockerfile for building a deploying a docker image for your operator. The generated boilerplate code works with the `operator` targets in the `Makefile` generated by `grafana-app-sdk project init`.

> [!NOTE]
> The `project` command also has a `kind` target, which can be used to generate a boilerplate kind, with
> ```
> grafana-app-sdk project kind add <kind name> [-s|--source=kinds]
> ```
> This is not a component command, but is useful for quickly generating a CUE kind in your `source` directory.

## Project Initialization

Project initialization is a special case of code generation used to initialize a new app-sdk project. It is not required to work with the app-sdk, but exists as a convenient way of setting up a project with the default workflow of the grafana-app-sdk. The syntax is:
```
grafana-app-sdk project init <project go module name>
```
This sets up your project with a go module, a `kinds` directory with a CUE module, a `Makefile` with some sample targets, and a `local` directory that can be used with `grafana-app-sdk project local` commands (see [Local Development & Testing](./local-development.md)).

## Examples & Testing

Code generation for both kinds and project components is done as part of the [issue tracker tutorial](./tutorials/issue-tracker/README.md) ([kind code generation](./tutorials/issue-tracker/03-generate-kind-code.md), [project component generation](./tutorials/issue-tracker/04-boilerplate.md)).

Automated testing of kind code generation is done using the files in [codegen/cuekind/testing/](../codegen/cuekind/testing/), with generated files compared against [codegen/testing/golden_generated](../codegen/testing/golden_generated/).
