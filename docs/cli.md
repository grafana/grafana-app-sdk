# Command-Line Interface

## Installation

The SDK CLI tool can be installed via `go install`, by downloading a binary, or by building from source.

### go install

To install via `go install`, you will need to make sure you have [go](https://go.dev/) installed on your machine. Run the following:
```shell
go install github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk@latest
```
This will build the latest release on your machine and put the binary in your go path (ensure that your `GOPATH/bin` (typically `$HOME/go/bin`) is in your `PATH`).

### Download Release Binary

If you prefer to download a binary and add it to your `PATH`, you can install a binary from the releases page:

1. [Visit the latest release page](https://github.com/grafana/grafana-app-sdk/releases/latest)
2. Find the appropriate artifact for your OS and architecture
3. Download the artifact and untar it into your PATH

### Building From Source

The CLI can be built from source by cloning the repository and running `make build`. This will put the built binary in `./target`. 
You can then copy the binary into your `PATH` as with downloading the binary. 

Note that building from source will have a different output of `grafana-app-sdk version` than one from either of the two other methods.

## Usage

The CLI tool is used for working with app projects, and includes commands to do the following:
* Initialize a new project
* Generate boilerplate code for a project component
* Generate go and TypeScript kind code from CUE kinds
* Create a local development environment

The general workflow using the CLI for a project is:
1. Initialize the project
2. Define kinds and generate go and TypeScript kind code
3. Generate boilerplate code for all applicable components

Then, the dev loop is:
1. Iterate on code and/or kinds
2. Re-generate go and TypeScript kind code if there are kind updates
3. Create local development environment and test
4. GOTO 1

When working with a project, there is the `generate` command, which is intended to be run many times, whenever the underlying kinds change for your project, or upon upgrades of the SDK library, and there is the set of commands grouped under the `project` command, all of which are based around doing a specific things with your project.

For any command, you can get usage information, including flags, with 
```
grafana-app-sdk <command> --help
```

### Initialize a new project

In an empty directory, run 
```
grafana-app-sdk project init <my project module name>
```
This command does the following:
* Creates an empty go module (similar to `go mod init`)
* Creates an empty CUE module in `--source` (`-s`, defaults to `kinds`)
* Creates an empty directory structure for your project
* Creates a default Makefile with prebuilt targets

If you `init` a project in a directory which already has a go module, it will ask if you want to overwrite or merge your project with the existing go project. However, it's generally advised that you use `project init` only on empty directories.

### Adding a New Kind

You can manually create kinds in CUE in your kind path (the `--source` or `-s` flag provided to commands, which defaults to `kinds`), but if you want a fully-commented template kind, you can use:
```
grafana-app-sdk project kind add <KindName>
```
This will add a CUE file to your `--source` directory with a filled-out custom kind in CUE, leaving only the lineage's schema(s) for you to fill out. It also provides extensive comment documentation on the different fields.

### Generate go and TypeScript kind code from CUE kinds

Arguably the most important function of the CLI, as it needs to be run whenever you update your kinds, is generating code from your CUE kinds.

For more details on kinds, see [Custom Kinds & CUE](./custom-kinds/README.md).

To generate code for your kinds, use 
```
grafana-app-sdk generate [-s|--source <cue module path>]
``` 
If you created your project with `project init`, then your default Makefile calls this command with `make generate`.

### Generate Boilerplate Code

```
grafana-app-sdk project component add <component>
``` 
allows you to add boilerplate code for a project component. Allowed components are:
* `frontend`
* `backend`
* `operator`

The boilerplate code will use the kinds defined in `-s|--source` (defaults to `./schemas`) for defining watchers, API routes, and other bits of code that deal woth the kinds.

Boilerplate code generation is typically expected to be run once, and then modified (the code may contain `TODO` or `FIXME` comments to prompt you as to where you need to modify it).

### Create a local development environment

The SDK has two subcommands as part of `grafana-app-sdk project local`: `init` and `generate`.

```
grafana-app-sdk project local init
``` 
initializes the `./local` directory in your project, and is automatically called as part of `project init`. This creates a Tiltfile, some scripts, and a config file.

```
grafana-app-sdk project local generate
``` 
generates a k3d config and kubernetes manifests for a local deployment based on your kinds in `-s|--source` (defaults to `./schemas`), 
and the configuration in `./local/config.yaml`. You can use the k3d config to create a local kubernetes, and the Tiltfile created by `project local init` in `./local/Tiltfile`.
The `make local/up` target in the detault Makefile generated by `project init` do this for you automatically.

To extend the local environment with custom kubernetes manifests, place them in `./local/custom`, and the Tiltfile will automatically pick them up 
(you can even overwrite objects in `./local/generated` this way).

Read more: [Local Development](local-development.md)

### Other commands

To determine the version of the SDK CLI you are using, run `grafana-app-sdk version [-v|--verbose]`.
