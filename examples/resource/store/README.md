# Store/SimpleStore Example

This example code is a simple demonstration of using `crd.Store` and `crd.SimpleStore` to manage custom resources without the need for a kubernetes client. 
It creates some custom resource definitions, then uses `crd.Store` to perform basic CRUD actions, and then `crd.SimpleStore` to do the same with a single definition. 
For more details about `crd.Store` and `crd.SimpleStore`, see the godocs for them, or [the README in the `crd` package](../../../crd/README.md).

## To Run

### Using the run script
1. Make sure you have k3d and go1.18+ installed
2. Run the following to create a local k3d cluster and start the operator:
    ```shell
   $ ./run.sh
    ```

### Manually
Start a local kubernetes cluster or use a remote one to which you have permission to create CRD's and monitor them.
Set your kube context to the appropriate cluster, then run the operator:
```shell
$ go run basic.go --kubecfg="path_to_your_kube_config"
```
