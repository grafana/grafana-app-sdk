# API server example

This example demonstrates how to use the SDK to create a simple K8s compatible API server.

## Usage

Start the server:

```sh
$ make run
```

Data is stored in the `./data` directory, and can be deleted by running `make clean`.

## Test the server with `kubectl`:

Tell `kubectl` to use the local kubeconfig:
```sh
$ export KUBECONFIG=apiserver.kubeconfig
```

Check API discovery:
```sh
$ kubectl api-resources
NAME            SHORTNAMES   APIVERSION                 NAMESPACED   KIND
externalnames                core.grafana.internal/v1   false        ExternalName
```

Check API discovery:
```sh
$ kubectl api-resources
NAME            SHORTNAMES   APIVERSION                 NAMESPACED   KIND
externalnames                core.grafana.internal/v1   false        ExternalName
```

Create a resource:
```sh
$ kubectl create -f ./testdata/example.yaml 
externalname.core.grafana.internal/example created
```

List the resource:
```sh
$ kubectl get externalname -o yaml
```

Test validation:
```sh
$ kubectl create -f ./testdata/invalid.yaml
Error from server (InternalError): error when creating "./testdata/invalid.yaml": Internal error occurred: failed to validate externalname: spec.target: Required value
```

View the OpenAPI spec for `core.grafana.internal/v1`: [https://localhost:6443/openapi/v3/apis/core.grafana.internal/v1]