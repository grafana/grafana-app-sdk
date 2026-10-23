# Store/SimpleStore Example

This example code is a simple demonstration of using `resource.Store` and `resource.SimpleStore` to manage resources as if they were in a key-value store.
It uses `resource.Store` to perform basic CRUD actions across several kinds, and then `resource.SimpleStore` to do the same with a single kind.
For more details, see [Resource Stores](../../../docs/resource-stores.md) or the godocs for the `resource` package.

## To Run

### Using the run script
1. Make sure you have k3d and go installed
2. Run the following to create a local k3d cluster and run the example:
    ```shell
   $ ./run.sh
    ```

### Manually
Point your kube config at a local cluster, then run the example:
```shell
$ go run store.go --kubecfg="path_to_your_kube_config"
```
