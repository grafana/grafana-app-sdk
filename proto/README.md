# Plugin protocol v3 protobuf definitions

This directory is the source of truth for the experimental
`grafana.plugin.v3` wire protocol used by Go backend plugins. Generated Go code
is checked in under `plugin/genproto/grafana/plugin/v3`.

## Regenerating Go code

Install `buf`, `protoc-gen-go`, and `protoc-gen-go-grpc`. The generator versions
should match those recorded at the top of the checked-in generated files.

```sh
go install github.com/bufbuild/buf/cmd/buf@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
```

Run these commands from this directory because the output paths in
`buf.gen.yaml` are relative to it:

```sh
buf lint
buf generate
```

After generation, run the focused tests and verify that the generated diff only
contains intentional protocol changes:

```sh
(cd ../plugin && go test ./...)
git diff --check
```

Do not edit files under `plugin/genproto` by hand.
