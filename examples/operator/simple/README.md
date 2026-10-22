# Example of `simple` package usage

This example code is a one-file example of an operator with a Watcher or a Reconciler, using the SDK `simple` package.

This example uses an app created using the `simple` package with an operator runner provided by the `operator` package. 
By default, the simple App uses the "opinionated" logic defined in [operator.OpinionatedWatcher](../../../operator/opinionatedwatcher.go#L38) and [operator.OpinionatedReconciler](../../../operator/reconciler.go#L135), 
but this can be turned off with configuration, which is noted in the `FIXME` comment in the file(s).

## To Run

### Using the run script

1. Make sure you have k3d and go installed
2. Run the following to create a local k3d cluster and start the operator (you can choose between watcher and reconciler):
    ```shell
   $ ./run.sh <watcher/reconciler>
    ```

### Manually

Point your kube config at a local cluster, then run the operator:

```shell
$ go run <watcher/reconciler>/main.go --kubecfg="path_to_your_kube_config"
```

## Usage

The operator will monitor for changes to any BasicCustomResource, and log them. To demonstrate, run:

```shell
$ kubectl create -f example.yaml
```

You should see a log line for the added resource. 

You can delete the custom resource and see the delete event with

```shell
$ kubectl delete BasicCustomResource test-resource
```
