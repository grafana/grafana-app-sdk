# Initializing Our Project

Before we can begin, we'll need a directory where our project will live. Feel free to make it wherever you want, and name it however you want. Throughout the rest of this tutorial, if it's referenced, our folder name will be `issue-tracker-project`.
```shell
mkdir issue-tracker-project && cd issue-tracker-project
```
So now we have an empty directory to work in. The SDK _can_ be used with an existing project, but the setup is simpler with a fresh one.
```shell
$ tree .
.

0 directories, 0 files
```

Now, we could go about initializing a go module, and a cue module, and creating our directory structure here, but we're going to need the `grafana-app-sdk` CLI later for doing our codegen, and it can help us by easily setting up the start of our project, so let's download it now:
```shell
go install github.com/grafana/grafana-app-sdk/cmd/grafana-app-sdk@latest
```
If you're unfamiliar with `go install`, it's similar to `go get`, but will compile a binary for the `main` package in what it pulls, and put that in `$GOPATH/bin`. If you don't have `$GOPATH/bin` in your path, you will want to add it, otherwise the CLI commands won't work for you. You can check if the CLI was installed successfully with:
You can then check if the install was successful by running.

**Note:** currently [there is a bug](https://github.com/grafana/grafana-app-sdk/issues/189) preventing `go install` from succeeding, 
to install the grafana-app-sdk, use the binary process listed below, or clone this repository, run `make build`, and copy `target/grafana-app-sdk` into your `PATH`.

If you're not comfortable using `go install`, the [github releases page](https://github.com/grafana/grafana-app-sdk/releases) for the project includes a binary for each architecture per release. You can download the binary and add it to your `PATH` to use the SDK CLI the same way as if you used `go install`.
```shell
grafana-app-sdk --help
```

Now that we have the CLI installed, let's initialize our project. In this tutorial, we're going to use `github.com/grafana/issue-tracker-project` as our go module name, but you can use whatever name you like--it won't affect anything except some imports on code that we work on later.
```shell
grafana-app-sdk project init "github.com/grafana/issue-tracker-project"
```
And the output of the command:
```shell
$ grafana-app-sdk project init "github.com/grafana/issue-tracker-project"
 * Writing file go.mod
 * Writing file go.sum
 * Writing file kinds/cue.mod/module.cue
 * Writing file Makefile
 * Writing file local/config.yaml
 * Writing file local/scripts/cluster.sh
 * Writing file local/scripts/push_image.sh
 * Writing file local/Tiltfile
$ tree .
.
├── Makefile
├── cmd
│   └── operator
├── go.mod
├── go.sum
├── local
│   ├── Tiltfile
│   ├── additional
│   ├── config.yaml
│   ├── mounted-files
│   │   └── plugin
│   └── scripts
│       ├── cluster.sh
│       └── push_image.sh
├── pkg
├── plugin
└── kinds
    └── cue.mod
        └── module.cue

12 directories, 8 files
```

As we can see from the command output, and the `tree` command, the `project init` command created a go module in the current directory, a Makefile, and several other directories. 
* `cmd/operator` is an empty place for us to put the operator binary code
* `local` is the initial setup for a local development environment, which we'll talk about in [Deployment and Running Locally](deployment.md)
* `pkg` is the empty directory for all our go packages
* `plugin` is an empty directory where our grafana plugin code will live
* `kinds` contains a CUE module, and it's where we'll be [Defining Our Kinds](02-defining-our-kinds.md) next

### Next: [Defining Our Kinds](02-defining-our-kinds.md)
