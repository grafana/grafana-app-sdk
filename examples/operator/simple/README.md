# Example of `simple` package usage

This example code is a one-file example of a kubernetes custom resource operator with a Watcher or a Reconciler, using the SDK `simple` package.

This example uses an app created using the `simple` package with an operator runner provided by the `operator` package. 
By default, the simple App uses the "opinionated" logic defined in [operator.OpinionatedWatcher](../../../operator/opinionatedwatcher.go#L38) and [operator.OpinionatedReconciler](../../../operator/reconciler.go#L135), 
but this can be turned off with configuration, which is noted in the `FIXME` comment in the file(s).

## To Run

### Using the run script

1. Make sure you have k3d and go1.18+ installed
2. Run the following to create a local k3d cluster and start the operator (you can choose between watcher and reconciler):
    ```shell
   $ ./run.sh <watcher/reconciler>
    ```

### Manually

Start a local kubernetes cluster or use a remote one to which you have permission to create CRD's and monitor them.
Set your kube context to the appropriate cluster, then run the operator:

```shell
$ go run <watcher/reconciler>/main.go --kubecfg="path_to_your_kube_config"
```

You may see one each of an error message about failing to watch and list the custom resource,
this is due to a slight delay between the custom resource being added, and the control plane knowing it exists.
This will only happen on the first run, when the resource definition doesn't yet exist in your cluster.

## Usage

The operator will monitor for changes to any BasicCustomResource in your cluster, and log them. To demonstrate, run:

```shell
$ kubectl create -f example.yaml
```

You should see a log line for the added resource. 

You can delete the custom resource and see the delete event with

```shell
$ kubectl delete BasicCustomResource test-resource
```

## Managing Custom Resources

You can see the custom resource created by the operator (and all custom resources in your cluster) with

```shell
$ kubectl get CustomResourceDefinitions
```

If you want to remove the custom resource definition created by the operator, you can do so with

```shell
$ kubectl delete CustomResourceDefinition basiccustomresources.example.grafana.com
```

Note that if there are any BasicCustomResources in your cluster, they will be deleted.

If the operator is not currently running, this command will hang while it waits for the resource deletion
(you can either run the operator, or remove the finalizer from each resource yourself via a kubectl patch).
If the operator is running, you will begin seeing errors in the console output, as the list/watch requests to kubernetes
will now result in errors.
