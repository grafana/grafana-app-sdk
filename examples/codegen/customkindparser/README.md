# ThemaGenerator

`ThemaGenerator` in the `codegen` package can be used in-code if desired, though the best way to use it is as a CLI tool for codegen, 
using `go generate` directives, or as part of your `make` process, via the `app-platform generate` command.

The `ThemaGenerator` allows for code generation based supported CUE schemas. 
The example `main.go` has some direct usage in `main()`, and a `go generate` directive using `app-platform generate`.

In order to properly use code generation, you must define your schemas in CUE.

TODO: Enforce the grafana #CustomStructured definition? Dependency management in cue is unfun currently.
Would it be better to do some shenanigans to get a custom definition type (like #AppPlatformObject) which extends on grafana's #CustomStructured in this project?

`main.go` shows usage in-code and using `go:generate`. To run it as codegen:
1. Run `make build` at the root level of the project to build out the `target` directory and get the binary
1. Run `go generate` in this directory to execute all `go:generate` commands in-code and generate YAML and go files
The command in `main.go` will take all supported cue selectors in all cue files in the directory and turn them into eponymous JSON CRD files. 
You can try adding a new object to example.cue, or a new cue file with one or more #Lineage or #CRD schemas in it to see them also get generated.

To run in-code, just use `go run main.go`. This will generate a JSON CRD representation of the `myObject` selector.

For more information on the codegen/cli binary flags, you can check out the [README in /cmd/grafana-app-sdk](../../../cmd/grafana-app-sdk/README.md), 
or run the binary in `target` with the `--help` flag, like so:
```shell
$ target/app-platform --help
```