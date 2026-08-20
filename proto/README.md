# Protobuf definitions

This defines the protobuf definitions used to communicate with go-plugins


## Generating

Requires `buf`, `protoc-gen-go`, and `protoc-gen-go-grpc` on your `PATH`:

```sh
go install github.com/bufbuild/buf/cmd/buf@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

From the repository root, `mage protobuf:generate` regenerates both this module
and the legacy `pluginv2` one. To run `buf` directly, do it from this directory,
since the `out` paths in `buf.gen.yaml` are relative to it:

```sh
buf lint
buf generate
```

Output is written to `genproto/grafana/plugin/v3/` at the repository root
(Go package `pluginv3`).
